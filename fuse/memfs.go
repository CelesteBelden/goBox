package main

import (
	"strings"
	"sync"
	"time"

	"github.com/winfsp/cgofuse/fuse"
)

// node represents a file or directory in memory or backed by a filesystem.
type node struct {
	stat        fuse.Stat_t
	data        []byte
	backend     Backend // nil if in-memory
	backendPath string  // mount-relative path under backend
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

// resolveBackend finds the nearest ancestor node with a backend and returns the backend and relative path.
// Returns (nil, path) if no backend is found in ancestors.
func (fs *MemFS) resolveBackend(path string) (Backend, string) {
	current := path
	for {
		if n, ok := fs.nodes[current]; ok && n.backend != nil {
			// Found a backend node; compute relative path
			relPath := strings.TrimPrefix(path, current)
			if relPath == "" {
				relPath = "/"
			}
			return n.backend, relPath
		}

		if current == "/" {
			break
		}
		// Move to parent
		current, _ = split(current)
		if current == "" {
			current = "/"
		}
	}
	return nil, path
}

// LinkLocal mounts a real folder/file at a mount path.
func (fs *MemFS) LinkLocal(mountPath string, targetRoot string) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	// Check if path already exists
	if _, ok := fs.nodes[mountPath]; ok {
		return -fuse.EEXIST
	}

	// Check parent exists and is a directory
	parent, _ := split(mountPath)
	if parent == "" {
		parent = "/"
	}
	pn, ok := fs.nodes[parent]
	if !ok {
		return -fuse.ENOENT
	}
	if pn.stat.Mode&fuse.S_IFDIR == 0 {
		return -fuse.ENOTDIR
	}

	// Create backend node
	lb := NewLocalBackend(targetRoot)
	now := fuse.Now()
	fs.nodes[mountPath] = &node{
		stat: fuse.Stat_t{
			Mode:  fuse.S_IFDIR | 0755,
			Nlink: 2,
			Atim:  now,
			Mtim:  now,
			Ctim:  now,
		},
		backend:     lb,
		backendPath: "/",
	}

	// Increment parent link count
	pn.stat.Nlink++

	return 0
}

