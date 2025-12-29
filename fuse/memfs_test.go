package main

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/winfsp/cgofuse/fuse"
)

// Test helpers

func newTestFS() *MemFS {
	return NewMemFS()
}

func assertSuccess(t *testing.T, errCode int, operation string) {
	t.Helper()
	if errCode != 0 {
		t.Errorf("%s failed with error code %d, expected 0", operation, errCode)
	}
}

func assertError(t *testing.T, errCode int, expectedError int, operation string) {
	t.Helper()
	if errCode != expectedError {
		t.Errorf("%s returned error code %d, expected %d", operation, errCode, expectedError)
	}
}

func assertStatMode(t *testing.T, stat *fuse.Stat_t, expectedMode uint32, path string) {
	t.Helper()
	if stat.Mode != expectedMode {
		t.Errorf("stat.Mode for %s = 0x%x, expected 0x%x", path, stat.Mode, expectedMode)
	}
}

func assertStatSize(t *testing.T, stat *fuse.Stat_t, expectedSize int64, path string) {
	t.Helper()
	if stat.Size != expectedSize {
		t.Errorf("stat.Size for %s = %d, expected %d", path, stat.Size, expectedSize)
	}
}

func assertTimeWithTolerance(t *testing.T, actual fuse.Timespec, label string, toleranceSec int64) {
	t.Helper()
	now := fuse.Now()
	diff := now.Sec - actual.Sec
	if diff < 0 {
		diff = -diff
	}
	if diff > toleranceSec {
		t.Errorf("%s timestamp difference %d seconds exceeds tolerance %d seconds", label, diff, toleranceSec)
	}
}

// Initialization and utility tests

func TestNewMemFS(t *testing.T) {
	fs := NewMemFS()

	// Check root exists
	var stat fuse.Stat_t
	errCode := fs.Getattr("/", &stat, 0)
	assertSuccess(t, errCode, "Getattr on root")

	// Check root is a directory
	if stat.Mode&fuse.S_IFMT != fuse.S_IFDIR {
		t.Errorf("root is not a directory, mode = 0x%x", stat.Mode)
	}

	// Check root has correct permissions
	expectedMode := uint32(fuse.S_IFDIR | 0755)
	assertStatMode(t, &stat, expectedMode, "/")

	// Check timestamps are recent (within 2 seconds)
	assertTimeWithTolerance(t, stat.Atim, "root access time", 2)
	assertTimeWithTolerance(t, stat.Mtim, "root modification time", 2)
	assertTimeWithTolerance(t, stat.Ctim, "root change time", 2)
}

func TestSplit(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantDir  string
		wantBase string
	}{
		{"root", "/", "", ""},
		{"simple file", "/file", "/", "file"},
		{"simple dir", "/dir/", "/", "dir"},
		{"nested file", "/a/b/c", "/a/b", "c"},
		{"nested dir", "/a/b/c/", "/a/b", "c"},
		{"two levels", "/a/b", "/a", "b"},
		{"deep nested", "/a/b/c/d/e", "/a/b/c/d", "e"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDir, gotBase := split(tt.path)
			if gotDir != tt.wantDir || gotBase != tt.wantBase {
				t.Errorf("split(%q) = (%q, %q), want (%q, %q)",
					tt.path, gotDir, gotBase, tt.wantDir, tt.wantBase)
			}
		})
	}
}

// Directory operation tests

func TestMkdir(t *testing.T) {
	fs := newTestFS()

	// Create simple directory
	errCode := fs.Mkdir("/testdir", 0755)
	assertSuccess(t, errCode, "Mkdir /testdir")

	// Verify directory exists and has correct attributes
	var stat fuse.Stat_t
	errCode = fs.Getattr("/testdir", &stat, 0)
	assertSuccess(t, errCode, "Getattr /testdir")

	expectedMode := uint32(fuse.S_IFDIR | 0755)
	assertStatMode(t, &stat, expectedMode, "/testdir")

	// Create nested directory
	errCode = fs.Mkdir("/testdir/subdir", 0700)
	assertSuccess(t, errCode, "Mkdir /testdir/subdir")

	errCode = fs.Getattr("/testdir/subdir", &stat, 0)
	assertSuccess(t, errCode, "Getattr /testdir/subdir")

	expectedMode = uint32(fuse.S_IFDIR | 0700)
	assertStatMode(t, &stat, expectedMode, "/testdir/subdir")

	// Try to create directory that already exists
	errCode = fs.Mkdir("/testdir", 0755)
	assertError(t, errCode, -fuse.EEXIST, "Mkdir existing directory")

	// Try to create directory with non-existent parent
	errCode = fs.Mkdir("/nonexistent/newdir", 0755)
	assertError(t, errCode, -fuse.ENOENT, "Mkdir with missing parent")
}

