package main

import (
	"flag"
	"log"

	"github.com/Ranxy/crystal/agent"
)

func main() {
	var connAAddr, connBAddr string
	flag.StringVar(&connAAddr, "a", "", "Address for connection A (e.g., tcp://localhost:1234 or unix:///path/to/socketA)")
	flag.StringVar(&connBAddr, "b", "", "Address for connection B (e.g., tcp://localhost:5678 or unix:///path/to/socketB)")
	flag.Parse()

	if connAAddr == "" || connBAddr == "" {
		log.Fatal("Both -a and -b parameters are required")
	}

	err := agent.Start(connAAddr, connBAddr)
	if err != nil {
		log.Fatalln("Start Agent Failed, Err ", err)
	}
}
