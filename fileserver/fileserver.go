package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
type Server interface {
	Upload(filecontent []byte)
	Fetch(filehash string) []byte
}

// should expose endpoints for the methods below
// explre nginx
type server struct {
	keydir map[string]string
	mutex  *sync.Mutex
}

const _dirName = "fileserver"

func (s *server) handleGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("filepath")
	if key == "" {
		http.Error(w, "Key is required", http.StatusBadRequest)
		return
	}

	s.mutex.Lock()
	hash, ok := s.keydir[key]
	s.mutex.Unlock()

	if !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	fmt.Fprintln(w, hash)
}

func (s *server) handleSet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "Key is required", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	value := string(body)

	s.mutex.Lock()
	s.keydir[key] = value
	s.mutex.Unlock()

	fmt.Fprintln(w, "Set successful")
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/get", handleGet)
	mux.HandleFunc("/set", handleSet)

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

	err = os.WriteFile(_dirName+filename, filecontent, os.ModePerm)

}

// looks up in the index to get the hash (which is the location) and returns the file if there
func (s *server) Fetch(filehash string) {}
