package main

import (
	peer "github.com/libp2p/go-libp2p/core/peer"
)

// Protocol definitions
const (
	helloProtocolID = "/hello/1.0.0" // Protocol for initial handshake
	chatProtocolID  = "/chat/1.0.0"  // Protocol for chat messages
)

// Global state for peer connection
var (
	connectedPeerID peer.ID
	peerIDSet       = make(chan bool, 1)
)

func init() {
	// Initialize zero value for peer ID check
	if connectedPeerID == "" {
		connectedPeerID = ""
	}
}
