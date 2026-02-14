package main

import (
	"sync/atomic"
)

// LookupStats tracks cache hit/miss statistics for Lookup operations
var LookupStats struct {
	MemoryCacheHits atomic.Int64 // Found in children map
	DiskCacheHits   atomic.Int64 // Found in disk cache via KeyDir
	ServerFetches   atomic.Int64 // Had to fetch from fileserver
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
