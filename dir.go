package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

var _ = (fs.NodeReaddirer)((*Directory)(nil))
var _ = (fs.NodeLookuper)((*Directory)(nil))

// Readdir lists the contents of the directory
func (d *Directory) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries := make(map[string]fuse.DirEntry, 0)
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
			newDir := d.fs.newDir(filepath.Join(d.path, entry.Name))
			newDir.parent = d
			newNode := d.NewPersistentInode(ctx, newDir, fs.StableAttr{Mode: syscall.S_IFDIR})
			d.AddChild(entry.Name, newNode, false)
			d.children[entry.Name] = newDir
		} else {
			file := &file{
				path: _cacheDir + "/" + entry.HashValue,
				attr: fuse.Attr{
					Mode: uint32(entry.Mode),
					Size: uint64(entry.Size),
				},
			}
			df := d.NewInode(ctx, file, fs.StableAttr{Ino: 0})
			d.AddChild(entry.Name, df, false)
			d.keyDir[d.path+"/"+entry.Name] = entry.HashValue
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

	hash, ok := d.keyDir[d.path+"/"+name]
	if ok {
		if hash == _NOT_FOUND {
			return nil, syscall.ENOENT
		}
		cachedData, err := os.ReadFile(_cacheDir + "/" + hash)
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
			d.keyDir[d.path+"/"+name] = _NOT_FOUND
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
	d.keyDir[d.path+"/"+name] = hash

	if cacheData, err := json.Marshal(entry); err == nil {
		if err := os.WriteFile(_cacheDir+"/"+hash, cacheData, 0644); err != nil {
			fmt.Println("Error writing file to disk cache:", err)
		}
	}

	return df, 0
}

func (d *Directory) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	fmt.Println("CALLED GETATTR for", d.attr)
	out.Mode = syscall.S_IFDIR | 0755
	out.Nlink = 2
	return 0
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

func (d *Directory) mapEntryToFile(entry KeyValue) *file {
	file := &file{
		Data: entry.Value,
		rc:   d.rc,
		path: _cacheDir + "/" + entry.HashValue,
	}
	file.attr.Mode = uint32(entry.Mode)
	file.attr.Size = uint64(entry.Size)
	file.attr.Gid = uint32(entry.Gid)

	return file
}
