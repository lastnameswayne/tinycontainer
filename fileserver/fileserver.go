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
	"strings"
	"sync"
)

// file
// open tar and content address all the files

//this one gets called when runc wants to read a file and its not cached
//on the local worker (the worker is in the cloud also)
//so on startup we build an index
//should laos be content addressed
//build images using docker
//docker save to get a tar file
//then we can checksum every file ()
//this one has an index of the same form as the worker

// Upload depends on if the worker reads the dockerfile on startup and then sends everything
// over here. Or if tje
// Fetch is always needed

// should expose endpoints for the methods below
// explre nginx
const defaultDirName = "fileserverfiles"

type server struct {
	keydir           map[string]string // file name to hash
	mutex            *sync.Mutex
	dirName          string
	knownDirectories map[string]map[string]struct{} //directory name to list of hashes. Each hash is the child.
}

func NewServer() server {
	return NewServerWithDir(defaultDirName)
}

func NewServerWithDir(dirName string) server {
	err := os.MkdirAll(dirName, os.ModePerm)
	if err != nil {
		panic(err)
	}

	return server{
		keydir:           map[string]string{},
		mutex:            &sync.Mutex{},
		dirName:          dirName,
		knownDirectories: map[string]map[string]struct{}{},
	}
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

	s.mutex.Lock()
	defer s.mutex.Unlock()

	isDirRequest := key[len(key)-1] == '/'
	if isDirRequest {
		dir := key[:len(key)-1] // Remove trailing slash
		fmt.Println("received get for directory", dir)

		hashes, ok := s.knownDirectories[dir]
		if !ok {
			http.Error(w, "Directory not found", http.StatusNotFound)
			return
		}

		entries := []KeyValue{}
		for hash, _ := range hashes {
			content, err := os.ReadFile(s.dirName + "/" + hash)
			if err != nil {
				continue
			}
			var entry KeyValue
			if err := json.Unmarshal(content, &entry); err != nil {
				continue
			}
			entry.HashValue = hash
			entries = append(entries, entry)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
		return
	}

	fmt.Println("received get for file", key)
	hash, ok := s.keydir[key]
	fmt.Println("key", key, "hash", hash, "ok", ok)

	if !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	file, err := os.OpenFile(s.dirName+"/"+hash, os.O_RDWR, 0644)
	if err != nil {
		http.Error(w, "Error opening file", http.StatusInternalServerError)
		return
	}

	filecontent, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
		return
	}

	entry := KeyValue{}
	json.Unmarshal(filecontent, &entry)

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

func (s *server) handleSet(w http.ResponseWriter, r *http.Request) {
	entry := KeyValue{}
	err := json.NewDecoder(r.Body).Decode(&entry)
	if err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	//hash of decodedValue
	h := sha1.New()
	h.Write(entry.Value)
	hash := h.Sum(nil)
	encoded := hex.EncodeToString(hash)

	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.keydir[entry.Key] = encoded
	fmt.Println("set key", entry.Key, "of size", len(entry.Value))

	// Add to parent's directory listing
	if entry.Parent != "" {
		if _, ok := s.knownDirectories[entry.Parent]; !ok {
			s.knownDirectories[entry.Parent] = map[string]struct{}{}
		}
		s.knownDirectories[entry.Parent][encoded] = struct{}{}
	}

	marshalledEntry, err := json.Marshal(entry)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(s.dirName+"/"+encoded, marshalledEntry, os.ModePerm)
	if err != nil {
		panic(err)
	}

	fmt.Fprintln(w, "Set successful")
	marshaledIndex, err := json.Marshal(s.keydir)
	if err != nil {
		return
	}
	os.WriteFile("index.json", marshaledIndex, 0755)
}

func (s *server) handleSetBatch(w http.ResponseWriter, r *http.Request) {
	var entries []KeyValue
	err := json.NewDecoder(r.Body).Decode(&entries)
	if err != nil {
		fmt.Println("Invalid JSON")
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, "Received request %d files\n", len(entries))

	s.mutex.Lock()
	defer s.mutex.Unlock()

	stored := 0
	for _, entry := range entries {
		h := sha1.New()
		if entry.IsDir {
			h.Write([]byte(entry.Key)) // Include key so directories get unique hashes
		}
		h.Write(entry.Value)
		hash := h.Sum(nil)
		encoded := hex.EncodeToString(hash)

		marshalledEntry, err := json.Marshal(entry)
		if err != nil {
			panic(err)
		}
		err = os.WriteFile(s.dirName+"/"+encoded, marshalledEntry, os.ModePerm)
		if err != nil {
			panic(err)
		}

		s.keydir[entry.Key] = encoded

		// Add to parent's directory listing
		if entry.Parent != "" {
			if _, ok := s.knownDirectories[entry.Parent]; !ok {
				s.knownDirectories[entry.Parent] = map[string]struct{}{}
			}
			s.knownDirectories[entry.Parent][encoded] = struct{}{}
		}

		stored = stored + 1
	}

	fmt.Fprintf(w, "Stored %d files\n", stored)
	marshaledIndex, err := json.Marshal(s.keydir)
	if err != nil {
		return
	}
	os.WriteFile("index.json", marshaledIndex, 0755)
}

type ExistsResponse struct {
	KeyValues []KeyValue `json:"keyValues"`
}

// handleList returns directory entries WITHOUT file content (Value field)
// This is much faster for directory listings since we skip the large payloads
func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		http.Error(w, "dir is required", http.StatusBadRequest)
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	fmt.Println("received list for directory", dir)

	hashes, ok := s.knownDirectories[dir]
	if !ok {
		http.Error(w, "Directory not found", http.StatusNotFound)
		return
	}

	// Lightweight entry without Value
	type ListEntry struct {
		Key       string `json:"key"`
		HashValue string `json:"hash_value"`
		Name      string `json:"name"`
		IsDir     bool   `json:"is_dir"`
		Size      int64  `json:"size"`
		Mode      int64  `json:"mode"`
	}

	entries := make([]ListEntry, 0, len(hashes))
	for hash := range hashes {
		content, err := os.ReadFile(s.dirName + "/" + hash)
		if err != nil {
			continue
		}
		var entry KeyValue
		if err := json.Unmarshal(content, &entry); err != nil {
			continue
		}
		entries = append(entries, ListEntry{
			Key:       entry.Key,
			HashValue: hash,
			Name:      entry.Name,
			IsDir:     entry.IsDir,
			Size:      entry.Size,
			Mode:      entry.Mode,
		})
	}

	fmt.Printf("returning %d entries for %s\n", len(entries), dir)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (s *server) handleExists(w http.ResponseWriter, r *http.Request) {
	var entries []KeyValue
	err := json.NewDecoder(r.Body).Decode(&entries)
	if err != nil {
		fmt.Println("Invalid JSON")
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, "Received request %d files\n", len(entries))

	// set of file names
	exists := map[string]struct{}{}
	for _, entry := range entries {
		_, ok := s.keydir[entry.Key]
		if !ok {
			continue
		}
		exists[entry.Key] = struct{}{}
	}

	response := []KeyValue{}
	for _, keyVal := range entries {
		if _, ok := exists[keyVal.Key]; !ok {
			continue
		}
		response = append(response, keyVal)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
	return
}

func main() {
	mux := http.NewServeMux()
	s := NewServer()
	mux.HandleFunc("/upload", s.handleSet)
	mux.HandleFunc("/fetch", s.handleGet)
	mux.HandleFunc("/list", s.handleList)
	mux.HandleFunc("/batch-upload", s.handleSetBatch)
	mux.HandleFunc("/exists", s.handleExists)

	server := &http.Server{
		Addr:    ":8443",
		Handler: mux,
	}

	// Generate your own certificate and key or use Let's Encrypt in real-world applications
	log.Println("Starting server on https://localhost:8443")
	log.Fatal(server.ListenAndServeTLS("server.crt", "server.key"))
}

// goal: load a docker tar ball into a cache