func TestRmdir(t *testing.T) {
	fs := newTestFS()

	// Create and remove empty directory
	fs.Mkdir("/emptydir", 0755)
	errCode := fs.Rmdir("/emptydir")
	assertSuccess(t, errCode, "Rmdir /emptydir")

	// Verify directory no longer exists
	var stat fuse.Stat_t
	errCode = fs.Getattr("/emptydir", &stat, 0)
	assertError(t, errCode, -fuse.ENOENT, "Getattr on removed directory")

	// Try to remove non-existent directory
	errCode = fs.Rmdir("/nonexistent")
	assertError(t, errCode, -fuse.ENOENT, "Rmdir non-existent")

	// Try to remove non-empty directory
	fs.Mkdir("/parentdir", 0755)
	fs.Mkdir("/parentdir/childdir", 0755)
	errCode = fs.Rmdir("/parentdir")
	assertError(t, errCode, -fuse.ENOTEMPTY, "Rmdir non-empty directory")

	// Try to remove root
	errCode = fs.Rmdir("/")
	assertError(t, errCode, -fuse.ENOENT, "Rmdir root")

	// Try to remove a file as directory
	fs.Mknod("/testfile", fuse.S_IFREG|0644, 0)
	errCode = fs.Rmdir("/testfile")
	assertError(t, errCode, -fuse.ENOTDIR, "Rmdir on file")
}

func TestOpendir(t *testing.T) {
	fs := newTestFS()

	// Open root directory
	errCode, fh := fs.Opendir("/")
	assertSuccess(t, errCode, "Opendir /")
	if fh != 0 {
		t.Errorf("Opendir returned file handle %d, expected 0", fh)
	}

	// Create and open a directory
	fs.Mkdir("/testdir", 0755)
	errCode, fh = fs.Opendir("/testdir")
	assertSuccess(t, errCode, "Opendir /testdir")

	// Try to open non-existent directory
	errCode, _ = fs.Opendir("/nonexistent")
	assertError(t, errCode, -fuse.ENOENT, "Opendir non-existent")

	// Try to open a file as directory
	fs.Mknod("/testfile", fuse.S_IFREG|0644, 0)
	errCode, _ = fs.Opendir("/testfile")
	assertError(t, errCode, -fuse.ENOTDIR, "Opendir on file")
}

func TestReaddir(t *testing.T) {
	fs := newTestFS()

	// Create test structure
	fs.Mkdir("/dir1", 0755)
	fs.Mkdir("/dir2", 0755)
	fs.Mknod("/file1", fuse.S_IFREG|0644, 0)
	fs.Mknod("/file2", fuse.S_IFREG|0644, 0)

	// Read root directory
	entries := make([]string, 0)
	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool {
		entries = append(entries, name)
		return true
	}

	errCode := fs.Readdir("/", fill, 0, 0)
	assertSuccess(t, errCode, "Readdir /")

	// Check that we got all entries (., .., dir1, dir2, file1, file2)
	expectedEntries := map[string]bool{
		".":     true,
		"..":    true,
		"dir1":  true,
		"dir2":  true,
		"file1": true,
		"file2": true,
	}

	if len(entries) != len(expectedEntries) {
		t.Errorf("Readdir returned %d entries, expected %d", len(entries), len(expectedEntries))
	}

	for _, entry := range entries {
		if !expectedEntries[entry] {
			t.Errorf("Readdir returned unexpected entry: %s", entry)
		}
	}

	// Read empty directory
	fs.Mkdir("/emptydir", 0755)
	entries = entries[:0]
	errCode = fs.Readdir("/emptydir", fill, 0, 0)
	assertSuccess(t, errCode, "Readdir /emptydir")

	// Should only have . and ..
	if len(entries) != 2 {
		t.Errorf("Readdir on empty directory returned %d entries, expected 2", len(entries))
	}

	// Try to readdir on non-existent path
	errCode = fs.Readdir("/nonexistent", fill, 0, 0)
	assertError(t, errCode, -fuse.ENOENT, "Readdir non-existent")

	// Try to readdir on a file
	errCode = fs.Readdir("/file1", fill, 0, 0)
	assertError(t, errCode, -fuse.ENOTDIR, "Readdir on file")
}

// File operation tests

