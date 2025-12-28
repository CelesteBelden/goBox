package main

import (
	"bufio"
	"context"
	"fmt"
	"time"

	host "github.com/libp2p/go-libp2p/core/host"
	peer "github.com/libp2p/go-libp2p/core/peer"
	protocol "github.com/libp2p/go-libp2p/core/protocol"
)

// connectToPeer attempts to connect to a peer using their multiaddress.
func connectToPeer(node host.Host, peerAddr string) error {
	fmt.Printf("\nAttempting to connect to: %s\n", peerAddr)

	// Parse the peer multiaddress
	addrInfo, err := peer.AddrInfoFromString(peerAddr)
	if err != nil {
		return fmt.Errorf("invalid peer address: %w", err)
	}

	// Connect to the peer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := node.Connect(ctx, *addrInfo); err != nil {
		return fmt.Errorf("failed to connect to peer: %w", err)
	}

	connectedPeerID = addrInfo.ID
	// Signal that peer ID is set
	select {
	case peerIDSet <- true:
	default:
	}

	fmt.Println("\nSuccessfully connected to peer!")

	// Send initial hello message
	stream, err := node.NewStream(ctx, addrInfo.ID, protocol.ID(helloProtocolID))
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}

	message := "Hello! Ready to chat.\n"
	_, err = stream.Write([]byte(message))
	if err != nil {
		stream.Close()
		return fmt.Errorf("failed to send hello message: %w", err)
	}

	// Read acknowledgment
	reader := bufio.NewReader(stream)
	response, err := reader.ReadString('\n')
	if err != nil {
		stream.Close()
		return fmt.Errorf("failed to read response: %w", err)
	}

	fmt.Printf("Response: %s", response)
	stream.Close()

	return nil
}

// waitForConnection waits for an incoming connection from a peer.
func waitForConnection() {
	fmt.Println("\nNo peer address provided. Waiting for incoming connection...")
	fmt.Println("To connect to this node, use: go run hello.go <multiaddress>")
}
