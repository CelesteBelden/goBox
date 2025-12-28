package main

import (
	"fmt"

	libp2p "github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p/core/host"
	protocol "github.com/libp2p/go-libp2p/core/protocol"
	multiaddr "github.com/multiformats/go-multiaddr"
)

// createNode creates a new libp2p host and returns it.
func createNode() (host.Host, error) {
	h, err := libp2p.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create node: %w", err)
	}
	return h, nil
}

// setupStreamHandlers registers the protocol handlers on the node.
func setupStreamHandlers(node host.Host) {
	node.SetStreamHandler(protocol.ID(helloProtocolID), handleHelloStream)
	node.SetStreamHandler(protocol.ID(chatProtocolID), handleChatStream)
}

// printNodeAddress prints the full multiaddress of a libp2p host.
func printNodeAddress(node host.Host) {
	fmt.Println("Node started successfully!")
	fmt.Println("Peer ID:", node.ID())
	fmt.Println("\nListening on:")
	addrs := node.Addrs()
	if len(addrs) > 0 {
		full := addrs[0].Encapsulate(multiaddr.StringCast("/p2p/" + node.ID().String()))
		fmt.Printf("  %s\n", full)
	}
}