func TestMknod(t *testing.T) {
	fs := newTestFS()

	// Create a file
	errCode := fs.Mknod("/testfile", fuse.S_IFREG|0644, 0)
	assertSuccess(t, errCode, "Mknod /testfile")

	// Verify file exists and has correct attributes
	var stat fuse.Stat_t
	errCode = fs.Getattr("/testfile", &stat, 0)
	assertSuccess(t, errCode, "Getattr /testfile")

	expectedMode := uint32(fuse.S_IFREG | 0644)
	assertStatMode(t, &stat, expectedMode, "/testfile")
	assertStatSize(t, &stat, 0, "/testfile")

	// Try to create file that already exists
	errCode = fs.Mknod("/testfile", fuse.S_IFREG|0644, 0)
	assertError(t, errCode, -fuse.EEXIST, "Mknod existing file")

	// Try to create file in non-existent directory
	errCode = fs.Mknod("/nonexistent/file", fuse.S_IFREG|0644, 0)
	assertError(t, errCode, -fuse.ENOENT, "Mknod with missing parent")

	// Create file in subdirectory
	fs.Mkdir("/subdir", 0755)
	errCode = fs.Mknod("/subdir/file", fuse.S_IFREG|0600, 0)
	assertSuccess(t, errCode, "Mknod /subdir/file")

	errCode = fs.Getattr("/subdir/file", &stat, 0)
	assertSuccess(t, errCode, "Getattr /subdir/file")
	expectedMode = uint32(fuse.S_IFREG | 0600)
	assertStatMode(t, &stat, expectedMode, "/subdir/file")
}

func TestCreate(t *testing.T) {
	fs := newTestFS()

	// Create a file
	errCode, fh := fs.Create("/newfile", 0, 0644)
	assertSuccess(t, errCode, "Create /newfile")
	if fh != 0 {
		t.Errorf("Create returned file handle %d, expected 0", fh)
	}

	// Verify file exists
	var stat fuse.Stat_t
	errCode = fs.Getattr("/newfile", &stat, 0)
	assertSuccess(t, errCode, "Getattr /newfile")

	expectedMode := uint32(fuse.S_IFREG | 0644)
	assertStatMode(t, &stat, expectedMode, "/newfile")

	// Create file that already exists (should succeed)
	errCode, _ = fs.Create("/newfile", 0, 0644)
	assertSuccess(t, errCode, "Create existing file")

	// Try to create file in non-existent directory
	errCode, _ = fs.Create("/nonexistent/file", 0, 0644)
	assertError(t, errCode, -fuse.ENOENT, "Create with missing parent")
}

func TestOpen(t *testing.T) {
	fs := newTestFS()

	// Create a file and open it
	fs.Mknod("/testfile", fuse.S_IFREG|0644, 0)
	errCode, fh := fs.Open("/testfile", 0)
	assertSuccess(t, errCode, "Open /testfile")
	if fh != 0 {
		t.Errorf("Open returned file handle %d, expected 0", fh)
	}

	// Try to open non-existent file
	errCode, _ = fs.Open("/nonexistent", 0)
	assertError(t, errCode, -fuse.ENOENT, "Open non-existent")

	// Try to open a directory as a file
	fs.Mkdir("/testdir", 0755)
	errCode, _ = fs.Open("/testdir", 0)
	assertError(t, errCode, -fuse.EISDIR, "Open directory as file")
}

func TestUnlink(t *testing.T) {
	fs := newTestFS()

	// Create and remove a file
	fs.Mknod("/testfile", fuse.S_IFREG|0644, 0)
	errCode := fs.Unlink("/testfile")
	assertSuccess(t, errCode, "Unlink /testfile")

	// Verify file no longer exists
	var stat fuse.Stat_t
	errCode = fs.Getattr("/testfile", &stat, 0)
	assertError(t, errCode, -fuse.ENOENT, "Getattr on removed file")

	// Try to unlink non-existent file
	errCode = fs.Unlink("/nonexistent")
	assertError(t, errCode, -fuse.ENOENT, "Unlink non-existent")

	// Try to unlink a directory
	fs.Mkdir("/testdir", 0755)
	errCode = fs.Unlink("/testdir")
	assertError(t, errCode, -fuse.EISDIR, "Unlink directory")
}

// File I/O tests

