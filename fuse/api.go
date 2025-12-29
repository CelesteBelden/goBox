package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/winfsp/cgofuse/fuse"
)

// APIServer wraps MemFS and provides HTTP endpoints
type APIServer struct {
	fs            *MemFS
	handleMap     map[uint64]*FileHandle
	handleMutex   sync.Mutex
	handleCounter atomic.Uint64
}

// FileHandle tracks open file handles server-side
type FileHandle struct {
	path string
	fh   uint64
}

// Response is the standard JSON response structure
type Response struct {
	Error int         `json:"error"`
	Data  interface{} `json:"data,omitempty"`
}

// NewAPIServer creates a new API server wrapping the filesystem
func NewAPIServer(fs *MemFS) *APIServer {
	return &APIServer{
		fs:        fs,
		handleMap: make(map[uint64]*FileHandle),
	}
}

// getNextHandleID generates the next incrementing handle ID
func (s *APIServer) getNextHandleID() uint64 {
	return s.handleCounter.Add(1)
}

// RegisterRoutes registers all HTTP endpoints
func (s *APIServer) RegisterRoutes() {
	// Metadata endpoints
	http.HandleFunc("/api/getattr", s.handleGetattr)
	http.HandleFunc("/api/chmod", s.handleChmod)
	http.HandleFunc("/api/chown", s.handleChown)
	http.HandleFunc("/api/utimens", s.handleUtimens)

	// Directory endpoints
	http.HandleFunc("/api/mkdir", s.handleMkdir)
	http.HandleFunc("/api/rmdir", s.handleRmdir)
	http.HandleFunc("/api/opendir", s.handleOpendir)
	http.HandleFunc("/api/readdir", s.handleReaddir)
	http.HandleFunc("/api/readdir/paginated", s.handleReaddirPaginated)

	// File endpoints
	http.HandleFunc("/api/create", s.handleCreate)
	http.HandleFunc("/api/unlink", s.handleUnlink)
	http.HandleFunc("/api/truncate", s.handleTruncate)
	http.HandleFunc("/api/rename", s.handleRename)

	// Binary file I/O
	http.HandleFunc("/api/files/read", s.handleFileRead)
	http.HandleFunc("/api/files/write", s.handleFileWrite)

	// Filesystem stats
	http.HandleFunc("/api/statfs", s.handleStatfs)
}

// Helper to write JSON response
func writeJSON(w http.ResponseWriter, statusCode int, resp Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(resp)
}