// Getattr gets file attributes.
func (fs *MemFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[path]
	if !ok {
		// Try to resolve via backend
		backend, relPath := fs.resolveBackend(path)
		if backend != nil {
			st, err := backend.Stat(relPath)
			if err == 0 {
				*stat = *st
				return 0
			}
			return err
		}
		return -fuse.ENOENT
	}

	// If this node has a backend, stat through the backend
	if n.backend != nil {
		st, err := n.backend.Stat(n.backendPath)
		if err == 0 {
			*stat = *st
			return 0
		}
		return err
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

	parent, basename := split(path)
	if parent == "" {
		parent = "/"
	}
	pn, ok := fs.nodes[parent]
	if !ok {
		// Try to resolve parent via backend
		backend, relPath := fs.resolveBackend(parent)
		if backend != nil {
			// Create in backend
			err := backend.Mkdir(relPath, mode)
			return err
		}
		return -fuse.ENOENT
	}
	if pn.stat.Mode&fuse.S_IFDIR == 0 {
		return -fuse.ENOTDIR
	}

	// Check if parent is backed; if so, create through backend
	if pn.backend != nil {
		// The relative path is just the basename since parent is the backend node
		relPath := "/" + basename
		err := pn.backend.Mkdir(relPath, mode)
		return err
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

	// Cannot remove root
	if path == "/" {
		return -fuse.ENOENT
	}

	n, ok := fs.nodes[path]
	if !ok {
		// Try to resolve via backend
		backend, relPath := fs.resolveBackend(path)
		if backend != nil {
			err := backend.Rmdir(relPath)
			return err
		}
		return -fuse.ENOENT
	}
	if n.stat.Mode&fuse.S_IFDIR == 0 {
		return -fuse.ENOTDIR
	}

	// Check if directory has a backend; if so, remove through backend
	if n.backend != nil {
		err := n.backend.Rmdir(n.backendPath)
		if err != 0 {
			return err
		}
		// Also remove from in-memory nodes
		parent, _ := split(path)
		if parent == "" {
			parent = "/"
		}
		if pn, ok := fs.nodes[parent]; ok {
			pn.stat.Nlink--
		}
		delete(fs.nodes, path)
		return 0
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
	if parent == "" {
		parent = "/"
	}
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
		// Try to resolve via backend
		backend, relPath := fs.resolveBackend(path)
		if backend != nil {
			err := backend.Unlink(relPath)
			return err
		}
		return -fuse.ENOENT
	}
	if n.stat.Mode&fuse.S_IFDIR != 0 {
		return -fuse.EISDIR
	}

	// If node has a backend, delete through it
	if n.backend != nil {
		err := n.backend.Unlink(n.backendPath)
		if err != 0 {
			return err
		}
		delete(fs.nodes, path)
		return 0
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
		// Try to resolve via backend
		backend, relPath := fs.resolveBackend(oldpath)
		if backend != nil {
			newBackend, newRelPath := fs.resolveBackend(newpath)
			// Can only rename within same backend
			if backend != newBackend {
				return -fuse.EIO
			}
			err := backend.Rename(relPath, newRelPath)
			return err
		}
		return -fuse.ENOENT
	}

	// Check new parent exists
	newParent, _ := split(newpath)
	if newParent == "" {
		newParent = "/"
	}
	if _, ok := fs.nodes[newParent]; !ok {
		return -fuse.ENOENT
	}

	// If node has a backend, rename through it
	if n.backend != nil {
		err := n.backend.Rename(n.backendPath, newpath)
		if err != 0 {
			return err
		}
		delete(fs.nodes, oldpath)
		fs.nodes[newpath] = n
		return 0
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

	n, ok := fs.nodes[path]
	if !ok {
		// Try to resolve via backend
		backend, relPath := fs.resolveBackend(path)
		if backend != nil {
			// Check if it's a file by calling Stat
			stat, err := backend.Stat(relPath)
			if err != 0 {
				return err, 0
			}
			if stat.Mode&fuse.S_IFDIR != 0 {
				return -fuse.EISDIR, 0
			}
			return 0, 0
		}
		return -fuse.ENOENT, 0
	}
	if n.stat.Mode&fuse.S_IFDIR != 0 {
		return -fuse.EISDIR, 0
	}
	return 0, 0
}

// Read reads data from a file.
func (fs *MemFS) Read(path string, buff []byte, ofst int64, fh uint64) int {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	n, ok := fs.nodes[path]
	if !ok {
		// Try to resolve via backend
		backend, relPath := fs.resolveBackend(path)
		if backend != nil {
			bytesRead, err := backend.Read(relPath, buff, ofst)
			if err != 0 {
				return err
			}
			return bytesRead
		}
		return -fuse.ENOENT
	}
	if n.stat.Mode&fuse.S_IFDIR != 0 {
		return -fuse.EISDIR
	}

	// If node has a backend, read through it
	if n.backend != nil {
		bytesRead, err := n.backend.Read(n.backendPath, buff, ofst)
		if err != 0 {
			return err
		}
		return bytesRead
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
		// Try to resolve via backend
		backend, relPath := fs.resolveBackend(path)
		if backend != nil {
			bytesWritten, err := backend.Write(relPath, buff, ofst)
			if err != 0 {
				return err
			}
			return bytesWritten
		}
		return -fuse.ENOENT
	}
	if n.stat.Mode&fuse.S_IFDIR != 0 {
		return -fuse.EISDIR
	}

	// If node has a backend, write through it
	if n.backend != nil {
		bytesWritten, err := n.backend.Write(n.backendPath, buff, ofst)
		if err != 0 {
			return err
		}
		return bytesWritten
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
		// Try to resolve via backend
		backend, relPath := fs.resolveBackend(path)
		if backend != nil {
			err := backend.Truncate(relPath, size)
			if err != 0 {
				return err
			}
			return 0
		}
		return -fuse.ENOENT
	}
	if n.stat.Mode&fuse.S_IFDIR != 0 {
		return -fuse.EISDIR
	}

	// If node has a backend, truncate through it
	if n.backend != nil {
		err := n.backend.Truncate(n.backendPath, size)
		if err != 0 {
			return err
		}
		return 0
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

	// Check if path exists in nodes first
	n, ok := fs.nodes[path]
	if ok && n.backend != nil {
		// This is a backend node itself; use its backend
		ents, err := n.backend.Readdir(n.backendPath)
		if err != 0 {
			return err
		}
		fill(".", nil, 0)
		fill("..", nil, 0)
		for _, e := range ents {
			// Skip Windows system files
			if e.Name == "desktop.ini" || e.Name == "thumbs.db" {
				continue
			}
			fill(e.Name, &e.Stat, 0)
		}
		return 0
	}

	// Check if this path is under a backend in an ancestor
	backend, relPath := fs.resolveBackend(path)
	if backend != nil && !ok {
		// This path is under a backend (not a node itself); use backend's Readdir
		ents, err := backend.Readdir(relPath)
		if err != 0 {
			return err
		}
		fill(".", nil, 0)
		fill("..", nil, 0)
		for _, e := range ents {
			// Skip Windows system files
			if e.Name == "desktop.ini" || e.Name == "thumbs.db" {
				continue
			}
			fill(e.Name, &e.Stat, 0)
		}
		return 0
	}

	// In-memory path
	if !ok {
		return -fuse.ENOENT
	}
	if n.stat.Mode&fuse.S_IFDIR == 0 {
		return -fuse.ENOTDIR
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
		// Try to resolve via backend
		backend, relPath := fs.resolveBackend(path)
		if backend != nil {
			// Check if it's a directory by calling Stat
			stat, err := backend.Stat(relPath)
			if err != 0 {
				return err, 0
			}
			if stat.Mode&fuse.S_IFDIR == 0 {
				return -fuse.ENOTDIR, 0
			}
			return 0, 0
		}
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

	parent, basename := split(path)
	if parent == "" {
		parent = "/"
	}
	pn, ok := fs.nodes[parent]
	if !ok {
		// Try to resolve via backend
		backend, relPath := fs.resolveBackend(path)
		if backend != nil {
			err := backend.Create(relPath, mode)
			if err != 0 {
				return err, 0
			}
			return 0, 0
		}
		return -fuse.ENOENT, 0
	}

	// Check if parent is backed; if so, create through backend
	if pn.backend != nil {
		// The relative path is just the basename since parent is the backend node
		relPath := "/" + basename
		err := pn.backend.Create(relPath, mode)
		if err != 0 {
			return err, 0
		}
		return 0, 0
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
