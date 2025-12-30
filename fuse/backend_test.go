package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/winfsp/cgofuse/fuse"
)

// TestLocalBackendStat tests Stat operation
func TestLocalBackendStat(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	b := NewLocalBackend(tmpDir)

	// Test stat on existing file
	stat, err := b.Stat("/testfile.txt")
	if err != 0 {
		t.Errorf("Stat failed with error %d, expected 0", err)
	}
	if stat == nil {
		t.Fatal("Stat returned nil")
	}
	if stat.Mode&fuse.S_IFREG == 0 {
		t.Errorf("Stat mode doesn't indicate regular file: 0x%x", stat.Mode)
	}
	if stat.Size != 5 {
		t.Errorf("Stat size = %d, expected 5", stat.Size)
	}

	// Test stat on non-existent file
	_, err = b.Stat("/nonexistent.txt")
	if err != -fuse.ENOENT {
		t.Errorf("Stat non-existent returned %d, expected %d", err, -fuse.ENOENT)
	}
}

// TestLocalBackendReaddir tests Readdir operation
func TestLocalBackendReaddir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content2"), 0644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	b := NewLocalBackend(tmpDir)

	ents, err := b.Readdir("/")
	if err != 0 {
		t.Errorf("Readdir failed with error %d", err)
	}

	if len(ents) != 3 {
		t.Errorf("Readdir returned %d entries, expected 3", len(ents))
	}

	// Check that entries are present
	names := make(map[string]bool)
	for _, e := range ents {
		names[e.Name] = true
	}

	if !names["file1.txt"] || !names["file2.txt"] || !names["subdir"] {
		t.Errorf("Readdir missing expected entries")
	}

	// Test readdir on non-existent directory
	_, err = b.Readdir("/nonexistent")
	if err != -fuse.ENOENT {
		t.Errorf("Readdir non-existent returned %d, expected %d", err, -fuse.ENOENT)
	}
}

// TestLocalBackendRead tests Read operation
func TestLocalBackendRead(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "testfile.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	b := NewLocalBackend(tmpDir)

	buff := make([]byte, 5)
	bytesRead, err := b.Read("/testfile.txt", buff, 0)
	if err != 0 {
		t.Errorf("Read failed with error %d", err)
	}
	if bytesRead != 5 {
		t.Errorf("Read returned %d bytes, expected 5", bytesRead)
	}
	if string(buff) != "hello" {
		t.Errorf("Read data = %s, expected 'hello'", string(buff))
	}

	// Test read with offset
	buff2 := make([]byte, 5)
	bytesRead, err = b.Read("/testfile.txt", buff2, 6)
	if err != 0 {
		t.Errorf("Read with offset failed with error %d", err)
	}
	if string(buff2) != "world" {
		t.Errorf("Read with offset data = %s, expected 'world'", string(buff2))
	}
}

// TestLocalBackendWrite tests Write operation
func TestLocalBackendWrite(t *testing.T) {
	tmpDir := t.TempDir()
	b := NewLocalBackend(tmpDir)

	testFile := "/testwrite.txt"
	data := []byte("test data")

	bytesWritten, err := b.Write(testFile, data, 0)
	if err != 0 {
		t.Errorf("Write failed with error %d", err)
	}
	if bytesWritten != len(data) {
		t.Errorf("Write returned %d bytes, expected %d", bytesWritten, len(data))
	}

	// Verify file was created
	content, osErr := os.ReadFile(filepath.Join(tmpDir, "testwrite.txt"))
	if osErr != nil {
		t.Errorf("failed to verify written file: %v", osErr)
	}
	if string(content) != "test data" {
		t.Errorf("file content = %s, expected 'test data'", string(content))
	}
}

// TestLocalBackendCreate tests Create operation
func TestLocalBackendCreate(t *testing.T) {
	tmpDir := t.TempDir()
	b := NewLocalBackend(tmpDir)

	err := b.Create("/newfile.txt", 0644)
	if err != 0 {
		t.Errorf("Create failed with error %d", err)
	}

	// Verify file exists
	_, err2 := os.Stat(filepath.Join(tmpDir, "newfile.txt"))
	if err2 != nil {
		t.Errorf("file not created: %v", err2)
	}
}

// TestLocalBackendMkdir tests Mkdir operation
func TestLocalBackendMkdir(t *testing.T) {
	tmpDir := t.TempDir()
	b := NewLocalBackend(tmpDir)

	err := b.Mkdir("/testdir", 0755)
	if err != 0 {
		t.Errorf("Mkdir failed with error %d", err)
	}

	// Verify directory exists
	stat, err2 := os.Stat(filepath.Join(tmpDir, "testdir"))
	if err2 != nil {
		t.Errorf("directory not created: %v", err2)
	}
	if !stat.IsDir() {
		t.Error("created path is not a directory")
	}

	// Test creating existing directory
	err = b.Mkdir("/testdir", 0755)
	if err != -fuse.EEXIST {
		t.Errorf("Mkdir existing returned %d, expected %d", err, -fuse.EEXIST)
	}
}

// TestLocalBackendUnlink tests Unlink operation
func TestLocalBackendUnlink(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "testfile.txt")
	os.WriteFile(testFile, []byte("content"), 0644)

	b := NewLocalBackend(tmpDir)

	err := b.Unlink("/testfile.txt")
	if err != 0 {
		t.Errorf("Unlink failed with error %d", err)
	}

	// Verify file is deleted
	_, err2 := os.Stat(testFile)
	if err2 == nil {
		t.Error("file still exists after unlink")
	}
}

// TestLocalBackendRmdir tests Rmdir operation
func TestLocalBackendRmdir(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "testdir")
	os.Mkdir(testDir, 0755)

	b := NewLocalBackend(tmpDir)

	err := b.Rmdir("/testdir")
	if err != 0 {
		t.Errorf("Rmdir failed with error %d", err)
	}

	// Verify directory is deleted
	_, err2 := os.Stat(testDir)
	if err2 == nil {
		t.Error("directory still exists after rmdir")
	}
}

// TestLocalBackendRename tests Rename operation
func TestLocalBackendRename(t *testing.T) {
	tmpDir := t.TempDir()
	oldFile := filepath.Join(tmpDir, "old.txt")
	os.WriteFile(oldFile, []byte("content"), 0644)

	b := NewLocalBackend(tmpDir)

	err := b.Rename("/old.txt", "/new.txt")
	if err != 0 {
		t.Errorf("Rename failed with error %d", err)
	}

	// Verify old file doesn't exist
	_, err2 := os.Stat(oldFile)
	if err2 == nil {
		t.Error("old file still exists after rename")
	}

	// Verify new file exists
	newFile := filepath.Join(tmpDir, "new.txt")
	_, err3 := os.Stat(newFile)
	if err3 != nil {
		t.Errorf("new file not created: %v", err3)
	}
}

// TestLocalBackendTruncate tests Truncate operation
func TestLocalBackendTruncate(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "testfile.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	b := NewLocalBackend(tmpDir)

	err := b.Truncate("/testfile.txt", 5)
	if err != 0 {
		t.Errorf("Truncate failed with error %d", err)
	}

	// Verify file size
	content, _ := os.ReadFile(testFile)
	if len(content) != 5 {
		t.Errorf("file size = %d, expected 5", len(content))
	}
}