func TestRead(t *testing.T) {
	fs := newTestFS()

	// Create file with content
	fs.Mknod("/testfile", fuse.S_IFREG|0644, 0)
	testData := []byte("Hello, World!")
	fs.Write("/testfile", testData, 0, 0)

	// Read entire file
	buffer := make([]byte, 100)
	bytesRead := fs.Read("/testfile", buffer, 0, 0)
	if bytesRead != len(testData) {
		t.Errorf("Read returned %d bytes, expected %d", bytesRead, len(testData))
	}

	readData := buffer[:bytesRead]
	if string(readData) != string(testData) {
		t.Errorf("Read returned %q, expected %q", readData, testData)
	}

	// Read at offset
	buffer = make([]byte, 5)
	bytesRead = fs.Read("/testfile", buffer, 7, 0)
	if bytesRead != 5 {
		t.Errorf("Read at offset returned %d bytes, expected 5", bytesRead)
	}
	if string(buffer[:bytesRead]) != "World" {
		t.Errorf("Read at offset returned %q, expected %q", buffer[:bytesRead], "World")
	}

	// Read beyond EOF
	buffer = make([]byte, 100)
	bytesRead = fs.Read("/testfile", buffer, 100, 0)
	if bytesRead != 0 {
		t.Errorf("Read beyond EOF returned %d bytes, expected 0", bytesRead)
	}

	// Read from empty file
	fs.Mknod("/emptyfile", fuse.S_IFREG|0644, 0)
	bytesRead = fs.Read("/emptyfile", buffer, 0, 0)
	if bytesRead != 0 {
		t.Errorf("Read from empty file returned %d bytes, expected 0", bytesRead)
	}

	// Try to read from non-existent file
	bytesRead = fs.Read("/nonexistent", buffer, 0, 0)
	assertError(t, bytesRead, -fuse.ENOENT, "Read non-existent")

	// Try to read from directory
	fs.Mkdir("/testdir", 0755)
	bytesRead = fs.Read("/testdir", buffer, 0, 0)
	assertError(t, bytesRead, -fuse.EISDIR, "Read directory")
}

func TestWrite(t *testing.T) {
	fs := newTestFS()

	// Create file and write data
	fs.Mknod("/testfile", fuse.S_IFREG|0644, 0)
	testData := []byte("Hello, World!")
	bytesWritten := fs.Write("/testfile", testData, 0, 0)
	if bytesWritten != len(testData) {
		t.Errorf("Write returned %d bytes, expected %d", bytesWritten, len(testData))
	}

	// Verify file size
	var stat fuse.Stat_t
	fs.Getattr("/testfile", &stat, 0)
	assertStatSize(t, &stat, int64(len(testData)), "/testfile")

	// Write at offset (expand file)
	moreData := []byte("!!!")
	bytesWritten = fs.Write("/testfile", moreData, 20, 0)
	if bytesWritten != len(moreData) {
		t.Errorf("Write at offset returned %d bytes, expected %d", bytesWritten, len(moreData))
	}

	fs.Getattr("/testfile", &stat, 0)
	assertStatSize(t, &stat, 23, "/testfile")

	// Verify sparse file has zeros in gap
	buffer := make([]byte, 23)
	fs.Read("/testfile", buffer, 0, 0)
	expectedData := make([]byte, 23)
	copy(expectedData[:13], testData)
	copy(expectedData[20:], moreData)
	if string(buffer) != string(expectedData) {
		t.Errorf("File content mismatch after sparse write")
	}

	// Overwrite existing data
	overwriteData := []byte("Goodbye")
	fs.Write("/testfile", overwriteData, 0, 0)
	buffer = make([]byte, 7)
	fs.Read("/testfile", buffer, 0, 0)
	if string(buffer) != string(overwriteData) {
		t.Errorf("Overwrite failed, got %q, expected %q", buffer, overwriteData)
	}

	// Try to write to non-existent file
	bytesWritten = fs.Write("/nonexistent", testData, 0, 0)
	assertError(t, bytesWritten, -fuse.ENOENT, "Write non-existent")

	// Try to write to directory
	fs.Mkdir("/testdir", 0755)
	bytesWritten = fs.Write("/testdir", testData, 0, 0)
	assertError(t, bytesWritten, -fuse.EISDIR, "Write directory")
}