// Helper to map FUSE error codes to HTTP status codes
func fuseErrorToHTTP(fuseErr int) int {
	switch fuseErr {
	case 0:
		return http.StatusOK
	case -2: // ENOENT (file not found)
		return http.StatusNotFound
	case -13: // EACCES (permission denied)
		return http.StatusForbidden
	case -17: // EEXIST (file exists)
		return http.StatusConflict
	case -21: // EISDIR (is a directory)
		return http.StatusBadRequest
	case -20: // ENOTDIR (not a directory)
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// ============ Metadata Endpoints ============

func (s *APIServer) handleGetattr(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	stat := &fuse.Stat_t{}
	err := s.fs.Getattr(path, stat, 0)
	statusCode := fuseErrorToHTTP(err)
	writeJSON(w, statusCode, Response{Error: err, Data: stat})
}

func (s *APIServer) handleChmod(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	var req struct {
		Path string `json:"path"`
		Mode uint32 `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	err := s.fs.Chmod(req.Path, req.Mode)
	statusCode := fuseErrorToHTTP(err)
	writeJSON(w, statusCode, Response{Error: err})
}

func (s *APIServer) handleChown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	var req struct {
		Path string `json:"path"`
		UID  uint32 `json:"uid"`
		GID  uint32 `json:"gid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	err := s.fs.Chown(req.Path, req.UID, req.GID)
	statusCode := fuseErrorToHTTP(err)
	writeJSON(w, statusCode, Response{Error: err})
}

func (s *APIServer) handleUtimens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	var req struct {
		Path string          `json:"path"`
		Tmsp []fuse.Timespec `json:"times"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	err := s.fs.Utimens(req.Path, req.Tmsp)
	statusCode := fuseErrorToHTTP(err)
	writeJSON(w, statusCode, Response{Error: err})
}

// ============ Directory Endpoints ============

func (s *APIServer) handleMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	var req struct {
		Path string `json:"path"`
		Mode uint32 `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	err := s.fs.Mkdir(req.Path, req.Mode)
	statusCode := fuseErrorToHTTP(err)
	writeJSON(w, statusCode, Response{Error: err})
}

func (s *APIServer) handleRmdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	err := s.fs.Rmdir(path)
	statusCode := fuseErrorToHTTP(err)
	writeJSON(w, statusCode, Response{Error: err})
}

func (s *APIServer) handleOpendir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	err, fh := s.fs.Opendir(req.Path)
	statusCode := fuseErrorToHTTP(err)

	if err == 0 {
		// Store handle server-side
		clientHandle := s.getNextHandleID()
		s.handleMutex.Lock()
		s.handleMap[clientHandle] = &FileHandle{path: req.Path, fh: fh}
		s.handleMutex.Unlock()

		writeJSON(w, statusCode, Response{Error: err, Data: map[string]uint64{"handle": clientHandle}})
	} else {
		writeJSON(w, statusCode, Response{Error: err})
	}
}

func (s *APIServer) handleReaddir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Collect entries with callback
	var entries []map[string]interface{}
	err := s.fs.Readdir(path, func(name string, stat *fuse.Stat_t, ofst int64) bool {
		entry := map[string]interface{}{
			"name": name,
			"stat": stat,
		}
		entries = append(entries, entry)
		return true
	}, 0, 0)

	if err != 0 {
		w.WriteHeader(fuseErrorToHTTP(err))
		json.NewEncoder(w).Encode(Response{Error: err})
		return
	}

	w.WriteHeader(http.StatusOK)

	// Stream entries in chunks
	for _, entry := range entries {
		resp := Response{Error: 0, Data: entry}
		json.NewEncoder(w).Encode(resp)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func (s *APIServer) handleReaddirPaginated(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Collect all entries
	var allEntries []map[string]interface{}
	err := s.fs.Readdir(path, func(name string, stat *fuse.Stat_t, ofst int64) bool {
		entry := map[string]interface{}{
			"name": name,
			"stat": stat,
		}
		allEntries = append(allEntries, entry)
		return true
	}, 0, 0)

	if err != 0 {
		statusCode := fuseErrorToHTTP(err)
		writeJSON(w, statusCode, Response{Error: err})
		return
	}

	// Paginate results
	end := offset + limit
	if end > len(allEntries) {
		end = len(allEntries)
	}

	pageEntries := allEntries[offset:end]
	data := map[string]interface{}{
		"entries": pageEntries,
		"offset":  offset,
		"limit":   limit,
		"total":   len(allEntries),
	}

	writeJSON(w, http.StatusOK, Response{Error: 0, Data: data})
}

// ============ File Endpoints ============

func (s *APIServer) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	var req struct {
		Path  string `json:"path"`
		Flags int    `json:"flags"`
		Mode  uint32 `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	err, fh := s.fs.Create(req.Path, req.Flags, req.Mode)
	statusCode := fuseErrorToHTTP(err)

	if err == 0 {
		// Store handle server-side
		clientHandle := s.getNextHandleID()
		s.handleMutex.Lock()
		s.handleMap[clientHandle] = &FileHandle{path: req.Path, fh: fh}
		s.handleMutex.Unlock()

		writeJSON(w, statusCode, Response{Error: err, Data: map[string]uint64{"handle": clientHandle}})
	} else {
		writeJSON(w, statusCode, Response{Error: err})
	}
}

func (s *APIServer) handleUnlink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	err := s.fs.Unlink(path)
	statusCode := fuseErrorToHTTP(err)
	writeJSON(w, statusCode, Response{Error: err})
}

func (s *APIServer) handleTruncate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	var req struct {
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	err := s.fs.Truncate(req.Path, req.Size, 0)
	statusCode := fuseErrorToHTTP(err)
	writeJSON(w, statusCode, Response{Error: err})
}

func (s *APIServer) handleRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	var req struct {
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	err := s.fs.Rename(req.OldPath, req.NewPath)
	statusCode := fuseErrorToHTTP(err)
	writeJSON(w, statusCode, Response{Error: err})
}

// ============ Binary File I/O ============

func (s *APIServer) handleFileRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	offset := int64(0)
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.ParseInt(o, 10, 64); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Get file stats to determine size
	stat := &fuse.Stat_t{}
	err := s.fs.Getattr(path, stat, 0)
	if err != 0 {
		statusCode := fuseErrorToHTTP(err)
		w.WriteHeader(statusCode)
		return
	}

	// Open file
	errOpen, fh := s.fs.Open(path, 0)
	if errOpen != 0 {
		statusCode := fuseErrorToHTTP(errOpen)
		w.WriteHeader(statusCode)
		return
	}

	// Read file content
	buff := make([]byte, stat.Size-offset)
	bytesRead := s.fs.Read(path, buff, offset, fh)

	if bytesRead < 0 {
		statusCode := fuseErrorToHTTP(bytesRead)
		w.WriteHeader(statusCode)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(buff[:bytesRead])
}

func (s *APIServer) handleFileWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	offset := int64(0)
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.ParseInt(o, 10, 64); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Read binary data from request body
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, Response{Error: -22})
		return
	}

	// Open file (create if doesn't exist)
	errOpen, fh := s.fs.Open(path, 0)
	if errOpen != 0 {
		// Try creating
		errCreate, fh := s.fs.Create(path, 2, 0644)
		if errCreate != 0 {
			statusCode := fuseErrorToHTTP(errCreate)
			writeJSON(w, statusCode, Response{Error: errCreate})
			return
		}
		defer func() {
			s.handleMutex.Lock()
			for id, handle := range s.handleMap {
				if handle.fh == fh {
					delete(s.handleMap, id)
				}
			}
			s.handleMutex.Unlock()
		}()

		// Write data
		bytesWritten := s.fs.Write(path, data, offset, fh)
		if bytesWritten < 0 {
			statusCode := fuseErrorToHTTP(bytesWritten)
			writeJSON(w, statusCode, Response{Error: bytesWritten})
		} else {
			writeJSON(w, http.StatusOK, Response{Error: 0, Data: map[string]int{"bytesWritten": bytesWritten}})
		}
		return
	}

	defer func() {
		s.handleMutex.Lock()
		for id, handle := range s.handleMap {
			if handle.fh == fh {
				delete(s.handleMap, id)
			}
		}
		s.handleMutex.Unlock()
	}()

	// Write data
	bytesWritten := s.fs.Write(path, data, offset, fh)
	if bytesWritten < 0 {
		statusCode := fuseErrorToHTTP(bytesWritten)
		writeJSON(w, statusCode, Response{Error: bytesWritten})
	} else {
		writeJSON(w, http.StatusOK, Response{Error: 0, Data: map[string]int{"bytesWritten": bytesWritten}})
	}
}

// ============ Filesystem Endpoints ============

func (s *APIServer) handleStatfs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, Response{Error: -1})
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	stat := &fuse.Statfs_t{}
	err := s.fs.Statfs(path, stat)
	statusCode := fuseErrorToHTTP(err)
	writeJSON(w, statusCode, Response{Error: err, Data: stat})
}
