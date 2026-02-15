package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Directory represents a directory in the filesystem
type Directory struct {
	fs.Inode
	rc       io.Reader
	keyDir   map[string]cachedMetadata
	attr     fuse.Attr
	path     string
	fs       *FS
	parent   *Directory
	children map[string]*Directory // directory name to object
}

type cachedMetadata struct {
	hash     string
	mode     int64
	size     int64
	notFound bool
}

var _ = (fs.NodeReaddirer)((*Directory)(nil))
var _ = (fs.NodeLookuper)((*Directory)(nil))

// Readdir lists the contents of the directory
func (d *Directory) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := make(map[string]fuse.DirEntry)
	for name, childDir := range d.children {
		entry := fuse.DirEntry{
			Name: name,
			Mode: fuse.S_IFDIR,
			Ino:  childDir.StableAttr().Ino,
		}
		entries[entry.Name] = entry
	}

	fileEntries, err := d.getContentsFromFileServer()
	if err != nil {
		// Fallback to d.children.
		fmt.Println("Error getting directory contents:", err)
		out := []fuse.DirEntry{}
		for _, entry := range entries {
			out = append(out, entry)
		}
		return fs.NewListDirStream(out), 0
	}

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

	out := []fuse.DirEntry{}
	for _, entry := range entries {
		out = append(out, entry)
	}
	return fs.NewListDirStream(out), 0
}

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

	// We can't cache the user's runscript, as it might change! Needs to be fetched fresh.
	if isScript(name) {
		entry, err := d.getEntryFromFileServer(name)
		if err != nil {
			return nil, syscall.EIO
		}
		LookupStats.ServerFetches.Add(1)
		f := d.mapEntryToFile(entry)
		return d.addFileChild(ctx, name, "", f), 0
	}

	metadata, ok := d.keyDir[filepath.Join(d.path, name)]
	if ok {
		if metadata.notFound {
			return nil, syscall.ENOENT
		}
		binaryData, err := os.ReadFile(filepath.Join(_cacheDir, metadata.hash))
		if err == nil {
			LookupStats.DiskCacheHits.Add(1)
			f := d.mapCachedEntryToFile(metadata, binaryData)
			return d.addFileChild(ctx, name, "", f), 0
		}
	}
	entry, err := d.getEntryFromFileServer(name)
	if err != nil {
		if err == ErrNotFoundOnFileServer {
			d.keyDir[filepath.Join(d.path, name)] = cachedMetadata{notFound: true}
		}
		fmt.Printf("Error fetching file data for %s: %v\n", name, err)
		return nil, syscall.ENOENT
	}
	if entry.IsDir {
		LookupStats.ServerFetches.Add(1)
		return d.addDirChild(ctx, name), 0
	}

	LookupStats.ServerFetches.Add(1)
	f := d.mapEntryToFile(entry)
	df := d.addFileChild(ctx, name, entry.HashValue, f)

	if err := os.WriteFile(_cacheDir+"/"+entry.HashValue, entry.Value, 0644); err != nil {
		fmt.Println("Error writing file to disk cache:", err)
	}
	return df, 0
}

func (d *Directory) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	fmt.Println("CALLED GETATTR for", d.attr)
	out.Mode = syscall.S_IFDIR | 0755
	out.Nlink = 2
	return 0
}

func (d *Directory) mapCachedEntryToFile(cachedMetadata cachedMetadata, binaryData []byte) *file {
	file := &file{
		Data: binaryData,
		rc:   d.rc,
		path: filepath.Join(_cacheDir, cachedMetadata.hash),
	}
	file.attr.Mode = uint32(cachedMetadata.mode)
	file.attr.Size = uint64(cachedMetadata.size)

	return file
}

func (d *Directory) mapEntryToFile(entry KeyValue) *file {
	file := &file{
		Data: entry.Value,
		rc:   d.rc,
		path: filepath.Join(_cacheDir, entry.HashValue),
	}
	file.attr.Mode = uint32(entry.Mode)
	file.attr.Size = uint64(entry.Size)
	file.attr.Gid = uint32(entry.Gid)

	return file
}

func (d *Directory) addFileChild(ctx context.Context, name, hash string, f *file) *fs.Inode {
	df := d.NewInode(ctx, f, fs.StableAttr{Ino: 0})
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

func (d *Directory) addDirChild(ctx context.Context, name string) *fs.Inode {
	newDir := d.fs.newDir(filepath.Join(d.path, name))
	newDir.parent = d
	node := d.NewPersistentInode(ctx, newDir, fs.StableAttr{Mode: syscall.S_IFDIR})
	d.AddChild(name, node, false)
	d.children[name] = newDir
	return node
}
