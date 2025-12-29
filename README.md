# goBox

A Go-based project combining FUSE filesystem implementation with a REST API and P2P networking capabilities.

## Project Structure

### `/fuse`
In-memory FUSE filesystem with HTTP REST API layer for remote filesystem operations.

**Features:**
- Mount virtual drives on Windows
- Full filesystem operations (create, read, write, delete files/folders)
- REST API for programmatic access
- Server-side file handle management

See [fuse/README.md](fuse/README.md) for details.

### `/p2pNode`
Peer-to-peer networking node implementation.

See [p2pNode/README.md](p2pNode/README.md) for details.

## Getting Started

### Prerequisites
- Go 1.21 or higher
- WinFsp (for FUSE on Windows)

### Installation

```bash
git clone <repository-url>
cd goBox
go mod download
```

### Running the FUSE Filesystem

```bash
cd fuse
go run . X:
```

The REST API will be available at `http://localhost:8080`

### Running the P2P Node

```bash
cd p2pNode
go run .
```

## API Testing

Import the Postman collection from the FUSE documentation to test all REST endpoints.

## License

[Your License Here]
