package agent

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func dial(address string) (net.Conn, error) {
	var dialer net.Dialer
	// set timeout
	dialer.Timeout = 10 * time.Second
	if strings.HasPrefix(address, "tcp://") || strings.HasPrefix(address, "unix://") {
		u, err := url.Parse(address)
		if err != nil {
			return nil, fmt.Errorf("failed to parse address %q: %v", address, err)
		}
		switch u.Scheme {
		case "tcp":
			host := u.Host
			if host == "" {
				host = u.Path
			}
			return dialer.Dial("tcp", host)
		case "unix":
			socketPath := u.Path
			return dialer.Dial("unix", socketPath)
		default:
			return nil, fmt.Errorf("unsupported scheme %q in address %q", u.Scheme, address)
		}
	} else {
		if strings.Contains(address, ":") && !strings.HasPrefix(address, "/") {
			// Assume it's a tcp host:port
			return dialer.Dial("tcp", address)
		} else {
			// Assume it's a unix socket path
			return dialer.Dial("unix", address)
		}
	}
}

func forward(src, dst net.Conn, shutdownChan chan struct{}, quitChan chan struct{}) {
	defer dst.Close()
	dataChan := make(chan []byte)
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := src.Read(buf)
			if err != nil {
				log.Println("Read from src failed:", err)
				close(dataChan)
				return
			}
			dataChan <- buf[:n]
		}
	}()
	for {
		select {
		case <-shutdownChan:
			log.Println("Shutting down forwarding from src to dst")
			return
		case data, ok := <-dataChan:
			if !ok {
				log.Println("Source connection closed")
				quitChan <- struct{}{}
				return
			}
			_, err := dst.Write(data)
			if err != nil {
				log.Println("Write to dst failed:", err)
				quitChan <- struct{}{}
				return
			}
		}
	}
}

func Start(connAAddr string, connBAddr string) error {

	connA, err := dial(connAAddr)
	if err != nil {
		return fmt.Errorf("Failed to connect to connA: %w", err)
	}
	defer connA.Close()

	connB, err := dial(connBAddr)
	if err != nil {
		return fmt.Errorf("Failed to connect to connB: %w", err)
	}
	defer connB.Close()

	// control channel
	shutdownChan := make(chan struct{})
	quitChan := make(chan struct{})

	// sig process
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received shutdown signal")
		close(shutdownChan)
	}()

	go forward(connA, connB, shutdownChan, quitChan)
	go forward(connB, connA, shutdownChan, quitChan)

	// wait quit or shutdown
	for i := 0; i < 2; i++ {
		select {
		case <-quitChan:
			log.Println("A forwarding goroutine exited, initiating shutdown...")
			close(shutdownChan)
		case <-shutdownChan:
			log.Println("Shutting down on user request...")
		}
	}

	log.Println("All done.")
	return nil
}
