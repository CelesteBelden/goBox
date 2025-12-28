package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	host "github.com/libp2p/go-libp2p/core/host"
	protocol "github.com/libp2p/go-libp2p/core/protocol"
)

// sendMessages reads from stdin and sends messages to the connected peer.
func sendMessages(node host.Host) {
	// Wait for peer ID to be set (for listening nodes)
	<-peerIDSet

	fmt.Println("Type your messages (Ctrl+C to exit):")
	reader := bufio.NewReader(os.Stdin)

	for {
		msg, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		stream, err := node.NewStream(ctx, connectedPeerID, protocol.ID(chatProtocolID))
		cancel()

		if err != nil {
			fmt.Printf("Error sending message: %v\n", err)
			continue
		}

		_, err = stream.Write([]byte(msg))
		stream.Close()

		if err != nil {
			fmt.Printf("Error writing message: %v\n", err)
		}
	}
}
