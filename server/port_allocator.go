package server

import (
	"fmt"
	"log"
	"net"
	"sync"
)

type PortAllocator struct {
	ports []int
	used  map[int]bool
	mu    sync.Mutex
}

func NewPortAllocator(startPort, endPort int) *PortAllocator {
	pa := &PortAllocator{
		ports: make([]int, 0),
		used:  make(map[int]bool),
	}
	for port := startPort; port <= endPort; port++ {
		pa.ports = append(pa.ports, port)
	}
	return pa
}

func (pa *PortAllocator) GetAvailablePort() (int, error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	for _, port := range pa.ports {
		if !pa.used[port] {
			// try bind
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				// bind failed, continue
				continue
			}
			// bind success and use this port
			pa.used[port] = true
			ln.Close()
			log.Printf("Allocated port %d\n", port)
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port")
}

func (pa *PortAllocator) ReleasePort(port int) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	delete(pa.used, port)
	log.Printf("Released port %d\n", port)
}
