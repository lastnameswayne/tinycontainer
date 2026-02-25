package main

import (
	"context"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Directory represents a directory in the filesystem
type Directory struct {
	fusefs.Inode
	mu       sync.RWMutex
	keyDir   map[string]cachedMetadata
	attr     fuse.Attr
	path     string
	rootFS   *FS
	parent   *Directory
	children map[string]*Directory // directory name to object
}

type cachedMetadata struct {
	hash string
	mode int64
	size int64
}

var _ = (fusefs.NodeReaddirer)((*Directory)(nil))
var _ = (fusefs.NodeLookuper)((*Directory)(nil))

const _kernelInodeTimeout = 5 * time.Minute

// Readdir lists the contents of the directory
func (d *Directory) Readdir(ctx context.Context) (fusefs.DirStream, syscall.Errno) {
	d.mu.RLock()
	entries := make(map[string]fuse.DirEntry)
	for name, childDir := range d.children {
		entry := fuse.DirEntry{
			Name: name,
			Mode: fuse.S_IFDIR,
			Ino:  childDir.StableAttr().Ino,
		}
		entries[entry.Name] = entry
	}
	d.mu.RUnlock()

	fileEntries, err := d.getContentsFromFileServer()
	if err != nil {
		// Fallback to d.children.
		log.Printf("error getting directory contents: %v", err)
		return dirEntriesToStream(entries), 0
	}

	d.mu.Lock()
	for _, entry := range fileEntries {
		if _, ok := entries[entry.Name]; ok {
			continue
		}
		if entry.IsDir {
			d.addDirChild(ctx, entry.Name)
		} else {
			f := &file{
				path: filepath.Join(_cacheDir, entry.HashValue),
				attr: fuse.Attr{Mode: uint32(entry.Mode), Size: uint64(entry.Size)},
			}
			d.addFileChild(ctx, entry.Name, entry.HashValue, f)
		}

		fuseEntry := fuse.DirEntry{
			Name: entry.Name,
			Mode: uint32(entry.Mode),
			Ino:  0,
		}
		entries[fuseEntry.Name] = fuseEntry
	}
	d.mu.Unlock()

	return dirEntriesToStream(entries), 0
}

func dirEntriesToStream(entries map[string]fuse.DirEntry) fusefs.DirStream {
	return fusefs.NewListDirStream(slices.Collect(maps.Values(entries)))
}

func (d *Directory) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fusefs.Inode, syscall.Errno) {
	key := filepath.Join(d.path, name)
	// Check if directory/file is known to be not found
	if d.rootFS.isNotFound(key) {
		return nil, syscall.ENOENT
	}
	// Skip Python temp files - they'll never exist on server
	if strings.Contains(name, ".pyc.") || strings.Contains(name, ".pyo.") || name == "__pycache__" {
		return nil, syscall.ENOENT
	}
	// We can't cache the user's runscript, as it might change! Needs to be fetched fresh.
	if isScript(name) {
		return d.scriptFromFileserver(ctx, name, out)
	}

	// Directory/File is in memory
	d.mu.RLock()
	if childDir, found := d.children[name]; found {
		d.mu.RUnlock()
		LookupStats.MemoryCacheHits.Add(1)
		return &childDir.Inode, 0
	}
	d.mu.RUnlock()

	// Directory/File is on the disk
	if inode, ok := d.fromDiskCache(ctx, name, key, out); ok {
		return inode, 0
	}

	// Last check: File/Directory has to be on the fileserver.
	return d.fromFileServer(ctx, name, key, out)
}

func (d *Directory) fromFileServer(ctx context.Context, name, key string, out *fuse.EntryOut) (*fusefs.Inode, syscall.Errno) {
	entry, err := d.getEntryFromFileServer(name)
	if err == ErrNotFoundOnFileServer {
		d.rootFS.addNotFound(key)
		return nil, syscall.ENOENT
	}
	if err != nil {
		log.Printf("error fetching file data for %s: %v", name, err)
		return nil, syscall.EIO
	}
	LookupStats.ServerFetches.Add(1)
	if entry.IsDir {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.addDirChild(ctx, name), 0
	}

	f := mapEntryToFile(entry)
	d.mu.Lock()
	df := d.addFileChild(ctx, name, entry.HashValue, f)
	d.mu.Unlock()
	if err := os.WriteFile(filepath.Join(_cacheDir, entry.HashValue), entry.Value, 0644); err != nil {
		log.Printf("error writing file to disk cache: %v", err)
	}
	setFileEntryOut(out, f.attr.Mode, f.attr.Size)
	return df, 0
}

