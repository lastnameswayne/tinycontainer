package main

import (
	"crypto/sha1"
	"crypto/tls"
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
type server struct {
	keydir map[string]string
	mutex  *sync.Mutex
}

func NewServer() server {
	err := os.MkdirAll("./"+_dirName, os.ModePerm)
	if err != nil {
		panic(err)
	}

	return server{
		keydir: map[string]string{},
		mutex:  &sync.Mutex{},
	}

}

const _dirName = "fileserverfiles"

func (s *server) handleGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("filepath")
	if key == "" {
		http.Error(w, "filepath is required", http.StatusBadRequest)
		return
	}

	s.mutex.Lock()
	hash, ok := s.keydir[key]
	s.mutex.Unlock()
	for ket := range s.keydir {
		fmt.Println(ket)
	}

	if !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	file, err := os.OpenFile(_dirName+"/"+hash, os.O_RDWR, 0644)
	if err != nil {
		panic(err)
	}

	filecontent, err := io.ReadAll(file)

	fmt.Fprintln(w, hash, filecontent)
}

// KeyValue represents the JSON structure for set requests
type KeyValue struct {
	Key   string `json:"key"`
	Value []byte `json:"value"` // Base64 encoded string of the binary data
}

func (s *server) handleSet(w http.ResponseWriter, r *http.Request) {
	fmt.Println("received set")
	var kv KeyValue
	err := json.NewDecoder(r.Body).Decode(&kv)
	if err != nil {
		http.Error(w, "Error decoding JSON", http.StatusBadRequest)
		return
	}
	fileContent := kv.Value

	//hash of decodedValue
	h := sha1.New()
	h.Write(fileContent)
	hash := h.Sum(nil)
	encoded := hex.EncodeToString(hash)

	s.mutex.Lock()
	s.keydir[kv.Key] = encoded
	s.mutex.Unlock()

	parts := strings.Split(encoded, "/")
	dirNames := parts[:len(parts)-1]
	if len(parts) != 1 {
		err := os.MkdirAll(strings.Join(dirNames, "/"), 0666)
		if err != nil {
			panic(err)
		}
	}

	err = os.WriteFile(_dirName+"/"+encoded, fileContent, os.ModePerm)
	if err != nil {
		panic(err)
	}

	fmt.Fprintln(w, "Set successful")
}

func main() {
	mux := http.NewServeMux()
	s := NewServer()
	mux.HandleFunc("/upload", s.handleSet)
	mux.HandleFunc("/fetch", s.handleGet)

	// Configure TLS for HTTP/2
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// For real applications ensure your settings are secure
	}
	server := &http.Server{
		Addr:      ":8443",
		Handler:   mux,
		TLSConfig: tlsCfg,
	}

	// Generate your own certificate and key or use Let's Encrypt in real-world applications
	log.Println("Starting HTTP/2 server on https://localhost:8443")
	log.Fatal(server.ListenAndServeTLS("server.crt", "server.key"))
}

// goal: load a docker tar ball into a cache

// given a file, hash it and store it in a map, write to SSD in a content-addressed way
// can use the bitcask for this
func (s *server) Upload(filename string, filecontent []byte) {
	err := os.MkdirAll(_dirName, os.ModePerm)
	if err != nil {
		panic(err)
	}

}

// looks up in the index to get the hash (which is the location) and returns the file if there
func (s *server) Fetch(filehash string) {}