func TestTruncate(t *testing.T) {
	fs := newTestFS()

	// Create file with content
	fs.Mknod("/testfile", fuse.S_IFREG|0644, 0)
	testData := []byte("Hello, World!")
	fs.Write("/testfile", testData, 0, 0)

	// Truncate to smaller size
	errCode := fs.Truncate("/testfile", 5, 0)
	assertSuccess(t, errCode, "Truncate to 5")

	var stat fuse.Stat_t
	fs.Getattr("/testfile", &stat, 0)
	assertStatSize(t, &stat, 5, "/testfile")

	buffer := make([]byte, 5)
	fs.Read("/testfile", buffer, 0, 0)
	if string(buffer) != "Hello" {
		t.Errorf("After truncate, content is %q, expected %q", buffer, "Hello")
	}

	// Truncate to larger size (should pad with zeros)
	errCode = fs.Truncate("/testfile", 10, 0)
	assertSuccess(t, errCode, "Truncate to 10")

	fs.Getattr("/testfile", &stat, 0)
	assertStatSize(t, &stat, 10, "/testfile")

	buffer = make([]byte, 10)
	fs.Read("/testfile", buffer, 0, 0)
	expectedData := append([]byte("Hello"), make([]byte, 5)...)
	if string(buffer) != string(expectedData) {
		t.Errorf("After truncate expansion, content mismatch")
	}

	// Truncate to zero
	errCode = fs.Truncate("/testfile", 0, 0)
	assertSuccess(t, errCode, "Truncate to 0")

	fs.Getattr("/testfile", &stat, 0)
	assertStatSize(t, &stat, 0, "/testfile")

	// Try to truncate non-existent file
	errCode = fs.Truncate("/nonexistent", 10, 0)
	assertError(t, errCode, -fuse.ENOENT, "Truncate non-existent")

	// Try to truncate directory
	fs.Mkdir("/testdir", 0755)
	errCode = fs.Truncate("/testdir", 10, 0)
	assertError(t, errCode, -fuse.EISDIR, "Truncate directory")
}

// Rename and metadata tests

func TestRename(t *testing.T) {
	fs := newTestFS()

	// Create and rename a file
	fs.Mknod("/oldname", fuse.S_IFREG|0644, 0)
	testData := []byte("test content")
	fs.Write("/oldname", testData, 0, 0)

	errCode := fs.Rename("/oldname", "/newname")
	assertSuccess(t, errCode, "Rename /oldname to /newname")

	// Verify old name doesn't exist
	var stat fuse.Stat_t
	errCode = fs.Getattr("/oldname", &stat, 0)
	assertError(t, errCode, -fuse.ENOENT, "Getattr old name")

	// Verify new name exists with correct content
	errCode = fs.Getattr("/newname", &stat, 0)
	assertSuccess(t, errCode, "Getattr new name")

	buffer := make([]byte, 100)
	bytesRead := fs.Read("/newname", buffer, 0, 0)
	if string(buffer[:bytesRead]) != string(testData) {
		t.Errorf("Content after rename mismatch")
	}

	// Rename to different directory
	fs.Mkdir("/dir", 0755)
	errCode = fs.Rename("/newname", "/dir/movedfile")
	assertSuccess(t, errCode, "Rename to different directory")

	errCode = fs.Getattr("/dir/movedfile", &stat, 0)
	assertSuccess(t, errCode, "Getattr moved file")

	// Rename directory
	fs.Mkdir("/olddir", 0755)
	fs.Mkdir("/olddir/subdir", 0755)
	fs.Mknod("/olddir/file", fuse.S_IFREG|0644, 0)

	errCode = fs.Rename("/olddir", "/newdir")
	assertSuccess(t, errCode, "Rename directory")

	// Verify old directory doesn't exist
	errCode = fs.Getattr("/olddir", &stat, 0)
	assertError(t, errCode, -fuse.ENOENT, "Getattr old directory name")

	// Verify new directory and children exist
	errCode = fs.Getattr("/newdir", &stat, 0)
	assertSuccess(t, errCode, "Getattr new directory name")

	errCode = fs.Getattr("/newdir/subdir", &stat, 0)
	assertSuccess(t, errCode, "Getattr renamed directory child")

	errCode = fs.Getattr("/newdir/file", &stat, 0)
	assertSuccess(t, errCode, "Getattr renamed directory file")

	// Rename to existing file (should overwrite)
	fs.Mknod("/file1", fuse.S_IFREG|0644, 0)
	fs.Mknod("/file2", fuse.S_IFREG|0644, 0)
	fs.Write("/file1", []byte("file1"), 0, 0)
	fs.Write("/file2", []byte("file2"), 0, 0)

	errCode = fs.Rename("/file1", "/file2")
	assertSuccess(t, errCode, "Rename to existing file")

	buffer = make([]byte, 100)
	bytesRead = fs.Read("/file2", buffer, 0, 0)
	if string(buffer[:bytesRead]) != "file1" {
		t.Errorf("After rename to existing, content is %q, expected 'file1'", buffer[:bytesRead])
	}

	// Try to rename non-existent file
	errCode = fs.Rename("/nonexistent", "/somewhere")
	assertError(t, errCode, -fuse.ENOENT, "Rename non-existent")
}

