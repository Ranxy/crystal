package server

import (
	"net"
	"sync"
)

type Connection struct {
	AgentConn     net.Conn
	ExternalConns map[net.Conn]bool
	mutex         sync.Mutex
	closeOnce     sync.Once     // Ensures the connection is closed only once
	closeChan     chan struct{} // Channel to signal connection closure
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

// Close gracefully closes the agent connection and all associated external connections.
// It uses sync.Once to ensure that the closing logic is executed only once,
// preventing panics from multiple goroutines trying to close the same resources.
func (c *Connection) Close() {
	c.closeOnce.Do(func() {
		// First, close the agent connection. This will unblock any pending I/O.
		handleNetCloseError(c.AgentConn)

		// Then, close all external connections.
		c.mutex.Lock()
		for conn := range c.ExternalConns {
			handleNetCloseError(conn)
		}
		c.mutex.Unlock()

		// Finally, close the channel to signal that the connection is fully torn down.
		close(c.closeChan)
	})
}
