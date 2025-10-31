package agent

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
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

// forward copies data from src to dst and signals completion via the WaitGroup.
func forward(dst, src net.Conn, wg *sync.WaitGroup) {
	defer wg.Done()
	// Closing the destination connection when copying is done or fails.
	// This is important to unblock the other forwarder.
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		// We expect an error when the connection is closed, so we only log unexpected errors.
		if !strings.Contains(err.Error(), "use of closed network connection") && err != io.EOF {
			log.Printf("Forwarding error: %v", err)
		}
	}
}

func Start(connAAddr string, connBAddr string) error {
	connA, err := dial(connAAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to connA: %w", err)
	}

	connB, err := dial(connBAddr)
	if err != nil {
		connA.Close() // Clean up connA if connB fails
		return fmt.Errorf("failed to connect to connB: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine to copy from A (server) to B (internal service)
	go forward(connB, connA, &wg)

	// Goroutine to copy from B (internal service) to A (server)
	go forward(connA, connB, &wg)

	// Goroutine to wait for both forwarders to finish and then close a channel.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Handle OS signals for graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Block until either both forwarders are done, or an OS signal is received.
	select {
	case <-done:
		log.Println("Connections closed normally.")
	case <-sigChan:
		log.Println("Received shutdown signal. Closing connections.")
		// Gracefully close connections. This will cause the `io.Copy` in `forward`
		// to return, which will in turn lead to the WaitGroup counter decreasing.
		connA.Close()
		connB.Close()
		// Wait for the 'done' signal to ensure cleanup is complete.
		<-done
	}

	log.Println("Agent shut down.")
	return nil
}
