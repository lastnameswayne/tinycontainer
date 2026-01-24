// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This program is the analogon of libfuse's hello.c, a a program that
// exposes a single file "file.txt" in the root directory.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/lastnameswayne/tinycontainer/db"
	"github.com/lastnameswayne/tinycontainer/tarread"
)

// LookupStats tracks cache hit/miss statistics for Lookup operations
var LookupStats struct {
	MemoryCacheHits atomic.Int64 // Found in children map
	DiskCacheHits   atomic.Int64 // Found in disk cache via KeyDir
	ServerFetches   atomic.Int64 // Had to fetch from fileserver
}

// GetAndResetLookupStats returns current stats and resets counters to 0
func GetAndResetLookupStats() (memoryHits, diskHits, serverFetches int64) {
	memoryHits = LookupStats.MemoryCacheHits.Swap(0)
	diskHits = LookupStats.DiskCacheHits.Swap(0)
	serverFetches = LookupStats.ServerFetches.Swap(0)
	return
}

var ErrNotFoundOnFileServer = fmt.Errorf("NOT FOUND ON FILESERVER")

type FS struct {
	fs.Inode

	root   *Directory
	nodeId uint64
	path   string
	size   int64
	client *http.Client
	KeyDir map[string]string
}

// We use this to cache directories we know are not on the fileserver to avoid attempting a re-fetch.
const _NOT_FOUND = "NOT_FOUND"

// KeyValue represents the JSON structure for set requests
type KeyValue struct {
	Key       string `json:"key"`
	Value     []byte `json:"value"` // Base64 encoded binary data
	HashValue string `json:"hash_value"`
	Parent    string `json:"parent"`
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	Mode      int64  `json:"mode"`
	ModTime   int64  `json:"mod_time"`
	Uid       int    `json:"uid"`
	Gid       int    `json:"gid"`
}

var _ = (fs.NodeStatfser)((*FS)(nil))

const cacheDir = "filecache"

func NewFS(path string) *FS {
	// Create cache directory
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		fmt.Println("Error creating cache directory:", err)
	}

	fs := &FS{
		path:   path,
		KeyDir: map[string]string{},
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	fs.client = client
	fs.root = fs.newDir(path)
	return fs
}

func (fs *FS) newDir(path string) *Directory {
	n := time.Now()
	now := uint64(n.UnixMilli())
	fmt.Println("NEW DIR", path)
	children := map[string]*Directory{}
	return &Directory{
		attr: fuse.Attr{
			Atime: now,
			Mtime: now,
			Ctime: now,
			Mode:  uint32(os.ModeDir),
		},
		children: children,
		path:     path,
		fs:       fs,
		KeyDir:   make(map[string]string),
	}
}

func (f *FS) Root() (*Directory, error) {
	return f.root, nil
}

func (f *FS) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	fmt.Println("CALLED STAT")
	*out = fuse.StatfsOut{
		Bsize:  512,
		Blocks: 10,
		Bavail: 1000,
		Bfree:  1000,
	}
	return 0
}

type Directory struct {
	fs.Inode
	rc     io.Reader
	KeyDir map[string]string // map from name --> hash
	attr   fuse.Attr
	//extra
	path     string
	fs       *FS
	parent   *Directory
	children map[string]*Directory // directory name to object
}