func TestGetattr(t *testing.T) {
	fs := newTestFS()

	// Get attributes of root
	var stat fuse.Stat_t
	errCode := fs.Getattr("/", &stat, 0)
	assertSuccess(t, errCode, "Getattr /")

	if stat.Mode&fuse.S_IFMT != fuse.S_IFDIR {
		t.Errorf("Root is not a directory")
	}

	// Create file and get attributes
	fs.Mknod("/testfile", fuse.S_IFREG|0600, 0)
	errCode = fs.Getattr("/testfile", &stat, 0)
	assertSuccess(t, errCode, "Getattr /testfile")

	expectedMode := uint32(fuse.S_IFREG | 0600)
	assertStatMode(t, &stat, expectedMode, "/testfile")
	assertStatSize(t, &stat, 0, "/testfile")

	// Write to file and check size changes
	fs.Write("/testfile", []byte("data"), 0, 0)
	errCode = fs.Getattr("/testfile", &stat, 0)
	assertSuccess(t, errCode, "Getattr after write")
	assertStatSize(t, &stat, 4, "/testfile")

	// Try to get attributes of non-existent path
	errCode = fs.Getattr("/nonexistent", &stat, 0)
	assertError(t, errCode, -fuse.ENOENT, "Getattr non-existent")
}

func TestChmod(t *testing.T) {
	fs := newTestFS()

	// Create file and change permissions
	fs.Mknod("/testfile", fuse.S_IFREG|0644, 0)

	errCode := fs.Chmod("/testfile", 0600)
	assertSuccess(t, errCode, "Chmod /testfile")

	var stat fuse.Stat_t
	fs.Getattr("/testfile", &stat, 0)
	expectedMode := uint32(fuse.S_IFREG | 0600)
	assertStatMode(t, &stat, expectedMode, "/testfile")

	// Change directory permissions
	fs.Mkdir("/testdir", 0755)
	errCode = fs.Chmod("/testdir", 0700)
	assertSuccess(t, errCode, "Chmod /testdir")

	fs.Getattr("/testdir", &stat, 0)
	expectedMode = uint32(fuse.S_IFDIR | 0700)
	assertStatMode(t, &stat, expectedMode, "/testdir")

	// Try to chmod non-existent path
	errCode = fs.Chmod("/nonexistent", 0755)
	assertError(t, errCode, -fuse.ENOENT, "Chmod non-existent")
}

func TestChown(t *testing.T) {
	fs := newTestFS()

	// Create file and change ownership
	fs.Mknod("/testfile", fuse.S_IFREG|0644, 0)

	errCode := fs.Chown("/testfile", 1000, 1000)
	assertSuccess(t, errCode, "Chown /testfile")

	var stat fuse.Stat_t
	fs.Getattr("/testfile", &stat, 0)
	if stat.Uid != 1000 || stat.Gid != 1000 {
		t.Errorf("Chown failed, uid=%d gid=%d, expected uid=1000 gid=1000", stat.Uid, stat.Gid)
	}

	// Change directory ownership
	fs.Mkdir("/testdir", 0755)
	errCode = fs.Chown("/testdir", 2000, 2000)
	assertSuccess(t, errCode, "Chown /testdir")

	fs.Getattr("/testdir", &stat, 0)
	if stat.Uid != 2000 || stat.Gid != 2000 {
		t.Errorf("Chown directory failed, uid=%d gid=%d, expected uid=2000 gid=2000", stat.Uid, stat.Gid)
	}

	// Try to chown non-existent path
	errCode = fs.Chown("/nonexistent", 1000, 1000)
	assertError(t, errCode, -fuse.ENOENT, "Chown non-existent")
}

func TestUtimens(t *testing.T) {
	fs := newTestFS()

	// Create file
	fs.Mknod("/testfile", fuse.S_IFREG|0644, 0)

	// Set custom timestamps
	customTime := fuse.Timespec{Sec: 1000000, Nsec: 0}
	times := []fuse.Timespec{customTime, customTime}

	errCode := fs.Utimens("/testfile", times)
	assertSuccess(t, errCode, "Utimens /testfile")

	var stat fuse.Stat_t
	fs.Getattr("/testfile", &stat, 0)

	if stat.Atim.Sec != 1000000 || stat.Mtim.Sec != 1000000 {
		t.Errorf("Utimens failed, atim=%d mtim=%d, expected both to be 1000000",
			stat.Atim.Sec, stat.Mtim.Sec)
	}

	// Set timestamps on directory
	fs.Mkdir("/testdir", 0755)
	errCode = fs.Utimens("/testdir", times)
	assertSuccess(t, errCode, "Utimens /testdir")

	fs.Getattr("/testdir", &stat, 0)
	if stat.Atim.Sec != 1000000 || stat.Mtim.Sec != 1000000 {
		t.Errorf("Utimens directory failed")
	}

	// Try to set timestamps on non-existent path
	errCode = fs.Utimens("/nonexistent", times)
	assertError(t, errCode, -fuse.ENOENT, "Utimens non-existent")
}

