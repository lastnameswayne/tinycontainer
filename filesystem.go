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
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/lastnameswayne/tinycontainer/tarread"
)

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

func NewFS(path string) *FS {
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
	files := map[string]*file{}
	return &Directory{
		attr: fuse.Attr{
			Atime: now,
			Mtime: now,
			Ctime: now,
			Mode:  uint32(os.ModeDir),
		},
		children: children,
		files:    files,
		path:     path,
		fs:       fs,
		KeyDir:   make(map[string]string),
	}
}

func (r *FS) ensureDir(ctx context.Context, current, parent *Directory, fullPath string) *Directory {
	if parent != nil {
		current = parent
	}
	parts := strings.Split(fullPath, "/")
	for i, part := range parts {
		if i == 0 {
			continue
		}
		if current.children == nil {
			current.children = make(map[string]*Directory)
		}
		if child, exists := current.children[part]; exists {
			fmt.Println("child exists")
			current = child
		} else {
			newDir := r.newDir(strings.Join(parts[:i+1], "/"))
			newNode := r.NewPersistentInode(ctx, newDir, fs.StableAttr{Mode: syscall.S_IFDIR})
			current.AddChild(part, newNode, false)
			current.children[part] = newDir
			current = newDir
		}
	}

	return current
}

func (fs *FS) newFile(path, name string, currentDir *Directory) *file {
	n := time.Now()
	now := uint64(n.UnixMilli())

	file := &file{
		attr: fuse.Attr{
			Atime: now,
			Mtime: now,
			Ctime: now,
			Mode:  uint32(0644),
		},
		path: path,
		fs:   fs,
	}

	currentDir.files[name] = file

	return file
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
	files    map[string]*file      // file name to object
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
	entries := []fuse.DirEntry{}

	// For each child in d.children
	for name, childDir := range d.children {
		// childDir.Inode is a directory => Mode bit for directory
		entries = append(entries, fuse.DirEntry{
			Name: name,
			Mode: fuse.S_IFDIR,
			Ino:  childDir.StableAttr().Ino,
		})
	}

	fileEntries, err := d.getDirectoryContentsFromFileServer()
	if err != nil {
		return nil, 1
	}

	for _, entry := range fileEntries {
		file := d.mapEntryToFile(entry)
		df := d.NewInode(
			ctx, file,
			fs.StableAttr{Ino: 0},
		)

		entries = append(entries, fuse.DirEntry{
			Name: entry.Name,
			Mode: uint32(entry.Mode),
			Ino:  df.StableAttr().Ino,
		})
	}

	return &CustomDirStream{entries: entries}, 0
}

var _ = (fs.NodeLookuper)((*Directory)(nil))

//the worker executes the containers

func (d *Directory) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Println("called lookup on dir", d.path, d.children)
	if childDir, found := d.children[name]; found {
		fmt.Println("Found child in map", name, d.children)
		return &childDir.Inode, 0
	}

	path := filepath.Join(d.path, name)
	fmt.Println("path is", path)

	isFile, err := d.isFile(name)
	if err != nil {
		return nil, syscall.ENOENT
	}
	if !isFile {
		return &d.fs.ensureDir(ctx, d, d.parent, path).Inode, 0
	}

	fmt.Println("looking in cache", d.KeyDir)
	hash, ok := d.KeyDir[d.path+"/"+name]
	for key := range d.KeyDir {
		fmt.Println(key)
	}
	if ok {
		fmt.Println("ok", ok, "hash", hash)
		_, err := os.Stat("./" + hash)
		fileExists := !errors.Is(err, os.ErrNotExist)
		if fileExists {
			path = "./" + hash
			return &d.fs.newFile(path, name, d).Inode, 0
		}
	}

	file, hash, err := d.getFileFromFileServer(name)
	if err != nil {
		return nil, 1
	}
	df := d.NewInode(
		ctx, file,
		fs.StableAttr{Ino: 0},
	)

	d.AddChild(name, df, false)
	d.KeyDir[d.path+"/"+name] = hash
	d.files[name] = file
	return df, 0

}

func (d *Directory) isFile(name string) (bool, error) {
	fmt.Println("Checking if", name, "is a file")
	entry, err := d.getDataFromFileServer(name)
	if err != nil {
		fmt.Println("Error occurred while checking if", name, "is a file:", err)
		return false, err
	}
	isFile := !entry.IsDir
	if !isFile {
		fmt.Println(name, "is a file")
	}
	return !entry.IsDir, nil
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

	var entries []KeyValue
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return KeyValue{}, fmt.Errorf("error decoding response: %w", err)
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

func (d *Directory) getFileFromFileServer(name string) (*file, string, error) {
	entry, err := d.getDataFromFileServer(name)
	if err != nil {
		return nil, "", err
	}

	return d.mapEntryToFile(entry), entry.HashValue, nil
}

func (d *Directory) mapEntryToFile(entry KeyValue) *file {
	file := &file{
		Data: entry.Value,
		rc:   d.rc,
		path: "./" + entry.HashValue,
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