func (r *FS) OnAdd(ctx context.Context) {
	p := r.EmbeddedInode()
	rf := r.newDir("app")
	p.AddChild("app", r.NewPersistentInode(ctx, rf, fs.StableAttr{Mode: syscall.S_IFDIR}), false)

	// these are empty dirs in the linux filesystem
	// They could also be served from the user / fileserver
	homeDir := r.newDir("home")
	homeDir.parent = rf
	homeNode := r.NewPersistentInode(ctx, homeDir, fs.StableAttr{Mode: syscall.S_IFDIR})
	rf.AddChild("home", homeNode, false)
	rf.children["home"] = homeDir

	libDir := r.newDir("lib")
	libDir.parent = rf
	libNode := r.NewPersistentInode(ctx, libDir, fs.StableAttr{Mode: syscall.S_IFDIR})
	rf.AddChild("lib", libNode, false)
	rf.children["lib"] = libDir

	mediaDir := r.newDir("media")
	mediaDir.parent = rf
	mediaNode := r.NewPersistentInode(ctx, mediaDir, fs.StableAttr{Mode: syscall.S_IFDIR})
	rf.AddChild("media", mediaNode, false)
	rf.children["media"] = mediaDir

	mntDir := r.newDir("mnt")
	mntDir.parent = rf
	mntNode := r.NewPersistentInode(ctx, mntDir, fs.StableAttr{Mode: syscall.S_IFDIR})
	rf.AddChild("mnt", mntNode, false)
	rf.children["mnt"] = mntDir

	optDir := r.newDir("opt")
	optDir.parent = rf
	optNode := r.NewPersistentInode(ctx, optDir, fs.StableAttr{Mode: syscall.S_IFDIR})
	rf.AddChild("opt", optNode, false)
	rf.children["opt"] = optDir

	procDir := r.newDir("proc")
	procDir.parent = rf
	procNode := r.NewPersistentInode(ctx, procDir, fs.StableAttr{Mode: syscall.S_IFDIR})
	rf.AddChild("proc", procNode, false)
	rf.children["proc"] = procDir

	devDir := r.newDir("dev")
	devDir.parent = rf
	devNode := r.NewPersistentInode(ctx, devDir, fs.StableAttr{Mode: syscall.S_IFDIR})
	rf.AddChild("dev", devNode, false)
	rf.children["dev"] = devDir

	sysDir := r.newDir("sys")
	sysDir.parent = rf
	sysNode := r.NewPersistentInode(ctx, sysDir, fs.StableAttr{Mode: syscall.S_IFDIR})
	rf.AddChild("sys", sysNode, false)
	rf.children["sys"] = sysDir

	lib64Dir := r.newDir("lib64")
	lib64Dir.parent = rf
	lib64Node := r.NewPersistentInode(ctx, lib64Dir, fs.StableAttr{Mode: syscall.S_IFDIR})
	rf.AddChild("lib64", lib64Node, false)
	rf.children["lib64"] = lib64Dir
}

// Open
// Read
// Release
// Readir
// Readdirplus
// Stat
var _ = (fs.NodeReaddirer)((*Directory)(nil))

// CustomDirStream is a custom implementation of the DirStream interface
type CustomDirStream struct {
	entries []fuse.DirEntry
	index   int
}

// HasNext indicates if there are further entries
func (ds *CustomDirStream) HasNext() bool {
	return ds.index < len(ds.entries)
}

// Next retrieves the next entry
func (ds *CustomDirStream) Next() (fuse.DirEntry, syscall.Errno) {
	if !ds.HasNext() {
		return fuse.DirEntry{}, syscall.ENOENT
	}
	entry := ds.entries[ds.index]
	ds.index++
	return entry, 0
}

// Close releases resources related to this directory stream
func (ds *CustomDirStream) Close() {}

// Readdir lists the contents of the directory
func (d *Directory) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	// Doing this to deduplicate
	entries := make(map[string]fuse.DirEntry, 0)

	for name, childDir := range d.children {
		entry := fuse.DirEntry{
			Name: name,
			Mode: fuse.S_IFDIR,
			Ino:  childDir.StableAttr().Ino,
		}
		entries[entry.Name] = entry
	}

	fileEntries, err := d.getDirectoryContentsFromFileServer()
	if err != nil {
		return nil, 1
	}

	for _, entry := range fileEntries {
		if _, ok := entries[entry.Name]; ok {
			continue
		}
		if entry.IsDir {
			newDir := d.fs.newDir(filepath.Join(d.path, entry.Name))
			newDir.parent = d
			newNode := d.NewPersistentInode(ctx, newDir, fs.StableAttr{Mode: syscall.S_IFDIR})
			d.AddChild(entry.Name, newNode, false)
			d.children[entry.Name] = newDir
		} else {
			file := d.mapEntryToFile(entry)
			df := d.NewInode(
				ctx, file,
				fs.StableAttr{Ino: 0},
			)

			d.AddChild(entry.Name, df, false)
			d.KeyDir[d.path+"/"+entry.Name] = entry.HashValue

			if cacheData, err := json.Marshal(entry); err == nil {
				if err := os.WriteFile(cacheDir+"/"+entry.HashValue, cacheData, 0644); err != nil {
					fmt.Println("Error writing file to disk cache:", err)
				}
			}

		}

		fuseEntry := fuse.DirEntry{
			Name: entry.Name,
			Mode: uint32(entry.Mode),
			Ino:  0,
		}

		entries[fuseEntry.Name] = fuseEntry
	}

	out := []fuse.DirEntry{}
	for _, entry := range entries {
		out = append(out, entry)
	}
	return &CustomDirStream{entries: out}, 0
}

