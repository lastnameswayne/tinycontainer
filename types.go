package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// LookupStats tracks cache hit/miss statistics for Lookup operations
var LookupStats struct {
	MemoryCacheHits atomic.Int64 // Found in children map
	DiskCacheHits   atomic.Int64 // Found in disk cache via KeyDir
	ServerFetches   atomic.Int64 // Had to fetch from fileserver
}

var ErrNotFoundOnFileServer = fmt.Errorf("NOT FOUND ON FILESERVER")

// We use this to cache directories we know are not on the fileserver to avoid attempting a re-fetch.
const _NOT_FOUND = "NOT_FOUND"

// FS is the root filesystem
type FS struct {
	fs.Inode

	root   *Directory
	nodeId uint64
	path   string
	size   int64
	client *http.Client
	KeyDir map[string]string
}

// Directory represents a directory in the filesystem
type Directory struct {
	fs.Inode
	rc       io.Reader
	keyDir   map[string]string // map from name --> hash
	attr     fuse.Attr
	path     string
	fs       *FS
	parent   *Directory
	children map[string]*Directory // directory name to object
}

// file represents a file in the filesystem
type file struct {
	fs.Inode
	rc   io.Reader
	Data []byte
	attr fuse.Attr
	mu   sync.Mutex
	path string
	fs   *FS
}

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

// ListEntry is a lightweight entry for directory listings (no file content)
type ListEntry struct {
	Key       string `json:"key"`
	HashValue string `json:"hash_value"`
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	Mode      int64  `json:"mode"`
}

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
