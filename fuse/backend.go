package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/winfsp/cgofuse/fuse"
)

// Backend represents a filesystem backend (local disk, network, etc.)
type Backend interface {
	Stat(path string) (*fuse.Stat_t, int)
	Readdir(path string) ([]DirEnt, int)
	Read(path string, buff []byte, ofst int64) (int, int)
	Write(path string, buff []byte, ofst int64) (int, int)
	Truncate(path string, size int64) int
	Mkdir(path string, mode uint32) int
	Create(path string, mode uint32) int
	Unlink(path string) int
	Rmdir(path string) int
	Rename(oldpath, newpath string) int
}

// DirEnt represents a directory entry
type DirEnt struct {
	Name string
	Stat fuse.Stat_t
}

// LocalBackend implements Backend for local filesystem
type LocalBackend struct {
	root string // absolute base path (e.g., "D:/Videos")
}

// NewLocalBackend creates a new local backend for the given root directory
func NewLocalBackend(root string) *LocalBackend {
	return &LocalBackend{root: root}
}

// abs converts a mount-relative path to an absolute filesystem path
func (b *LocalBackend) abs(path string) string {
	// Remove leading slash and convert to OS path separators
	p := strings.TrimPrefix(path, "/")
	p = strings.ReplaceAll(p, "/", string(filepath.Separator))
	return filepath.Join(b.root, p)
}

// Stat returns file attributes
func (b *LocalBackend) Stat(path string) (*fuse.Stat_t, int) {
	ap := b.abs(path)
	info, err := os.Lstat(ap)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, -fuse.ENOENT
		}
		return nil, -fuse.EIO
	}

	st := &fuse.Stat_t{}
	mode := uint32(0644) | fuse.S_IFREG
	var nlink uint32 = 1
	if info.IsDir() {
		mode = uint32(0755) | fuse.S_IFDIR
		nlink = 2
	}

	st.Mode = mode
	st.Nlink = nlink
	st.Size = info.Size()
	st.Mtim = fuse.NewTimespec(info.ModTime())
	st.Atim = fuse.Now()
	st.Ctim = fuse.Now()

	return st, 0
}

// Readdir lists directory entries
func (b *LocalBackend) Readdir(path string) ([]DirEnt, int) {
	ap := b.abs(path)
	ents, err := os.ReadDir(ap)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, -fuse.ENOENT
		}
		return nil, -fuse.EIO
	}

	out := make([]DirEnt, 0, len(ents))
	for _, e := range ents {
		info, err := e.Info()
		if err != nil {
			continue
		}

		mode := uint32(0644) | fuse.S_IFREG
		var nlink uint32 = 1
		if info.IsDir() {
			mode = uint32(0755) | fuse.S_IFDIR
			nlink = 2
		}

		out = append(out, DirEnt{
			Name: e.Name(),
			Stat: fuse.Stat_t{
				Mode:  mode,
				Nlink: nlink,
				Size:  info.Size(),
				Mtim:  fuse.NewTimespec(info.ModTime()),
				Atim:  fuse.Now(),
				Ctim:  fuse.Now(),
			},
		})
	}

	return out, 0
}

// Read reads file content
func (b *LocalBackend) Read(path string, buff []byte, ofst int64) (int, int) {
	ap := b.abs(path)
	f, err := os.Open(ap)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, -fuse.ENOENT
		}
		return 0, -fuse.EIO
	}
	defer f.Close()

	n, err := f.ReadAt(buff, ofst)
	if err != nil && err != io.EOF {
		return 0, -fuse.EIO
	}

	return n, 0
}

// Write writes file content
func (b *LocalBackend) Write(path string, buff []byte, ofst int64) (int, int) {
	ap := b.abs(path)
	f, err := os.OpenFile(ap, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, -fuse.EIO
	}
	defer f.Close()

	n, err := f.WriteAt(buff, ofst)
	if err != nil {
		return 0, -fuse.EIO
	}

	return n, 0
}

// Truncate changes file size
func (b *LocalBackend) Truncate(path string, size int64) int {
	ap := b.abs(path)
	if err := os.Truncate(ap, size); err != nil {
		return -fuse.EIO
	}
	return 0
}

// Mkdir creates a directory
func (b *LocalBackend) Mkdir(path string, mode uint32) int {
	ap := b.abs(path)
	if err := os.Mkdir(ap, os.FileMode(mode)); err != nil {
		if os.IsExist(err) {
			return -fuse.EEXIST
		}
		return -fuse.EIO
	}
	return 0
}

// Create creates a file
func (b *LocalBackend) Create(path string, mode uint32) int {
	ap := b.abs(path)
	f, err := os.OpenFile(ap, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return -fuse.EIO
	}
	f.Close()
	return 0
}

// Unlink deletes a file
func (b *LocalBackend) Unlink(path string) int {
	ap := b.abs(path)
	if err := os.Remove(ap); err != nil {
		if os.IsNotExist(err) {
			return -fuse.ENOENT
		}
		return -fuse.EIO
	}
	return 0
}

// Rmdir removes a directory
func (b *LocalBackend) Rmdir(path string) int {
	ap := b.abs(path)
	if err := os.Remove(ap); err != nil {
		if os.IsNotExist(err) {
			return -fuse.ENOENT
		}
		return -fuse.EIO
	}
	return 0
}

// Rename moves or renames a file/directory
func (b *LocalBackend) Rename(oldpath, newpath string) int {
	apOld := b.abs(oldpath)
	apNew := b.abs(newpath)
	if err := os.Rename(apOld, apNew); err != nil {
		return -fuse.EIO
	}
	return 0
}