var _ = (fs.NodeLookuper)((*Directory)(nil))

//the worker executes the containers

func (d *Directory) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Println("called lookup on dir", d.path)

	if childDir, found := d.children[name]; found {
		fmt.Println("Found child in map", name)
		LookupStats.MemoryCacheHits.Add(1)
		return &childDir.Inode, 0
	}

	// Skip Python temp files - they'll never exist on server
	if strings.Contains(name, ".pyc.") || strings.Contains(name, ".pyo.") || name == "__pycache__" {
		return nil, syscall.ENOENT
	}

	path := filepath.Join(d.path, name)
	fmt.Println("path is", path)

	// We can't cache the user's runscript, as it might change! Needs to be fetched fresh.
	if isScript(name) {
		entry, _, err := d.getFileFromFileServer(name)
		if err != nil {
			return nil, 1
		}
		LookupStats.ServerFetches.Add(1)
		file := d.mapEntryToFile(*entry)
		df := d.NewInode(
			ctx, file,
			fs.StableAttr{Ino: 0},
		)

		d.AddChild(name, df, false)
		return df, 0
	}

	hash, ok := d.KeyDir[d.path+"/"+name]
	if ok {
		if hash == _NOT_FOUND {
			return nil, syscall.ENOENT
		}
		fmt.Println("ok", ok, "hash", hash)
		cachedData, err := os.ReadFile(cacheDir + "/" + hash)
		if err == nil {
			var entry KeyValue
			if json.Unmarshal(cachedData, &entry) == nil {
				fmt.Println("FOUND FILE ON DISK", hash)
				LookupStats.DiskCacheHits.Add(1)
				file := d.mapEntryToFile(entry)
				df := d.NewInode(ctx, file, fs.StableAttr{Ino: 0})
				d.AddChild(name, df, false)
				return df, 0
			}
		}
	}

	isFile, err := d.isFile(name)
	if err != nil {
		if err == ErrNotFoundOnFileServer {
			d.KeyDir[d.path+"/"+name] = _NOT_FOUND
		}
		return nil, syscall.ENOENT
	}
	if !isFile {
		LookupStats.ServerFetches.Add(1)
		newDir := d.fs.newDir(path)
		newDir.parent = d
		newNode := d.NewPersistentInode(ctx, newDir, fs.StableAttr{Mode: syscall.S_IFDIR})
		d.AddChild(name, newNode, false)
		d.children[name] = newDir
		return newNode, 0
	}

	entry, hash, err := d.getFileFromFileServer(name)
	if err != nil {
		return nil, 1
	}
	LookupStats.ServerFetches.Add(1)
	file := d.mapEntryToFile(*entry)
	df := d.NewInode(
		ctx, file,
		fs.StableAttr{Ino: 0},
	)

	d.AddChild(name, df, false)
	d.KeyDir[d.path+"/"+name] = hash

	if cacheData, err := json.Marshal(entry); err == nil {
		if err := os.WriteFile(cacheDir+"/"+hash, cacheData, 0644); err != nil {
			fmt.Println("Error writing file to disk cache:", err)
		}
	}

	return df, 0

}

func (d *Directory) isFile(name string) (bool, error) {
	fmt.Println("Checking if", name, "is a file", d.path)
	fileEntry, fileErr := d.getDataFromFileServer(name)
	if fileErr != nil {
		fmt.Printf("Error fetching file data for %s: %v\n", name, fileErr)
		return false, fileErr
	}

	fmt.Printf("File entry for %s: %+v\n", name, fileEntry.Name)
	isFile := !fileEntry.IsDir
	if isFile {
		fmt.Println(name, "is a file")
	}
	return isFile, nil
}

