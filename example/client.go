package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	mu      sync.Mutex
	clients map[net.Conn]bool
)

// Telnet command codes
const (
	IAC  = 255 // Interpret As Command
	WILL = 251
	WONT = 252
	DO   = 253
	DONT = 254
	IP   = 244 // Interrupt Process
)

func main() {
	clients = make(map[net.Conn]bool)

	var addr string
	flag.StringVar(&addr, "addr", ":8111", "Address for listen")
	flag.Parse()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	log.Printf("Start server at %s \n", addr)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		mu.Lock()
		for conn := range clients {
			conn.Write([]byte("Server Shutdown\n"))
			conn.Close()
		}
		clients = make(map[net.Conn]bool)
		mu.Unlock()
		os.Exit(0)
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		fmt.Printf("New connection: %s\n", conn.RemoteAddr())
		mu.Lock()
		clients[conn] = true
		mu.Unlock()
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer func() {
		mu.Lock()
		delete(clients, conn)
		mu.Unlock()
		conn.Close()
		log.Printf("Connection %s closed\n", conn.RemoteAddr())
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Channel to receive data from the connection
	readChan := make(chan []byte)
	errChan := make(chan error)

	// Goroutine to read data from the connection
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				errChan <- err
				return
			}
			readChan <- buf[:n]
		}
	}()

	for {
		select {
		case <-ticker.C:
			currentTime := time.Now().Format("2006-01-02 15:04:05")
			message := "Hello World " + currentTime + "\n"
			if _, err := conn.Write([]byte(message)); err != nil {
				return // Exit if write fails
			}
		case data := <-readChan:
			// Check for Telnet Interrupt command (Ctrl+C)
			if bytes.Contains(data, []byte{IAC, IP}) {
				log.Printf("Received Ctrl+C (Interrupt) from %s. Closing connection.", conn.RemoteAddr())
				conn.Write([]byte("Interrupt received. Bye!\n"))
				return
			}
			line := string(bytes.TrimSpace(data))

			// Echo received data back to the client
			fmt.Printf("From client %s: %s\n", conn.RemoteAddr(), line)

		case err := <-errChan:
			if err == io.EOF {
				log.Printf("Client %s disconnected (EOF).\n", conn.RemoteAddr())
			} else {
				log.Printf("Error reading from %s: %v\n", conn.RemoteAddr(), err)
			}
			return
		}
	}
}
