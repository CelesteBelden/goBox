package main

import (
	"bufio"
	"fmt"
	"log"

	network "github.com/libp2p/go-libp2p/core/network"
)

// handleHelloStream handles the initial handshake.
func handleHelloStream(stream network.Stream) {
	defer stream.Close()

	reader := bufio.NewReader(stream)
	msg, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("Error reading from stream: %v", err)
		return
	}

	remotePeer := stream.Conn().RemotePeer()
	connectedPeerID = remotePeer
	// Signal that peer ID is set
	select {
	case peerIDSet <- true:
	default:
	}

	fmt.Printf("\n[Connected] Peer: %s\n", remotePeer.String()[:12])
	fmt.Printf("Received: %s", msg)

	// Send acknowledgment
	response := "Hello! Ready to chat.\n"
	_, err = stream.Write([]byte(response))
	if err != nil {
		log.Printf("Error writing to stream: %v", err)
	}
}

// handleChatStream handles incoming chat messages.
func handleChatStream(stream network.Stream) {
	defer stream.Close()

	remotePeer := stream.Conn().RemotePeer()
	reader := bufio.NewReader(stream)
	msg, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	fmt.Printf("\n[%s]: %s", remotePeer.String()[:12], msg)
}