func (d *Directory) getDirectoryContentsFromFileServer() ([]KeyValue, error) {
	path := d.path
	if path != "app" {
		path = strings.TrimPrefix(path, "app")
	}
	path = strings.TrimPrefix(path, "/")
	requestUrl := fmt.Sprintf("https://46.101.149.241:8443/fetch?filepath=%s", path+"/")
	fmt.Println("CALLING URL WITH", requestUrl)

	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := d.fs.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFoundOnFileServer
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var entries []KeyValue
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return entries, nil
}

func (d *Directory) getDataFromFileServer(name string) (KeyValue, error) {
	path := d.path
	if path != "app" {
		path = strings.TrimPrefix(path, "app")
	}
	path = strings.TrimPrefix(path, "/")
	requestUrl := fmt.Sprintf("https://46.101.149.241:8443/fetch?filepath=%s", path+"/"+name)
	fmt.Println("CALLING URL WITH", requestUrl)

	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return KeyValue{}, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := d.fs.client.Do(req)
	if err != nil {
		return KeyValue{}, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return KeyValue{}, ErrNotFoundOnFileServer
	}

	if resp.StatusCode != http.StatusOK {
		return KeyValue{}, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var entry KeyValue
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return KeyValue{}, fmt.Errorf("error decoding response: %w", err)
	}

	return entry, nil
}

func (d *Directory) getFileFromFileServer(name string) (*KeyValue, string, error) {
	entry, err := d.getDataFromFileServer(name)
	if err != nil {
		return nil, "", err
	}

	return &entry, entry.HashValue, nil
}

func (d *Directory) mapEntryToFile(entry KeyValue) *file {
	file := &file{
		Data: entry.Value,
		rc:   d.rc,
		path: cacheDir + "/" + entry.HashValue,
	}
	file.attr.Mode = uint32(entry.Mode)
	file.attr.Size = uint64(entry.Size)
	file.attr.Gid = uint32(entry.Gid)

	return file
}

func (d *Directory) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	fmt.Println("CALLED GETATTR for", d.attr)
	out.Mode = syscall.S_IFDIR | 0755
	out.Nlink = 2
	return 0
}

func (f *file) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fmt.Println("CALLING READ", f.path, len(f.Data))
	end := int(off) + len(dest)
	if end > len(f.Data) {
		end = len(f.Data)
	}
	return fuse.ReadResultData(f.Data[off:end]), 0
}

// file is a file
type file struct {
	fs.Inode
	rc   io.Reader
	Data []byte
	attr fuse.Attr
	mu   sync.Mutex
	path string
	fs   *FS
}

func (f *file) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0777
	out.Nlink = 1
	out.Size = f.attr.Size
	const bs = 512
	out.Blksize = bs
	out.Blocks = (out.Size + bs - 1) / bs
	return 0
}

func (f *file) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	fmt.Println("OPENING FILE", f.path)

	if f.Data == nil {
		fmt.Println("Data is nil, attempting to read")
		reader, err := os.Open(f.path)
		if err != nil {
			panic("cant open")
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			return nil, 0, syscall.EIO
		}
		f.Data = content
	}

	return f, uint32(0), 0
}

var _ = (fs.NodeLookuper)((*Directory)(nil))

var _ = (fs.NodeReader)((*file)(nil))
var _ = (fs.NodeOpener)((*file)(nil))

func main() {
	// Initialize database for run logging
	if err := db.Init("runs.db"); err != nil {
		log.Printf("Warning: failed to initialize database: %v", err)
	}

	tarread.Export("archive.tar", "https://46.101.149.241:8443")
	flag.Parse()
	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  hello MOUNTPOINT")
	}
	opts := &fs.Options{}
	cmd := exec.Command("umount", flag.Arg(0))
	err := cmd.Run()
	if err != nil {
		log.Default().Printf("Command umount execution failed: %v", err)
	}
	// expose /run endpoint
	handler := http.NewServeMux()
	handler.HandleFunc("/run", Run)
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
	server, err := fs.Mount(flag.Arg(0), root, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}
	server.Wait()
}

func isScript(filename string) bool {
	// use heurestics to decide
	if path.Ext(filename) != ".py" {
		return false
	}

	parts := strings.Split(filename, "_")
	if len(parts) != 2 {
		return false
	}

	app := parts[1]
	if app != "app.py" {
		return false
	}
	return true
}
