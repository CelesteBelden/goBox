package main

import (
	"strings"
	"sync"
	"time"

	"github.com/winfsp/cgofuse/fuse"
)

// node represents a file or directory in memory.
type node struct {
	stat fuse.Stat_t
	data []byte
}

// MemFS is an in-memory filesystem.
type MemFS struct {
	fuse.FileSystemBase
	lock  sync.Mutex
	nodes map[string]*node
}

// NewMemFS creates a new in-memory filesystem with a root directory.
func NewMemFS() *MemFS {
	fs := &MemFS{
		nodes: make(map[string]*node),
	}
	now := fuse.Now()
	fs.nodes["/"] = &node{
		stat: fuse.Stat_t{
			Mode:  fuse.S_IFDIR | 0755,
			Nlink: 2,
			Atim:  now,
			Mtim:  now,
			Ctim:  now,
		},
	}
	return fs
}

// split returns parent directory and base name.
func split(path string) (string, string) {
	path = strings.TrimSuffix(path, "/")
	i := strings.LastIndex(path, "/")
	if i == -1 {
		return "", path
	}
	if i == 0 {
		return "/", path[1:]
	}
	return path[:i], path[i+1:]
}

// Getattr gets file attributes.
func (fs *MemFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[path]
	if !ok {
		return -fuse.ENOENT
	}
	*stat = n.stat
	return 0
}

// Mkdir creates a directory.
func (fs *MemFS) Mkdir(path string, mode uint32) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	if _, ok := fs.nodes[path]; ok {
		return -fuse.EEXIST
	}

	parent, _ := split(path)
	pn, ok := fs.nodes[parent]
	if !ok {
		return -fuse.ENOENT
	}
	if pn.stat.Mode&fuse.S_IFDIR == 0 {
		return -fuse.ENOTDIR
	}

	now := fuse.Now()
	fs.nodes[path] = &node{
		stat: fuse.Stat_t{
			Mode:  fuse.S_IFDIR | mode,
			Nlink: 2,
			Atim:  now,
			Mtim:  now,
			Ctim:  now,
		},
	}
	pn.stat.Nlink++
	return 0
}

// Rmdir removes a directory.
func (fs *MemFS) Rmdir(path string) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[path]
	if !ok {
		return -fuse.ENOENT
	}
	if n.stat.Mode&fuse.S_IFDIR == 0 {
		return -fuse.ENOTDIR
	}

	// Check if directory is empty
	prefix := path
	if prefix != "/" {
		prefix += "/"
	}
	for p := range fs.nodes {
		if strings.HasPrefix(p, prefix) && p != path {
			return -fuse.ENOTEMPTY
		}
	}

	parent, _ := split(path)
	if pn, ok := fs.nodes[parent]; ok {
		pn.stat.Nlink--
	}
	delete(fs.nodes, path)
	return 0
}

// Mknod creates a file node.
func (fs *MemFS) Mknod(path string, mode uint32, dev uint64) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	if _, ok := fs.nodes[path]; ok {
		return -fuse.EEXIST
	}

	parent, _ := split(path)
	if _, ok := fs.nodes[parent]; !ok {
		return -fuse.ENOENT
	}

	now := fuse.Now()
	fs.nodes[path] = &node{
		stat: fuse.Stat_t{
			Mode:  fuse.S_IFREG | mode,
			Nlink: 1,
			Atim:  now,
			Mtim:  now,
			Ctim:  now,
		},
		data: []byte{},
	}
	return 0
}

// Unlink removes a file.
func (fs *MemFS) Unlink(path string) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[path]
	if !ok {
		return -fuse.ENOENT
	}
	if n.stat.Mode&fuse.S_IFDIR != 0 {
		return -fuse.EISDIR
	}
	delete(fs.nodes, path)
	return 0
}

// Rename moves/renames a file or directory.
func (fs *MemFS) Rename(oldpath string, newpath string) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[oldpath]
	if !ok {
		return -fuse.ENOENT
	}

	// Check new parent exists
	newParent, _ := split(newpath)
	if _, ok := fs.nodes[newParent]; !ok {
		return -fuse.ENOENT
	}

	// Remove existing target if any
	delete(fs.nodes, newpath)

	// Move node
	delete(fs.nodes, oldpath)
	fs.nodes[newpath] = n

	// If directory, update children paths
	if n.stat.Mode&fuse.S_IFDIR != 0 {
		oldPrefix := oldpath + "/"
		newPrefix := newpath + "/"
		for p, child := range fs.nodes {
			if strings.HasPrefix(p, oldPrefix) {
				newChildPath := newPrefix + strings.TrimPrefix(p, oldPrefix)
				delete(fs.nodes, p)
				fs.nodes[newChildPath] = child
			}
		}
	}

	return 0
}