func (d *Directory) Getattr(ctx context.Context, f fusefs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = syscall.S_IFDIR | 0755
	out.Nlink = 2
	return 0
}

func mapCachedEntryToFile(cachedMetadata cachedMetadata, binaryData []byte) *file {
	file := &file{
		Data: binaryData,
		path: filepath.Join(_cacheDir, cachedMetadata.hash),
	}
	file.attr.Mode = uint32(cachedMetadata.mode)
	file.attr.Size = uint64(cachedMetadata.size)

	return file
}

func mapEntryToFile(entry KeyValue) *file {
	if int64(len(entry.Value)) != entry.Size {
		log.Printf("SIZE MISMATCH for %s: Value len=%d, Size=%d", entry.Name, len(entry.Value), entry.Size)
	}
	file := &file{
		Data: entry.Value,
		path: filepath.Join(_cacheDir, entry.HashValue),
	}
	file.attr.Mode = uint32(entry.Mode)
	file.attr.Size = uint64(entry.Size)

	return file
}

func setFileEntryOut(out *fuse.EntryOut, mode uint32, size uint64) {
	out.Attr.Mode = mode | 0777
	out.Attr.Size = size
	out.Attr.Nlink = 1
	out.SetEntryTimeout(_kernelInodeTimeout)
	out.SetAttrTimeout(_kernelInodeTimeout)
}

func (d *Directory) fromDiskCache(ctx context.Context, name, key string, out *fuse.EntryOut) (*fusefs.Inode, bool) {
	d.mu.RLock()
	metadata, ok := d.keyDir[key]
	d.mu.RUnlock()
	if !ok {
		return nil, false
	}
	binaryData, err := os.ReadFile(filepath.Join(_cacheDir, metadata.hash))
	if err != nil {
		return nil, false
	}
	LookupStats.DiskCacheHits.Add(1)
	d.mu.Lock()
	inode := d.addFileChild(ctx, name, "", mapCachedEntryToFile(metadata, binaryData))
	d.mu.Unlock()
	setFileEntryOut(out, uint32(metadata.mode), uint64(metadata.size))
	return inode, true
}

func (d *Directory) scriptFromFileserver(ctx context.Context, name string, out *fuse.EntryOut) (*fusefs.Inode, syscall.Errno) {
	entry, err := d.getEntryFromFileServer(name)
	if err != nil {
		return nil, syscall.EIO
	}
	LookupStats.ServerFetches.Add(1)
	out.SetEntryTimeout(0)
	out.SetAttrTimeout(0)
	df := d.NewInode(ctx, mapEntryToFile(entry), fusefs.StableAttr{Ino: 0})
	d.mu.Lock()
	d.AddChild(name, df, true) // overwrite=true: always replace stale script inodes
	d.mu.Unlock()
	return df, 0
}

func (d *Directory) addFileChild(ctx context.Context, name, hash string, f *file) *fusefs.Inode {
	df := d.NewInode(ctx, f, fusefs.StableAttr{Ino: 0})
	d.AddChild(name, df, false)
	if hash != "" {
		d.keyDir[filepath.Join(d.path, name)] = cachedMetadata{
			hash: hash,
			size: int64(f.attr.Size),
			mode: int64(f.attr.Mode),
		}
	}
	return df
}

func (d *Directory) addDirChild(ctx context.Context, name string) *fusefs.Inode {
	if dir, ok := d.children[name]; ok {
		return &dir.Inode
	}
	newDir := d.rootFS.newDir(filepath.Join(d.path, name))
	newDir.parent = d
	node := d.NewPersistentInode(ctx, newDir, fusefs.StableAttr{Mode: syscall.S_IFDIR})
	d.AddChild(name, node, false)
	d.children[name] = newDir
	return node
}
