# GoBox FUSE Filesystem

An in-memory filesystem implementation using [cgofuse](https://github.com/winfsp/cgofuse) for Windows (WinFsp) and Unix (libfuse).

## Prerequisites

- **Windows:** Install [WinFsp](https://winfsp.dev/)
- **Linux/macOS:** Install libfuse (`apt install libfuse-dev` or `brew install macfuse`)

## Building & Running

```bash
# Build
cd fuse
go build

# Run (Windows - mount as X: drive)
fuse.exe X:

# Run (Linux/macOS)
./fuse /mnt/gobox

# Stop
Ctrl+C
```

The HTTP REST API server runs on **localhost:8080** alongside the FUSE mount, allowing you to manage the filesystem programmatically.

---

## REST API Endpoints

The FUSE filesystem exposes a full REST API for filesystem operations:

### Backend Linking

| Endpoint | Method | Description | Body |
|----------|--------|-------------|------|
| `/api/link/local` | POST | Link a real filesystem directory into the FUSE mount | `{"path": "/mount/point", "target": "/real/path"}` |

### Metadata Operations

| Endpoint | Method | Description | Query |
|----------|--------|-------------|-------|
| `/api/getattr` | GET | Get file/directory attributes | `path` |
| `/api/chmod` | POST | Change file permissions | Body: `{"path", "mode"}` |
| `/api/chown` | POST | Change file owner | Body: `{"path", "uid", "gid"}` |
| `/api/statfs` | GET | Get filesystem stats | `path` |

### Directory Operations

| Endpoint | Method | Description | Body/Query |
|----------|--------|-------------|-----------|
| `/api/mkdir` | POST | Create directory | `{"path", "mode"}` |
| `/api/rmdir` | DELETE | Remove directory | `path` query param |
| `/api/opendir` | POST | Open directory | `{"path"}` |
| `/api/readdir` | GET | List directory contents | `path` query param |
| `/api/readdir/paginated` | GET | List with pagination | `path`, `limit`, `offset` query params |

### File Operations

| Endpoint | Method | Description | Body/Query |
|----------|--------|-------------|-----------|
| `/api/create` | POST | Create file | `{"path", "mode"}` |
| `/api/unlink` | DELETE | Delete file | `path` query param |
| `/api/truncate` | POST | Resize file | `{"path", "size"}` |
| `/api/rename` | POST | Move/rename file | `{"oldpath", "newpath"}` |

### Binary File I/O

| Endpoint | Method | Description | Query |
|----------|--------|-------------|-------|
| `/api/files/read` | GET | Read binary data | `path`, `offset` |
| `/api/files/write` | POST | Write binary data | `path`, `offset`; raw body is file content |

---

## Backend Linking

Link real filesystem directories into your FUSE mount to provide access to external files:

```bash
# Link a directory
curl -X POST http://localhost:8080/api/link/local \
  -H "Content-Type: application/json" \
  -d '{"path": "/media", "target": "C:/Users/Downloads"}'

# Create a file in the linked directory
curl -X POST http://localhost:8080/api/create \
  -H "Content-Type: application/json" \
  -d '{"path": "/media/newfile.txt", "mode": 420}'

# Write data to the file
curl -X POST http://localhost:8080/api/files/write?path=/media/newfile.txt&offset=0 \
  --data "Hello from linked backend!"

# List linked directory
curl http://localhost:8080/api/readdir?path=/media
```

All operations on linked paths are delegated to the real filesystem. Nested backends are resolved by walking up the path tree to find the nearest parent with a backend.

---

## MemFS Function Reference

### Core Types

| Type | Description |
|------|-------------|
| `node` | Represents a file or directory in memory. Contains `stat` (metadata) and `data` (file contents). |
| `MemFS` | The main filesystem struct. Embeds `fuse.FileSystemBase` and stores all nodes in a map. |

### Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `NewMemFS` | `func NewMemFS() *MemFS` | Creates a new in-memory filesystem with an empty root directory (`/`). |
| `split` | `func split(path string) (string, string)` | Helper that splits a path into parent directory and base name. |

### Filesystem Methods

#### File/Directory Attributes

| Method | Signature | Description |
|--------|-----------|-------------|
| `Getattr` | `(path string, stat *fuse.Stat_t, fh uint64) int` | Gets file/directory attributes (size, mode, timestamps). Called by `stat`, `ls`, etc. |
| `Chmod` | `(path string, mode uint32) int` | Changes file permissions. |
| `Chown` | `(path string, uid uint32, gid uint32) int` | Changes file owner and group. |
| `Utimens` | `(path string, tmsp []fuse.Timespec) int` | Sets access and modification timestamps. |
| `Statfs` | `(path string, stat *fuse.Statfs_t) int` | Returns filesystem statistics (total space, free space, etc.). |

#### Directory Operations

| Method | Signature | Description |
|--------|-----------|-------------|
| `Mkdir` | `(path string, mode uint32) int` | Creates a new directory with the specified permissions. |
| `Rmdir` | `(path string) int` | Removes an empty directory. Returns error if not empty. |
| `Opendir` | `(path string) (int, uint64)` | Opens a directory for reading. Returns error code and file handle. |
| `Readdir` | `(path string, fill func(...), ofst int64, fh uint64) int` | Lists directory contents. The `fill` callback is called for each entry. |

#### File Operations

| Method | Signature | Description |
|--------|-----------|-------------|
| `Create` | `(path string, flags int, mode uint32) (int, uint64)` | Creates and opens a new file. Returns error code and file handle. |
| `Mknod` | `(path string, mode uint32, dev uint64) int` | Creates a file node (lower-level than Create). |
| `Open` | `(path string, flags int) (int, uint64)` | Opens an existing file. Returns error code and file handle. |
| `Read` | `(path string, buff []byte, ofst int64, fh uint64) int` | Reads data from a file at the given offset. Returns bytes read. |
| `Write` | `(path string, buff []byte, ofst int64, fh uint64) int` | Writes data to a file at the given offset. Returns bytes written. |
| `Truncate` | `(path string, size int64, fh uint64) int` | Resizes a file to the specified size. |
| `Unlink` | `(path string) int` | Deletes a file. |
| `Rename` | `(oldpath string, newpath string) int` | Moves or renames a file/directory. |

#### Return Values

All methods return `0` on success or a negative error code on failure:

| Error Code | Constant | Meaning |
|------------|----------|---------|
| `-2` | `ENOENT` | File or directory not found |
| `-17` | `EEXIST` | File already exists |
| `-20` | `ENOTDIR` | Not a directory |
| `-21` | `EISDIR` | Is a directory (when file expected) |
| `-39` | `ENOTEMPTY` | Directory not empty |

---

## Mode & Flag Reference

### File Permissions (Mode)

Modes are **octal numbers** representing read/write/execute permissions for owner, group, and others.

```
0 7 7 7
│ │ │ └── Others (everyone else)
│ │ └──── Group  
│ └────── Owner
└──────── Octal prefix
```

Each digit is a sum of:

| Value | Permission | Symbol |
|-------|------------|--------|
| 4 | Read | `r` |
| 2 | Write | `w` |
| 1 | Execute | `x` |

#### Common Permission Modes

| Mode | Binary | Symbolic | Description |
|------|--------|----------|-------------|
| `0777` | `rwxrwxrwx` | Full access | Everyone can read, write, execute |
| `0755` | `rwxr-xr-x` | Standard directory | Owner full, others read/execute |
| `0666` | `rw-rw-rw-` | Public file | Everyone can read/write |
| `0644` | `rw-r--r--` | Standard file | Owner read/write, others read-only |
| `0600` | `rw-------` | Private file | Owner read/write only |
| `0400` | `r--------` | Read-only | Owner read only |

### File Type Flags

Combined with permissions in the mode field:

| Flag | Description |
|------|-------------|
| `fuse.S_IFREG` | Regular file |
| `fuse.S_IFDIR` | Directory |
| `fuse.S_IFLNK` | Symbolic link |
| `fuse.S_IFMT` | Bitmask for file type |

**Example:** `fuse.S_IFREG | 0644` = regular file with rw-r--r-- permissions

### Open Flags

Control how files are opened:

| Flag | Value | Description |
|------|-------|-------------|
| `fuse.O_RDONLY` | 0 | Open for reading only |
| `fuse.O_WRONLY` | 1 | Open for writing only |
| `fuse.O_RDWR` | 2 | Open for reading and writing |
| `fuse.O_CREAT` | — | Create file if it doesn't exist |
| `fuse.O_TRUNC` | — | Truncate file to zero length |
| `fuse.O_APPEND` | — | Append to end of file |
| `fuse.O_EXCL` | — | Fail if file exists (with O_CREAT) |

**Combine with `|`:**
```go
fuse.O_CREAT | fuse.O_WRONLY | fuse.O_TRUNC  // Create/overwrite for writing
fuse.O_RDWR | fuse.O_APPEND                   // Read/write, append mode
```

### Access Check Flags

Used with the `Access` method:

| Flag | Value | Description |
|------|-------|-------------|
| `fuse.F_OK` | 0 | Check if file exists |
| `fuse.R_OK` | 4 | Check read permission |
| `fuse.W_OK` | 2 | Check write permission |
| `fuse.X_OK` | 1 | Check execute permission |

---

## Usage Examples

### Creating Files and Directories

```go
fs := NewMemFS()

// Create a directory (rwxr-xr-x)
fs.Mkdir("/mydir", 0755)

// Create a file (rw-rw-rw-)
fs.Create("/mydir/file.txt", 0, 0666)

// Write content
fs.Write("/mydir/file.txt", []byte("Hello, World!"), 0, 0)

// Mount the filesystem
host := fuse.NewFileSystemHost(fs)
host.Mount("", []string{"X:"})
```

### Reading Files

```go
// Read into buffer
buff := make([]byte, 1024)
bytesRead := fs.Read("/mydir/file.txt", buff, 0, 0)
content := string(buff[:bytesRead])
```