func TestStatfs(t *testing.T) {
	fs := newTestFS()

	var statfs fuse.Statfs_t
	errCode := fs.Statfs("/", &statfs)
	assertSuccess(t, errCode, "Statfs /")

	// Check that filesystem stats are populated
	// The exact values don't matter much for an in-memory filesystem
	// but we can check they're non-zero
	if statfs.Bsize == 0 {
		t.Error("Statfs returned zero block size")
	}
	if statfs.Frsize == 0 {
		t.Error("Statfs returned zero fragment size")
	}
	if statfs.Blocks == 0 {
		t.Error("Statfs returned zero blocks")
	}

	// Statfs should work with any path
	fs.Mkdir("/testdir", 0755)
	errCode = fs.Statfs("/testdir", &statfs)
	assertSuccess(t, errCode, "Statfs /testdir")
}

// Error condition tests

func TestErrorConditions(t *testing.T) {
	fs := newTestFS()

	// ENOENT - file/directory not found
	var stat fuse.Stat_t
	errCode := fs.Getattr("/nonexistent", &stat, 0)
	assertError(t, errCode, -fuse.ENOENT, "ENOENT: Getattr")

	errCode = fs.Mkdir("/nonexistent/dir", 0755)
	assertError(t, errCode, -fuse.ENOENT, "ENOENT: Mkdir with missing parent")

	errCode = fs.Rmdir("/nonexistent")
	assertError(t, errCode, -fuse.ENOENT, "ENOENT: Rmdir")

	errCode = fs.Unlink("/nonexistent")
	assertError(t, errCode, -fuse.ENOENT, "ENOENT: Unlink")

	errCode, _ = fs.Open("/nonexistent", 0)
	assertError(t, errCode, -fuse.ENOENT, "ENOENT: Open")

	// EEXIST - file/directory already exists
	fs.Mkdir("/existingdir", 0755)
	errCode = fs.Mkdir("/existingdir", 0755)
	assertError(t, errCode, -fuse.EEXIST, "EEXIST: Mkdir")

	fs.Mknod("/existingfile", fuse.S_IFREG|0644, 0)
	errCode = fs.Mknod("/existingfile", fuse.S_IFREG|0644, 0)
	assertError(t, errCode, -fuse.EEXIST, "EEXIST: Mknod")

	// ENOTDIR - path component is not a directory
	fs.Mknod("/file", fuse.S_IFREG|0644, 0)
	errCode = fs.Mkdir("/file/subdir", 0755)
	assertError(t, errCode, -fuse.ENOTDIR, "ENOTDIR: Mkdir under file")

	errCode = fs.Rmdir("/file")
	assertError(t, errCode, -fuse.ENOTDIR, "ENOTDIR: Rmdir on file")

	errCode, _ = fs.Opendir("/file")
	assertError(t, errCode, -fuse.ENOTDIR, "ENOTDIR: Opendir on file")

	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool { return true }
	errCode = fs.Readdir("/file", fill, 0, 0)
	assertError(t, errCode, -fuse.ENOTDIR, "ENOTDIR: Readdir on file")

	// EISDIR - operation invalid on directory
	fs.Mkdir("/dir", 0755)
	errCode, _ = fs.Open("/dir", 0)
	assertError(t, errCode, -fuse.EISDIR, "EISDIR: Open")

	errCode = fs.Unlink("/dir")
	assertError(t, errCode, -fuse.EISDIR, "EISDIR: Unlink")

	buffer := make([]byte, 100)
	bytesRead := fs.Read("/dir", buffer, 0, 0)
	assertError(t, bytesRead, -fuse.EISDIR, "EISDIR: Read")

	bytesWritten := fs.Write("/dir", []byte("data"), 0, 0)
	assertError(t, bytesWritten, -fuse.EISDIR, "EISDIR: Write")

	errCode = fs.Truncate("/dir", 10, 0)
	assertError(t, errCode, -fuse.EISDIR, "EISDIR: Truncate")

	// ENOTEMPTY - directory not empty
	fs.Mkdir("/parent", 0755)
	fs.Mkdir("/parent/child", 0755)
	errCode = fs.Rmdir("/parent")
	assertError(t, errCode, -fuse.ENOTEMPTY, "ENOTEMPTY: Rmdir")

	// Clean up and verify it works after removing child
	fs.Rmdir("/parent/child")
	errCode = fs.Rmdir("/parent")
	assertSuccess(t, errCode, "Rmdir after cleanup")
}

