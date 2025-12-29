package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/winfsp/cgofuse/fuse"
)

func main() {
	fs := NewMemFS()
	host := fuse.NewFileSystemHost(fs)

	// Create API server
	api := NewAPIServer(fs)
	api.RegisterRoutes()

	// Graceful shutdown on Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		host.Unmount()
		os.Exit(0)
	}()

	// Start HTTP API server in a goroutine
	go func() {
		fmt.Println("Starting API server on http://localhost:8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal(err)
		}
	}()

	// Mount with command-line args (e.g., X:) - this blocks
	host.Mount("", os.Args[1:])
}
