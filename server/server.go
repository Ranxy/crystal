package server

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
)

type Server struct {
	listener         net.Listener
	portToConnection map[int]*Connection
	portAllocator    *PortAllocator
}

func NewServer(listen string) *Server {
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		log.Fatal(err)
	}

	s := &Server{
		listener:         listener,
		portToConnection: make(map[int]*Connection),
		portAllocator:    NewPortAllocator(9000, 10000),
	}
	return s
}

func (s *Server) Close() {
	log.Println("start close server!")
	handleNetCloseError(s.listener)
}

func (s *Server) StartProxy() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go s.handleAgentConn(conn)
	}
}

func (s *Server) handleAgentConn(agentConn net.Conn) {
	port, err := s.portAllocator.GetAvailablePort()
	if err != nil {
		log.Println("Failed to allocate port:", err)
		handleNetCloseError(agentConn) // Close connection if we can't get a port
		return
	}
	defer s.portAllocator.ReleasePort(port)

	conn := NewConnection(agentConn)
	s.portToConnection[port] = conn
	defer delete(s.portToConnection, port)

	log.Printf("Agent connected from %s, forwarding public port %d", agentConn.RemoteAddr(), port)

	// Listen on all interfaces for external connections
	extListener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Printf("Failed to listen on public port %d: %v", port, err)
		return
	}
	defer handleNetCloseError(extListener)

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine to handle incoming external connections
	go func() {
		defer wg.Done()
		handleExternalConnections(extListener, conn)
	}()

	// Goroutine to forward data from the agent to all external clients
	go func() {
		defer wg.Done()
		forwardAgentToExternal(conn)
		// Once agent forwarding stops (e.g., agent disconnects),
		// we must close the external listener to stop accepting new connections.
		handleNetCloseError(extListener)
	}()

	wg.Wait()
	log.Printf("Stopped forwarding for agent %s on port %d", agentConn.RemoteAddr(), port)
}

// handleExternalConnections accepts new external clients and starts forwarding their data.
func handleExternalConnections(listener net.Listener, conn *Connection) {
	for {
		extConn, err := listener.Accept()
		if err != nil {
			// This error is expected when the listener is closed, so we just exit.
			if strings.Contains(err.Error(), "use of closed network connection") {
				log.Println("External listener is closed. Stopping to accept new connections.")
			} else {
				log.Println("Error accepting external connection:", err)
			}
			return
		}
		log.Printf("Accepted external connection from %s for agent %s", extConn.RemoteAddr(), conn.AgentConn.RemoteAddr())
		conn.AddExternalConn(extConn)
		go forwardExternalToAgent(extConn, conn)
	}
}

// forwardExternalToAgent forwards data from a single external client to the agent.
func forwardExternalToAgent(extConn net.Conn, conn *Connection) {
	defer func() {
		log.Printf("Closing external connection from %s", extConn.RemoteAddr())
		conn.RemoveExternalConn(extConn)
		handleNetCloseError(extConn)
	}()

	// Using io.Copy for efficient, buffered data transfer
	if _, err := io.Copy(conn.AgentConn, extConn); err != nil {
		// This error is expected if the agent connection is closed while copying
		if !strings.Contains(err.Error(), "use of closed network connection") && err != io.EOF {
			log.Printf("Error forwarding from external client %s to agent: %v", extConn.RemoteAddr(), err)
		}
	}
}

// forwardAgentToExternal reads data from the agent and broadcasts it to all connected external clients.
func forwardAgentToExternal(conn *Connection) {
	// When this function exits (e.g., agent disconnects), close the entire connection object.
	// This will close the agent connection and all associated external connections.
	defer conn.Close()

	buf := make([]byte, 2048) // Increased buffer size
	for {
		n, err := conn.AgentConn.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading from agent %s: %v", conn.AgentConn.RemoteAddr(), err)
			} else {
				log.Printf("Agent %s disconnected.", conn.AgentConn.RemoteAddr())
			}
			break
		}

		data := buf[:n]
		conn.mutex.Lock()
		// Create a slice of connections to write to, to avoid holding the lock while writing.
		connsToWrite := make([]net.Conn, 0, len(conn.ExternalConns))
		for extConn := range conn.ExternalConns {
			connsToWrite = append(connsToWrite, extConn)
		}
		conn.mutex.Unlock()

		for _, extConn := range connsToWrite {
			_, err := extConn.Write(data)
			if err != nil {
				log.Printf("Error writing to external client %s: %v. Closing connection.", extConn.RemoteAddr(), err)
				// Let the reading goroutine for this connection handle the closure.
			}
		}
	}
}

// handleNetCloseError closes an io.Closer and logs any unexpected errors.
func handleNetCloseError(c io.Closer) {
	err := c.Close()
	if err != nil {
		// It's common to get this error when a connection is already closed by the other side,
		// so we can safely ignore it.
		if !strings.Contains(err.Error(), "use of closed network connection") {
			log.Printf("Error during close: %s\n", err.Error())
		}
	}
}
