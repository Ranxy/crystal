package server

import (
	"fmt"
	"log"
	"net"
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
	port, err := portAllocator.GetAvailablePort()
	if err != nil {
		log.Println(err)
		agentConn.Close()
		return
	}
	conn := NewConnection(agentConn)
	portToConnection[port] = conn
	log.Printf("Start Agent From %s and forward to %d", agentConn.RemoteAddr(), port)
	extListener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Println(err)
		agentConn.Close()
		portAllocator.ReleasePort(port)
		return
	}

	go handleExternalConnections(extListener, conn)
	go forwardAgentToExternal(conn)
	agentConn.Close()
	extListener.Close()
	portAllocator.ReleasePort(port)
	delete(portToConnection, port)
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
	buf := make([]byte, 1024)
	for {
		n, err := extConn.Read(buf)
		if err != nil {
			log.Println(err)
			extConn.Close()
			break
		}
		_, err = agentConn.Write(buf[:n])
		if err != nil {
			log.Println(err)
			agentConn.Close()
			break
		}
	}
}

func forwardAgentToExternal(conn *Connection) {
	buf := make([]byte, 1024)
	for {
		n, err := conn.AgentConn.Read(buf)
		if err != nil {
			log.Println(err)
			conn.Close()
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
