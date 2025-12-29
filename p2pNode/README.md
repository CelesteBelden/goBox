# GoBox P2P Node

A peer-to-peer chat node built with [libp2p](https://libp2p.io/), supporting direct connections and automatic LAN discovery via mDNS.

## Features

- **Direct P2P connections** via multiaddress
- **Automatic LAN discovery** using mDNS
- **Simple chat protocol** for real-time messaging
- **Handshake protocol** for connection establishment

## Prerequisites

- Go 1.21+
- libp2p dependencies (automatically fetched via `go mod`)

## Building & Running

```bash
# Build
cd p2pNode
go build

# Run as listener (wait for incoming connections)
./p2pNode

# Run and connect to a peer
./p2pNode <multiaddress>
```

### Example

**Terminal 1 (Listener):**
```bash
./p2pNode
# Output:
# Node started successfully!
# Peer ID: 12D3KooW...
# Listening on:
#   /ip4/192.168.1.100/tcp/12345/p2p/12D3KooW...
#
# No peer address provided. Waiting for incoming connection...
```

**Terminal 2 (Connector):**
```bash
./p2pNode /ip4/192.168.1.100/tcp/12345/p2p/12D3KooW...
# Output:
# Attempting to connect to: /ip4/192.168.1.100/tcp/12345/p2p/12D3KooW...
# Successfully connected to peer!
# Response: Hello! Ready to chat.
# Type your messages (Ctrl+C to exit):
```

---

## Project Structure

| File | Description |
|------|-------------|
| [main.go](main.go) | Entry point - creates node, sets up handlers, starts mDNS |
| [node.go](node.go) | Node creation and configuration |
| [config.go](config.go) | Protocol IDs and global state |
| [connection.go](connection.go) | Peer connection logic |
| [handlers.go](handlers.go) | Stream handlers for hello/chat protocols |
| [messages.go](messages.go) | Message sending logic |
| [mdns.go](mdns.go) | mDNS discovery for LAN peers |

---

## Function Reference

### Node Creation & Setup

| Function | File | Description |
|----------|------|-------------|
| `createNode()` | node.go | Creates a new libp2p host with default settings |
| `setupStreamHandlers(node)` | node.go | Registers protocol handlers on the node |
| `printNodeAddress(node)` | node.go | Prints the node's peer ID and multiaddress |

### Connection Management

| Function | File | Description |
|----------|------|-------------|
| `connectToPeer(node, peerAddr)` | connection.go | Connects to a peer via multiaddress, sends hello |
| `waitForConnection()` | connection.go | Displays waiting message for incoming connections |

### Stream Handlers

| Function | File | Description |
|----------|------|-------------|
| `handleHelloStream(stream)` | handlers.go | Handles initial handshake - receives hello, sends ack |
| `handleChatStream(stream)` | handlers.go | Handles incoming chat messages, prints to console |

### Messaging

| Function | File | Description |
|----------|------|-------------|
| `sendMessages(node)` | messages.go | Reads stdin and sends messages to connected peer |

### Discovery

| Function | File | Description |
|----------|------|-------------|
| `startMDNS(host, serviceTag)` | mdns.go | Starts mDNS service for LAN peer discovery |
| `HandlePeerFound(peerInfo)` | mdns.go | Callback when mDNS discovers a peer - auto-connects |

---

## Protocols

| Protocol ID | Purpose | Description |
|-------------|---------|-------------|
| `/hello/1.0.0` | Handshake | Initial connection establishment and acknowledgment |
| `/chat/1.0.0` | Chat | Real-time message exchange |

---

## Multiaddress Format

Multiaddresses uniquely identify a peer on the network:

```
/ip4/<IP>/tcp/<PORT>/p2p/<PEER_ID>
```

| Component | Example | Description |
|-----------|---------|-------------|
| `/ip4/` | `192.168.1.100` | IPv4 address |
| `/ip6/` | `::1` | IPv6 address |
| `/tcp/` | `12345` | TCP port |
| `/p2p/` | `12D3KooW...` | Peer ID (public key hash) |

**Examples:**
```
/ip4/192.168.1.100/tcp/4001/p2p/12D3KooWAbCdEf...
/ip4/127.0.0.1/tcp/9000/p2p/12D3KooWXyZ123...
/ip6/::1/tcp/4001/p2p/12D3KooWAbCdEf...
```

---

## mDNS Discovery

The node automatically discovers other GoBox nodes on the local network using mDNS (Multicast DNS). All nodes advertising the same service tag (`gobox-mdns`) will automatically find and connect to each other.

**How it works:**
1. Node starts and registers mDNS service with tag `gobox-mdns`
2. mDNS broadcasts presence on the LAN
3. Other nodes with the same tag are discovered
4. `HandlePeerFound` is called, which auto-connects to the peer

**Note:** mDNS only works on local networks. For internet connections, use direct multiaddress.

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                        main.go                          │
│  - Create node                                          │
│  - Setup handlers                                       │
│  - Start mDNS                                           │
│  - Connect to peer OR wait for connection               │
└─────────────────────────────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        ▼                   ▼                   ▼
┌───────────────┐   ┌───────────────┐   ┌───────────────┐
│   node.go     │   │  handlers.go  │   │   mdns.go     │
│ createNode()  │   │ handleHello() │   │ startMDNS()   │
│ setupHandlers │   │ handleChat()  │   │ HandlePeer()  │
└───────────────┘   └───────────────┘   └───────────────┘
        │                   ▲                   │
        ▼                   │                   ▼
┌───────────────┐   ┌───────────────┐   ┌───────────────┐
│ connection.go │   │  messages.go  │   │  config.go    │
│ connectToPeer │◄──│ sendMessages  │   │ Protocol IDs  │
│ waitForConn   │   │ (stdin loop)  │   │ Global state  │
└───────────────┘   └───────────────┘   └───────────────┘
```

---

## Dependencies

- `github.com/libp2p/go-libp2p` - Core libp2p library
- `github.com/libp2p/go-libp2p/core/host` - Host interface
- `github.com/libp2p/go-libp2p/core/peer` - Peer ID handling
- `github.com/libp2p/go-libp2p/core/network` - Network streams
- `github.com/libp2p/go-libp2p/core/protocol` - Protocol IDs
- `github.com/libp2p/go-libp2p/p2p/discovery/mdns` - mDNS discovery
- `github.com/multiformats/go-multiaddr` - Multiaddress parsing
