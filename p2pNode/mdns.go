package main

import (
	"context"
	"log"
	"time"

	hostpkg "github.com/libp2p/go-libp2p/core/host"
	peerpkg "github.com/libp2p/go-libp2p/core/peer"
	mdns "github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

// mdnsNotifee gets notified when new peers are discovered via mDNS.
type mdnsNotifee struct {
	h hostpkg.Host
}

// HandlePeerFound is called when mDNS discovers a peer.
func (n *mdnsNotifee) HandlePeerFound(pi peerpkg.AddrInfo) {
	// ignore ourselves
	if pi.ID == n.h.ID() {
		return
	}

	log.Printf("mDNS discovered peer: %s", pi.ID.String())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := n.h.Connect(ctx, pi); err != nil {
		log.Printf("mDNS: failed to connect to discovered peer %s: %v", pi.ID.String(), err)
		return
	}

	connectedPeerID = pi.ID
	select {
	case peerIDSet <- true:
	default:
	}
}

// startMDNS registers the mDNS service and notifee. Service tag should be unique-ish.
func startMDNS(h hostpkg.Host, serviceTag string) error {
	n := &mdnsNotifee{h: h}
	// NewMdnsService registers the notifee and starts the service.
	_ = mdns.NewMdnsService(h, serviceTag, n)
	return nil
}