// Open opens a file.
func (fs *MemFS) Open(path string, flags int) (int, uint64) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	_, ok := fs.nodes[path]
	if !ok {
		return -fuse.ENOENT, 0
	}
	return 0, 0
}

// Read reads data from a file.
func (fs *MemFS) Read(path string, buff []byte, ofst int64, fh uint64) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[path]
	if !ok {
		return -fuse.ENOENT
	}

	size := int64(len(n.data))
	if ofst >= size {
		return 0
	}

	end := ofst + int64(len(buff))
	if end > size {
		end = size
	}

	return copy(buff, n.data[ofst:end])
}

// Write writes data to a file.
func (fs *MemFS) Write(path string, buff []byte, ofst int64, fh uint64) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[path]
	if !ok {
		return -fuse.ENOENT
	}

	end := ofst + int64(len(buff))
	if end > int64(len(n.data)) {
		newData := make([]byte, end)
		copy(newData, n.data)
		n.data = newData
	}
	copy(n.data[ofst:], buff)

	n.stat.Size = int64(len(n.data))
	n.stat.Mtim = fuse.Now()
	return len(buff)
}

// Truncate changes the size of a file.
func (fs *MemFS) Truncate(path string, size int64, fh uint64) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[path]
	if !ok {
		return -fuse.ENOENT
	}

	if size < int64(len(n.data)) {
		n.data = n.data[:size]
	} else if size > int64(len(n.data)) {
		newData := make([]byte, size)
		copy(newData, n.data)
		n.data = newData
	}

	n.stat.Size = size
	n.stat.Mtim = fuse.Now()
	return 0
}

// Readdir reads directory entries.
func (fs *MemFS) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64, fh uint64) int {

	fs.lock.Lock()
	defer fs.lock.Unlock()

	if _, ok := fs.nodes[path]; !ok {
		return -fuse.ENOENT
	}

	fill(".", nil, 0)
	fill("..", nil, 0)

	prefix := path
	if prefix != "/" {
		prefix += "/"
	}

	for p, n := range fs.nodes {
		if p == path {
			continue
		}
		if strings.HasPrefix(p, prefix) {
			suffix := strings.TrimPrefix(p, prefix)
			// Only direct children (no nested paths)
			if !strings.Contains(suffix, "/") {
				fill(suffix, &n.stat, 0)
			}
		}
	}

	return 0
}

// Opendir opens a directory.
func (fs *MemFS) Opendir(path string) (int, uint64) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[path]
	if !ok {
		return -fuse.ENOENT, 0
	}
	if n.stat.Mode&fuse.S_IFDIR == 0 {
		return -fuse.ENOTDIR, 0
	}
	return 0, 0
}

// Utimens sets file access and modification times.
func (fs *MemFS) Utimens(path string, tmsp []fuse.Timespec) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[path]
	if !ok {
		return -fuse.ENOENT
	}

	if tmsp == nil {
		now := fuse.Now()
		n.stat.Atim = now
		n.stat.Mtim = now
	} else {
		n.stat.Atim = tmsp[0]
		n.stat.Mtim = tmsp[1]
	}
	return 0
}

// Create creates and opens a file.
func (fs *MemFS) Create(path string, flags int, mode uint32) (int, uint64) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	parent, _ := split(path)
	if _, ok := fs.nodes[parent]; !ok {
		return -fuse.ENOENT, 0
	}

	now := fuse.Now()
	fs.nodes[path] = &node{
		stat: fuse.Stat_t{
			Mode:  fuse.S_IFREG | mode,
			Nlink: 1,
			Atim:  now,
			Mtim:  now,
			Ctim:  now,
		},
		data: []byte{},
	}
	return 0, 0
}

// Statfs gets filesystem statistics.
func (fs *MemFS) Statfs(path string, stat *fuse.Statfs_t) int {
	stat.Bsize = 4096
	stat.Frsize = 4096
	stat.Blocks = 1000000
	stat.Bfree = 1000000
	stat.Bavail = 1000000
	stat.Files = 1000000
	stat.Ffree = 1000000
	stat.Namemax = 255
	return 0
}

// Chmod changes file mode.
func (fs *MemFS) Chmod(path string, mode uint32) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[path]
	if !ok {
		return -fuse.ENOENT
	}

	n.stat.Mode = (n.stat.Mode & fuse.S_IFMT) | mode
	n.stat.Ctim = fuse.Now()
	return 0
}

// Chown changes file owner/group.
func (fs *MemFS) Chown(path string, uid uint32, gid uint32) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[path]
	if !ok {
		return -fuse.ENOENT
	}

	if uid != ^uint32(0) {
		n.stat.Uid = uid
	}
	if gid != ^uint32(0) {
		n.stat.Gid = gid
	}
	n.stat.Ctim = fuse.Now()
	return 0
}

// unused import guard
var _ = time.Now
