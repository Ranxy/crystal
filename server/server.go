package server

import (
	"fmt"
	"io"
	"log"
	"net"
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
	defer handleNetCloseError(agentConn) // Ensure agentConn is closed when the function exits

	port, err := s.portAllocator.GetAvailablePort()
	if err != nil {
		log.Println(err)
		return
	}
	defer s.portAllocator.ReleasePort(port) // Release port when the function exits

	conn := NewConnection(agentConn)
	s.portToConnection[port] = conn
	defer delete(s.portToConnection, port) // Remove from map when the function exits

	log.Printf("Start Agent From %s and forward to %d", agentConn.RemoteAddr(), port)

	extListener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Println(err)
		return
	}
	defer handleNetCloseError(extListener) // Ensure listener is closed when the function exits

	quitChan := make(chan struct{})

	var wg sync.WaitGroup

	// Handle external connections
	wg.Add(1)
	go func() {
		defer wg.Done()
		handleExternalConnections(extListener, conn, quitChan)
	}()

	// Forward data from agent to external connections
	wg.Add(1)
	go func() {
		defer wg.Done()
		forwardAgentToExternal(conn, quitChan)
	}()

	// Wait for all goroutines to finish
	wg.Wait()
}

func handleExternalConnections(listener net.Listener, conn *Connection, quitChan chan struct{}) {

	go func() {
		select {
		case <-quitChan:
			log.Println("Agent external quit")
			handleNetCloseError(listener)
			return
		case <-conn.closeChan:
			log.Println("handle external conn close")
			handleNetCloseError(listener)
			return
		}
	}()

	for {
		extConn, err := listener.Accept()
		if err != nil {
			log.Println(err)
			return
		}
		conn.AddExternalConn(extConn)
		go forwardExternalToAgent(extConn, conn.AgentConn)

	}
}

func forwardExternalToAgent(extConn net.Conn, agentConn net.Conn) {
	defer handleNetCloseError(extConn)
	buf := make([]byte, 1024)
	for {
		n, err := extConn.Read(buf)
		if err != nil {
			log.Println("reader ext conn: ", err)
			break
		}
		_, err = agentConn.Write(buf[:n])
		if err != nil {
			log.Println("write agent conn: ", err)
			break
		}
	}
}

func forwardAgentToExternal(conn *Connection, quitChan chan struct{}) {
	defer conn.Close()
	buf := make([]byte, 1024)
	for {
		n, err := conn.AgentConn.Read(buf)
		if err != nil {
			log.Println("reade agent conn: ", err)
			close(quitChan)
			break
		}
		conn.mutex.Lock()
		for extConn := range conn.ExternalConns {
			_, err := extConn.Write(buf[:n])
			if err != nil {
				log.Println("write ext conn: ", err)
				conn.RemoveExternalConn(extConn)
				handleNetCloseError(extConn)
			}
		}
		conn.mutex.Unlock()
	}
}

func handleNetCloseError(c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Printf("closed with error %s\n", err.Error())
	}
}
