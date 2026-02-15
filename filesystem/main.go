package main

import (
	"flag"
	"log"
	"net/http"
	"os/exec"

	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/lastnameswayne/tinycontainer/db"
)

func main() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  hello MOUNTPOINT")
	}
	opts := &fusefs.Options{}
	cmd := exec.Command("umount", flag.Arg(0))
	err := cmd.Run()
	if err != nil {
		log.Default().Printf("Command umount execution failed: %v", err)
	}

	// Initialize database for run logging
	if err := db.Init("runs.db"); err != nil {
		log.Printf("Warning: failed to initialize database: %v", err)
	}

	// start up web server
	handler := http.NewServeMux()
	handler.HandleFunc("/run", Run)
	handler.HandleFunc("/run/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./website/index.html")
	})
	handler.HandleFunc("/stats", Stats)
	handler.Handle("/", http.FileServer(http.Dir("./website")))
	httpserver := &http.Server{
		Addr:    ":8444",
		Handler: handler,
	}
	go func() {
		log.Println("Starting HTTP server on :8444")
		if err := httpserver.ListenAndServe(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	//init root
	opts.Debug = true
	root := NewFS(flag.Arg(0))
	root.root = root.newDir("/") // Explicitly set the root directory
	server, err := fusefs.Mount(flag.Arg(0), root, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}
	server.Wait()
}
