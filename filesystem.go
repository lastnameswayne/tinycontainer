// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This program is the analogon of libfuse's hello.c, a a program that
// exposes a single file "file.txt" in the root directory.
package main

import (
	"archive/tar"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/lastnameswayne/tinycontainer/tarread"
)

type FS struct {
	fs.Inode

	root   *Directory
	nodeId uint64
	path   string
	size   int64
}

var _ = (fs.NodeStatfser)((*FS)(nil))

func NewFS(path string) *FS {
	fs := &FS{
		path: path,
	}

	fs.root = fs.newDir(path, os.ModeDir)
	return fs
}

func (fs *FS) newDir(path string, mode os.FileMode) *Directory {
	n := time.Now()
	now := uint64(n.UnixMilli())
	return &Directory{
		attr: fuse.Attr{
			Ino:     0,
			Atime:   now,
			Mtime:   now,
			Ctime:   now,
			Crtime_: now,
			Mode:    uint32(os.ModeDir),
		},
		path: path,
		fs:   fs,
	}
}

func (fs *FS) newFile(name string) *file {
	n := time.Now()
	now := uint64(n.UnixMilli())
	return &file{
		attr: fuse.Attr{
			Ino:     0,
			Atime:   now,
			Mtime:   now,
			Ctime:   now,
			Crtime_: now,
			Mode:    uint32(os.ModeDir),
		},
		path: path,
		fs:   fs,
	}
}

func (f *FS) Root() (*Directory, error) {
	return f.root, nil
}

func (f *FS) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	s := syscall.Statfs_t{}
	err := syscall.Statfs(f.path, &s)
	if err != nil {
		return 1
	}

	out.Blocks = s.Blocks
	out.Bfree = s.Bfree
	out.Bavail = s.Bavail
	out.Ffree = s.Ffree
	out.Bsize = s.Bsize
	return 0
}

type Directory struct {
	fs.Inode
	lock   sync.RWMutex
	rc     io.Reader
	KeyDir map[string]string
	File   *file
	attr   fuse.Attr
	//extra
	path   string
	fs     *FS
	parent *Directory
}

// func (r *FS) OnAdd(ctx context.Context) {
// 	p := r.EmbeddedInode()
// 	rf := Directory{rc: r.rc, KeyDir: map[string]string{}, path: "app"}
// 	p.AddChild("app", r.NewPersistentInode(ctx, &rf, fs.StableAttr{Mode: syscall.S_IFDIR}), false)
// }

// Open
// Read
// Release
// Readir
// Readdirplus
// Stat
var root = "app"

// attrFromHeader fills a fuse.Attr struct from a tar.Header.
func attrFromHeader(h *tar.Header) fuse.Attr {
	out := fuse.Attr{
		Mode: uint32(h.Mode),
		Size: uint64(h.Size),
		Owner: fuse.Owner{
			Uid: uint32(h.Uid),
			Gid: uint32(h.Gid),
		},
	}
	out.SetTimes(&h.AccessTime, &h.ModTime, &h.ChangeTime)

	return out
}

var _ = (fs.NodeLookuper)((*Directory)(nil))

//the worker executes the containers

func (d *Directory) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	d.lock.RLock()
	defer d.lock.RUnlock()

	path := filepath.Join(d.path, name)
	stats, err := os.Stat(path)
	if err != nil {
		return nil, syscall.ENOENT
	}

	switch {
	case stats.IsDir():
		return &d.fs.newDir(path, stats.Mode()).Inode, 0

	case stats.Mode().IsRegular():
		return d.fs.newFile(path, stats.Mode()).Inode, 0

	default:
		panic("unknown type in fs")
	}

	// fmt.Println("called lookup on directory for", d.attr.String(), name)
	// for k, _ := range d.KeyDir {
	// 	fmt.Println("key", k)
	// }
	// hash, ok := d.KeyDir[name]
	// if ok {
	// 	fmt.Println("found hash", hash)
	// 	_, err := os.Stat("./" + root + "/" + hash)
	// 	fileExists := !errors.Is(err, os.ErrNotExist)

	// 	if fileExists {
	// 		//read file at hash
	// 		fmt.Println("reading file ./app/", hash)
	// 		reader, err := os.Open("./" + root + "/" + hash)
	// 		if err != nil {
	// 			return nil, syscall.ENOENT
	// 		}

	// 		fmt.Println("reader")
	// 		file := &file{
	// 			rc: reader,
	// 		}

	// 		fmt.Println("new node")
	// 		df := d.NewInode(
	// 			ctx, file,
	// 			fs.StableAttr{Ino: 0})

	// 		return df, 0

	// 	}
	// }
	// client := &http.Client{
	// 	Transport: &http.Transport{
	// 		TLSClientConfig: &tls.Config{
	// 			InsecureSkipVerify: true,
	// 		},
	// 	},
	// }
	// //file does not exist on the SSD
	// fmt.Println("need to call NFS", name)
	// //need to call NFS which has it
	// //we need a NFS with access to the tar file
	// //when we get the file from NFS we store it here in this FS
	// //this function should never read from the tar file directly
	// //the NFS should have all the entire docker image
	// //and then return the content here in bytes and we save it
	// requestUrl := fmt.Sprintf("https://localhost:8443/fetch?filepath=%s", d.name+"/"+name)
	// buffer := bytes.NewBuffer([]byte{})
	// req, err := http.NewRequest("GET", requestUrl, buffer)
	// if err != nil {
	// 	log.Fatalf("Error creating HTTP request: %v", err)
	// }

	// // Send the HTTP request
	// resp, err := client.Do(req)
	// if err != nil {
	// 	log.Fatalf("Error sending HTTP request: %v", err)
	// }

	// filecontent, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	fmt.Printf("error reading body: %s\n", err)
	// 	os.Exit(1)
	// }

	// // Assume `received` is the string received from the client
	// parts := strings.SplitN(string(filecontent), "|||", 2)
	// if len(parts) < 2 {
	// 	return nil, 1
	// }
	// hash = parts[0]
	// filecontentstring := parts[1]

	// fmt.Println("received", string(filecontent))

	// file := &file{
	// 	Data: []byte(filecontentstring),
	// 	rc:   d.rc,
	// }
	// file.Attr.Mode = 0777
	// file.Attr.Size = uint64(len(filecontent))
	// // Close the response body
	// defer resp.Body.Close()

	// df := d.NewPersistentInode(
	// 	ctx, file,
	// 	fs.StableAttr{Ino: 0})

	// d.AddChild(hash, df, false)
	// d.KeyDir[d.name+"/"+name] = hash
	// return df, 0
}

func (d *Directory) Getattr() fuse.Attr {
	d.lock.RLock()
	defer d.lock.RUnlock()
	if d.File == nil {
		// root directory
		return fuse.Attr{Mode: 0755}
	}
	return d.attr
}

func (f *file) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fmt.Println("CALLING READ", f.Data)
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
	fmt.Println("OPENING FILE", f.Data)

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Data == nil {
		fmt.Println("Data is nil, attempting to read")
		content, err := io.ReadAll(f.rc)
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
	tarread.Export("archive.tar", "https://localhost:8443")
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
	server, err := fs.Mount(flag.Arg(0), root, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}
	server.Wait()
}
