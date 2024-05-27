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
	"strings"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type HelloRoot struct {
	fs.Inode
	rc     io.ReadCloser
	KeyDir map[string]string
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
func (r *HelloRoot) OnAdd(ctx context.Context) {
	//take a tarball, save filepath, hash content and save to a map
	//use the hash as the file location. hash should be like 10 chars long
	fmt.Println(r.rc)
	reader := tar.NewReader(r.rc)
	defer r.rc.Close()

	var longName *string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("err %v", err)
			break
		}

		if header.Typeflag == 'L' {
			buf := bytes.NewBuffer(make([]byte, 0, header.Size))
			io.Copy(buf, reader)
			s := buf.String()
			longName = &s
			continue
		}

		if longName != nil {
			header.Name = *longName
			longName = nil
		}

		buf := bytes.NewBuffer(make([]byte, 0, header.Size))
		io.Copy(buf, reader)
		filepathStr := filepath.Clean(header.Name)
		dir, _ := filepath.Split(filepathStr)

		p := r.EmbeddedInode()
		for _, comp := range strings.Split(dir, "/") {
			if len(comp) == 0 {
				continue
			}
			ch := p.GetChild(comp)
			if ch != nil {
				p = ch
				continue
			}

			ch = p.NewPersistentInode(ctx, &fs.Inode{}, fs.StableAttr{Mode: syscall.S_IFDIR})
			p.AddChild(comp, ch, false)
		}

		attr := HeaderToFileInfo(header)

		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			h := sha1.New()
			content := buf.Bytes()
			h.Write(content)
			hash := h.Sum(nil)
			encoded := hex.EncodeToString(hash)
			r.KeyDir[filepathStr] = encoded

			df := r.NewPersistentInode(
				ctx, &file{
					Attr:   attr,
					Data:   []byte("hello world"),
					FileID: filepathStr,
					Themap: &r.KeyDir,
				},
				fs.StableAttr{Ino: 0})

			p.AddChild(encoded, df, false)
		}
	}
}

func (r *HelloRoot) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0755
	return 0
}

func (f *HelloRoot) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	fmt.Println("CALLED OPEN ON ROOT")

	// We don't return a filehandle since we don't really need
	// one.  The file content is immutable, so hint the kernel to
	// cache the data.
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

func (f *HelloRoot) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Println("root reading file", name)
	if ch := f.GetChild(name); ch != nil {
		//RETURNS A CHILD (not a my own file) WHICH IS WHY MY LOOKUP IS NOT BEING CALLED
		return ch, 0
	}

	return nil, syscall.ENOENT
}
func (f *file) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Println("file reading file", name)

	return nil, syscall.ENOENT
}

func (f *file) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := int(off) + len(dest)
	if end > len(f.Data) {
		end = len(f.Data)
	}
	return fuse.ReadResultData(f.Data[off:end]), 0
}

// file is a file
type file struct {
	fs.Inode
	Data   []byte
	Attr   fuse.Attr
	FileID string
	mu     sync.Mutex
	Themap *map[string]string
}

func (f *file) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	fmt.Println("CALLED OPEN")
	hash, ok := (*f.Themap)[f.FileID]
	if !ok {
		fmt.Println("its not in the map")
		//call the NFS
	}
	fmt.Println("FOUND HASH")

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Data == nil {
		rc, err := os.Open(hash)
		if err != nil {
			return nil, 0, syscall.EIO
		}
		content, err := io.ReadAll(rc)
		if err != nil {
			return nil, 0, syscall.EIO
		}

		f.Data = content
	}

	// We don't return a filehandle since we don't really need
	// one.  The file content is immutable, so hint the kernel to
	// cache the data.
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

var _ = (fs.NodeGetattrer)((*HelloRoot)(nil))
var _ = (fs.NodeOnAdder)((*HelloRoot)(nil))
var _ = (fs.NodeOpener)((*HelloRoot)(nil))

var _ = (fs.NodeReader)((*file)(nil))
var _ = (fs.NodeOpener)((*file)(nil))
var _ = (fs.NodeLookuper)((*HelloRoot)(nil))
var _ = (fs.NodeLookuper)((*file)(nil))

func main() {

	flag.Parse()
	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  hello MOUNTPOINT")
	}
	opts := &fs.Options{}
	opts.Debug = true
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
	root := &HelloRoot{rc: f, KeyDir: map[string]string{}}
	server, err := fs.Mount(flag.Arg(0), root, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}
	fmt.Println("test")
	server.Wait()
}
