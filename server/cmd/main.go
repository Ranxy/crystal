package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Ranxy/crystal/server"
)

func main() {
	var listen string
	flag.StringVar(&listen, "listen", ":9000", "Address for connection handle")
	flag.Parse()

	if listen == "" {
		log.Fatal("listen parameters are required")
	}

	s := server.NewServer(listen)
	log.Printf("Server started on %s\n", listen)

	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start proxy in a goroutine
	go s.StartProxy()

	// Wait for shutdown signal
	<-sigChan
	log.Println("Received shutdown signal. Closing server.")
	s.Close()
	log.Println("Server shut down.")
}