// Concurrency tests

func TestConcurrency(t *testing.T) {
	fs := newTestFS()

	// Test concurrent reads and writes
	t.Run("ConcurrentReadWrite", func(t *testing.T) {
		// Create test files
		for i := 0; i < 5; i++ {
			path := "/file" + string(rune('0'+i))
			fs.Mknod(path, fuse.S_IFREG|0644, 0)
		}

		var wg sync.WaitGroup

		// Start writer goroutines
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				path := "/file" + string(rune('0'+id))
				for j := 0; j < 10; j++ {
					data := []byte("data" + string(rune('0'+id)) + string(rune('0'+j)))
					fs.Write(path, data, 0, 0)
					time.Sleep(time.Millisecond * time.Duration(rand.Intn(5)))
				}
			}(i)
		}

		// Start reader goroutines
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				path := "/file" + string(rune('0'+id))
				buffer := make([]byte, 100)
				for j := 0; j < 10; j++ {
					fs.Read(path, buffer, 0, 0)
					time.Sleep(time.Millisecond * time.Duration(rand.Intn(5)))
				}
			}(i)
		}

		wg.Wait()
	})

	// Test concurrent directory operations
	t.Run("ConcurrentMkdirRmdir", func(t *testing.T) {
		var wg sync.WaitGroup

		// Create and remove directories concurrently
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					path := "/dir" + string(rune('0'+id)) + string(rune('0'+j))
					fs.Mkdir(path, 0755)
					time.Sleep(time.Millisecond * time.Duration(rand.Intn(3)))
					fs.Rmdir(path)
				}
			}(i)
		}

		wg.Wait()
	})

	// Test concurrent file creation and deletion
	t.Run("ConcurrentCreateUnlink", func(t *testing.T) {
		var wg sync.WaitGroup

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					path := "/f" + string(rune('0'+id)) + string(rune('0'+j))
					fs.Mknod(path, fuse.S_IFREG|0644, 0)
					fs.Write(path, []byte("test"), 0, 0)
					time.Sleep(time.Millisecond * time.Duration(rand.Intn(3)))
					fs.Unlink(path)
				}
			}(i)
		}

		wg.Wait()
	})

	// Test concurrent operations on the same file
	t.Run("ConcurrentSameFile", func(t *testing.T) {
		fs.Mknod("/shared", fuse.S_IFREG|0644, 0)

		var wg sync.WaitGroup

		// Multiple writers to the same file
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					data := []byte("goroutine" + string(rune('0'+id)))
					fs.Write("/shared", data, int64(id*10), 0)
					time.Sleep(time.Millisecond)
				}
			}(i)
		}

		// Multiple readers of the same file
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				buffer := make([]byte, 100)
				for j := 0; j < 10; j++ {
					fs.Read("/shared", buffer, 0, 0)
					time.Sleep(time.Millisecond)
				}
			}()
		}

		wg.Wait()

		// Verify file still exists and is readable
		var stat fuse.Stat_t
		errCode := fs.Getattr("/shared", &stat, 0)
		assertSuccess(t, errCode, "Getattr after concurrent access")
	})

	// Test concurrent metadata operations
	t.Run("ConcurrentMetadata", func(t *testing.T) {
		fs.Mknod("/metafile", fuse.S_IFREG|0644, 0)

		var wg sync.WaitGroup

		// Concurrent chmod
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				modes := []uint32{0600, 0644, 0666}
				for j := 0; j < 5; j++ {
					fs.Chmod("/metafile", modes[id])
					time.Sleep(time.Millisecond)
				}
			}(i)
		}

		// Concurrent chown
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					fs.Chown("/metafile", uint32(1000+id), uint32(1000+id))
					time.Sleep(time.Millisecond)
				}
			}(i)
		}

		// Concurrent getattr
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var stat fuse.Stat_t
				for j := 0; j < 10; j++ {
					fs.Getattr("/metafile", &stat, 0)
					time.Sleep(time.Millisecond)
				}
			}()
		}

		wg.Wait()
	})
}
