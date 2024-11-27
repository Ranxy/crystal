package main

import (
	"flag"
	"log"

	"github.com/Ranxy/crystal/server"
)

func main() {
	var listen string
	flag.StringVar(&listen, "listen", ":9000", "Address for connection handle")
	flag.Parse()

	if listen == "" {
		log.Fatal("listen parameters are required")
	}

	server.StartProxy(listen)
}
