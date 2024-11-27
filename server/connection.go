package server

import (
	"net"
	"sync"
)

type Connection struct {
	AgentConn     net.Conn
	ExternalConns map[net.Conn]bool
	mutex         sync.Mutex
	closeChan     chan struct{} // 新增关闭通道
}

func NewConnection(agentConn net.Conn) *Connection {
	return &Connection{
		AgentConn:     agentConn,
		ExternalConns: make(map[net.Conn]bool),
		closeChan:     make(chan struct{}),
	}
}

func (c *Connection) AddExternalConn(extConn net.Conn) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.ExternalConns[extConn] = true
}

func (c *Connection) RemoveExternalConn(extConn net.Conn) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.ExternalConns, extConn)
}

func (c *Connection) Close() {
	c.AgentConn.Close()
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for conn := range c.ExternalConns {
		conn.Close()
	}
	close(c.closeChan)
}
