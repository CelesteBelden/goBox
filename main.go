package main

import (
	"log"
	"os"
)

func main() {
	// Create the node
	node, err := createNode()
	if err != nil {
		log.Fatalf("Error creating node: %v", err)
	}
	defer node.Close()

	// Set up stream handlers
	setupStreamHandlers(node)
	// Start mDNS discovery (LAN)
	if err := startMDNS(node, "gobox-mdns"); err != nil {
		log.Printf("mDNS not started: %v", err)
	}

	// Print node details
	printNodeAddress(node)

	// Check if a peer address was provided as a command-line argument
	if len(os.Args) > 1 {
		peerAddr := os.Args[1]
		if err := connectToPeer(node, peerAddr); err != nil {
			log.Fatalf("Error: %v", err)
		}

		// Start sending messages
		go sendMessages(node)
	} else {
		waitForConnection()

		// Start sending messages (will block until peer connects)
		go sendMessages(node)
	}

	// Keep the program running indefinitely
	select {}
}
