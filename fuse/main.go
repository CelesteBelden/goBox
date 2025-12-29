package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/winfsp/cgofuse/fuse"
)

func main() {
	fs := NewMemFS()
	host := fuse.NewFileSystemHost(fs)

	// Graceful shutdown on Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		host.Unmount()
	}()

	fs.Mkdir("/testdir", 0777)
	fs.Create("/testdir/testfile.txt", 2, 0777)

	// Mount with command-line args (e.g., X:)
	host.Mount("", os.Args[1:])
}
