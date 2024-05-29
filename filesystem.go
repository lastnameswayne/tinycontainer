// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This program is the analogon of libfuse's hello.c, a a program that
// exposes a single file "file.txt" in the root directory.
package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type FS struct {
	fs.Inode
	rc io.ReadCloser
}

type Directory struct {
	fs.Inode
	rc     io.ReadCloser
	KeyDir map[string]string
	File   *file
}

// Open
// Read
// Release
// Readir
// Readdirplus
// Stat
// HeaderToFileInfo fills a fuse.Attr struct from a tar.Header.
func HeaderToFileInfo(h *tar.Header) fuse.Attr {
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

func (r *FS) OnAdd(ctx context.Context) {
	p := r.EmbeddedInode()
	rf := Directory{rc: r.rc, KeyDir: map[string]string{}}
	p.AddChild("data", r.NewPersistentInode(ctx, &rf, fs.StableAttr{Mode: syscall.S_IFDIR}), false)

}

var _ = (fs.NodeLookuper)((*Directory)(nil))

func (r *Directory) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0755
	return 0
}

func (d *Directory) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Println("called lookup on directory for", name)
	for k, _ := range d.KeyDir {
		fmt.Println("key", k)
	}
	hash, ok := d.KeyDir[name]
	if ok {

		fmt.Println("found on cache", hash)
	}
	reader := tar.NewReader(d.rc)

	for {
		header, err := reader.Next()
		if err == io.EOF {
			fmt.Println("END OF FILE")
			break
		}
		if err != nil {
			log.Printf("err %v", err)
			break
		}

		buf := bytes.NewBuffer(make([]byte, 0, header.Size))
		io.Copy(buf, reader)
		filepathStr := filepath.Clean(header.Name)
		_, base := filepath.Split(filepathStr)

		fmt.Println("filepath", filepathStr)
		attr := HeaderToFileInfo(header)

		if base == name {
			if header.Typeflag == tar.TypeDir {
				// Handle directory
				dir := &Directory{
					rc:     d.rc,
					KeyDir: d.KeyDir,
				}
				return &dir.Inode, 0
			} else {
				// Handle files
				h := sha1.New()
				content := buf.Bytes()
				h.Write(content)
				hash := h.Sum(nil)
				encoded := hex.EncodeToString(hash)
				d.KeyDir[base] = encoded

				file := &file{
					Attr: attr,
					Data: []byte("content"),
					rc:   d.rc,
				}

				df := d.NewPersistentInode(
					ctx, file,
					fs.StableAttr{Ino: 0})

				d.AddChild(encoded, df, false)

				return &file.Inode, 0
			}
		}
	}

	return nil, syscall.ENOENT
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
	rc   io.ReadCloser
	Data []byte
	Attr fuse.Attr
	mu   sync.Mutex
}

func (f *file) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	fmt.Println("OPENING FILE", f.Data)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Data == nil {
		fmt.Println("Data is nil, attempting to read")
		// Uncomment and handle reading from the ReadCloser
		content, err := io.ReadAll(f.rc)
		if err != nil {
			return nil, 0, syscall.EIO
		}
		f.rc.Close() // Ensure to close after reading
		f.Data = content
	}

	return f, fuse.FOPEN_KEEP_CACHE, 0 // Return 'f' as the file handle
}

var _ = (fs.NodeOnAdder)((*FS)(nil))
var _ = (fs.NodeLookuper)((*Directory)(nil))

var _ = (fs.NodeReader)((*file)(nil))
var _ = (fs.NodeOpener)((*file)(nil))

func main() {

	flag.Parse()
	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  hello MOUNTPOINT")
	}
	opts := &fs.Options{}
	cmd := exec.Command("umount", flag.Arg(0))
	err := cmd.Run()
	if err != nil {
		log.Default().Printf("Command execution failed: %v", err)
	}
	//init root
	f, err := os.Open("archive.tar")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	opts.Debug = true
	root := &FS{rc: f}
	server, err := fs.Mount(flag.Arg(0), root, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}
	fmt.Println("test")
	server.Wait()
}
