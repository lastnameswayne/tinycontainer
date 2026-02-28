package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const defaultDirName = "fileserverfiles"

type server struct {
	keydir           map[string]string // file path to content hash
	mu               sync.RWMutex
	dirName          string
	knownDirectories map[string]map[string]struct{} // directory path to set of child hashes
}

func NewServer() server {
	return NewServerWithDir(defaultDirName)
}

func NewServerWithDir(dirName string) server {
	err := os.MkdirAll(dirName, os.ModePerm)
	if err != nil {
		panic(err)
	}

	s := server{
		keydir:           map[string]string{},
		dirName:          dirName,
		knownDirectories: map[string]map[string]struct{}{},
	}
	if err := s.buildIndex(); err != nil {
		log.Printf("buildIndex: %v", err)
	}
	return s
}

// buildIndex scans all blob files in s.dirName and reconstructs keydir and knownDirectories.
// Each blob file is a JSON-encoded KeyValue whose filename is the content hash.
func (s *server) buildIndex() error {
	entries, err := os.ReadDir(s.dirName)
	if err != nil {
		return fmt.Errorf("reading dir %s: %w", s.dirName, err)
	}
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		hash := de.Name()
		content, err := os.ReadFile(filepath.Join(s.dirName, hash))
		if err != nil {
			log.Printf("buildIndex: skipping %s: %v", hash, err)
			continue
		}
		var entry KeyValue
		if err := json.Unmarshal(content, &entry); err != nil {
			log.Printf("buildIndex: skipping %s (bad JSON): %v", hash, err)
			continue
		}
		s.keydir[entry.Key] = hash
		if entry.Parent != "" {
			if _, ok := s.knownDirectories[entry.Parent]; !ok {
				s.knownDirectories[entry.Parent] = map[string]struct{}{}
			}
			s.knownDirectories[entry.Parent][hash] = struct{}{}
		}
	}
	log.Printf("buildIndex: loaded %d entries from %s", len(s.keydir), s.dirName)
	return nil
}

func (s *server) handleGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("filepath")

	if key == "" {
		http.Error(w, "filepath is required", http.StatusBadRequest)
		return
	}

	// Filter out non-essential files that cause slowdowns
	if strings.HasSuffix(key, ".json") || strings.HasSuffix(key, ".txt") {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	isDirRequest := key[len(key)-1] == '/'
	if isDirRequest {
		dir := key[:len(key)-1] // Remove trailing slash
		log.Printf("received get for directory %s", dir)

		s.mu.RLock()
		hashes, ok := s.knownDirectories[dir]
		hashSlice := make([]string, 0, len(hashes))
		for h := range hashes {
			hashSlice = append(hashSlice, h)
		}
		s.mu.RUnlock()

		if !ok {
			http.Error(w, "Directory not found", http.StatusNotFound)
			return
		}

		entries := []KeyValue{}
		for _, hash := range hashSlice {
			content, err := os.ReadFile(filepath.Join(s.dirName, hash))
			if err != nil {
				continue
			}
			var entry KeyValue
			if err := json.Unmarshal(content, &entry); err != nil {
				continue
			}
			entry.HashValue = hash
			entry.Value = nil
			entries = append(entries, entry)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
		return
	}

	log.Printf("received get for file %s", key)
	s.mu.RLock()
	hash, ok := s.keydir[key]
	s.mu.RUnlock()
	log.Printf("key=%s hash=%s ok=%v", key, hash, ok)

	if !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	file, err := os.Open(filepath.Join(s.dirName, hash))
	if err != nil {
		http.Error(w, "Error opening file", http.StatusInternalServerError)
		return
	}

	filecontent, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
		return
	}

	var entry KeyValue
	if err := json.Unmarshal(filecontent, &entry); err != nil {
		log.Printf("corrupt file for hash %s: %v", hash, err)
		http.Error(w, "corrupt file", http.StatusInternalServerError)
		return
	}

	entry.HashValue = hash
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entry)
}

// KeyValue represents the JSON structure for set requests
type KeyValue struct {
	Key       string `json:"key"`
	Value     []byte `json:"value"`      // Base64 encoded binary data
	HashValue string `json:"hash_value"` // Content hash for caching
	Parent    string `json:"parent"`
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	Mode      int64  `json:"mode"`
	ModTime   int64  `json:"mod_time"`
	Uid       int    `json:"uid"`
	Gid       int    `json:"gid"`
}

func (s *server) handleSetBatch(w http.ResponseWriter, r *http.Request) {
	var entries []KeyValue
	err := json.NewDecoder(r.Body).Decode(&entries)
	if err != nil {
		log.Printf("invalid JSON in batch upload: %v", err)
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, "Received request %d files\n", len(entries))

	stored := 0
	for _, entry := range entries {
		h := sha1.New()
		if entry.IsDir {
			h.Write([]byte(entry.Key)) // Include key so directories get unique hashes
		}
		h.Write(entry.Value)
		encoded := hex.EncodeToString(h.Sum(nil))

		marshalledEntry, err := json.Marshal(entry)
		if err != nil {
			log.Printf("failed to marshal entry for key=%s: %v", entry.Key, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(filepath.Join(s.dirName, encoded), marshalledEntry, os.ModePerm); err != nil {
			log.Printf("failed to write file for key=%s: %v", entry.Key, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		s.mu.Lock()
		s.keydir[entry.Key] = encoded
		// Add to parent's directory listing
		if entry.Parent != "" {
			if _, ok := s.knownDirectories[entry.Parent]; !ok {
				s.knownDirectories[entry.Parent] = map[string]struct{}{}
			}
			s.knownDirectories[entry.Parent][encoded] = struct{}{}
		}
		s.mu.Unlock()

		stored++
	}

	fmt.Fprintf(w, "Stored %d files\n", stored)
}

// SyncEntry is metadata sent by client for sync comparison
type SyncEntry struct {
	Key  string `json:"key"`
	Hash string `json:"hash"` // client-computed content hash
}

type SyncResponse struct {
	NeedUpload []string `json:"need_upload"`
}

func (s *server) handleSync(w http.ResponseWriter, r *http.Request) {
	var entries []SyncEntry
	err := json.NewDecoder(r.Body).Decode(&entries)
	if err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("sync: received %d file hashes", len(entries))

	s.mu.RLock()
	defer s.mu.RUnlock()

	needUpload := []string{}
	for _, entry := range entries {
		existingHash, exists := s.keydir[entry.Key]
		if !exists || existingHash != entry.Hash {
			needUpload = append(needUpload, entry.Key)
		}
	}

	log.Printf("sync: %d files need upload", len(needUpload))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SyncResponse{NeedUpload: needUpload})
}

func main() {
	mux := http.NewServeMux()
	s := NewServer()
	mux.HandleFunc("/fetch", s.handleGet)
	mux.HandleFunc("/batch-upload", s.handleSetBatch)
	mux.HandleFunc("/sync", s.handleSync)

	server := &http.Server{
		Addr:    ":8443",
		Handler: mux,
	}

	log.Println("Starting server on https://localhost:8443")
	log.Fatal(server.ListenAndServeTLS("server.crt", "server.key"))
}
