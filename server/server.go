package server

import (
	"fmt"
	"log"
	"net"
	"sync"
)

var (
	portToConnection map[int]*Connection
	portAllocator    *PortAllocator
)

func init() {
	portToConnection = make(map[int]*Connection)
	portAllocator = NewPortAllocator(9000, 10000)
}

func StartProxy(listen string) {
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go handleAgentConn(conn)
	}
}

func handleAgentConn(agentConn net.Conn) {
	defer agentConn.Close() // Ensure agentConn is closed when the function exits

	port, err := portAllocator.GetAvailablePort()
	if err != nil {
		log.Println(err)
		return
	}
	defer portAllocator.ReleasePort(port) // Release port when the function exits

	conn := NewConnection(agentConn)
	portToConnection[port] = conn
	defer delete(portToConnection, port) // Remove from map when the function exits

	log.Printf("Start Agent From %s and forward to %d", agentConn.RemoteAddr(), port)

	extListener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Println(err)
		return
	}
	defer extListener.Close() // Ensure listener is closed when the function exits

	var wg sync.WaitGroup

	// Handle external connections
	wg.Add(1)
	go func() {
		defer wg.Done()
		handleExternalConnections(extListener, conn)
	}()

	// Forward data from agent to external connections
	wg.Add(1)
	go func() {
		defer wg.Done()
		forwardAgentToExternal(conn)
	}()

	// Wait for all goroutines to finish
	wg.Wait()
}

func handleExternalConnections(listener net.Listener, conn *Connection) {
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
	defer extConn.Close()
	buf := make([]byte, 1024)
	for {
		n, err := extConn.Read(buf)
		if err != nil {
			log.Println(err)
			break
		}
		_, err = agentConn.Write(buf[:n])
		if err != nil {
			log.Println(err)
			break
		}
	}
}

func forwardAgentToExternal(conn *Connection) {
	defer conn.Close()
	buf := make([]byte, 1024)
	for {
		n, err := conn.AgentConn.Read(buf)
		if err != nil {
			log.Println(err)
			break
		}
		conn.mutex.Lock()
		for extConn := range conn.ExternalConns {
			_, err := extConn.Write(buf[:n])
			if err != nil {
				log.Println(err)
				conn.RemoveExternalConn(extConn)
				extConn.Close()
			}
		}
		conn.mutex.Unlock()
	}
}
