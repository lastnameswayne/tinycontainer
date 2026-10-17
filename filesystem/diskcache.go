package main

import (
	"container/list"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// diskCacheLRU bounds the size of filecache/ with least-recently-used eviction.
// The bound comes from SWAY_DISK_CACHE_BYTES; 0 or unset means unbounded (the
// original behaviour). Added for the cache-tier experiment (E5).
type diskCacheLRU struct {
	mu       sync.Mutex
	capBytes int64
	total    int64
	order    *list.List // front = most recently used; values are hashes
	entries  map[string]*list.Element
	sizes    map[string]int64
	evicted  int64
}

var diskCache = newDiskCacheLRU()

func newDiskCacheLRU() *diskCacheLRU {
	c := &diskCacheLRU{
		order:   list.New(),
		entries: map[string]*list.Element{},
		sizes:   map[string]int64{},
	}
	if v, err := strconv.ParseInt(os.Getenv("SWAY_DISK_CACHE_BYTES"), 10, 64); err == nil && v > 0 {
		c.capBytes = v
	}
	return c
}

// load indexes files already in the cache directory, e.g. after a worker restart.
func (c *diskCacheLRU) load(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() {
			continue
		}
		c.added(e.Name(), info.Size())
	}
	log.Printf("disk cache: %d files, %d bytes, cap %d bytes", len(c.sizes), c.total, c.capBytes)
}

func (c *diskCacheLRU) added(hash string, size int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[hash]; ok {
		c.order.MoveToFront(el)
		return
	}
	c.entries[hash] = c.order.PushFront(hash)
	c.sizes[hash] = size
	c.total += size
	c.evictLocked(hash)
}

func (c *diskCacheLRU) touched(hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[hash]; ok {
		c.order.MoveToFront(el)
	}
}

// evictLocked removes least recently used files until the cache fits, never evicting keep.
func (c *diskCacheLRU) evictLocked(keep string) {
	if c.capBytes <= 0 {
		return
	}
	for c.total > c.capBytes {
		el := c.order.Back()
		if el == nil {
			return
		}
		hash := el.Value.(string)
		if hash == keep {
			return
		}
		c.order.Remove(el)
		delete(c.entries, hash)
		c.total -= c.sizes[hash]
		delete(c.sizes, hash)
		c.evicted++
		if err := os.Remove(filepath.Join(_cacheDir, hash)); err != nil && !os.IsNotExist(err) {
			log.Printf("disk cache: evict %s: %v", hash, err)
		}
	}
}
