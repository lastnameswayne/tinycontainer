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
	"flag"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type HelloRoot struct {
	fs.Inode
	rc io.ReadCloser
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
		dir, base := filepath.Split(filepath.Clean(header.Name))

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
			df := &fs.MemRegularFile{}
			df.Attr = attr
			p.AddChild(base, r.NewPersistentInode(ctx, df, fs.StableAttr{}), false)
		}
	}

	ch := r.NewPersistentInode(
		ctx, &file{Data: []byte("hello world")}, fs.StableAttr{Ino: 2})
	r.AddChild("file.txt", ch, false)
}

func (r *HelloRoot) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0755
	return 0
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
	Data []byte
}

func (f *file) Open(ctx context.Context, flags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	fmt.Println("CALLING OPEN FILE")
	if f.Data == nil {
	}
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

var _ = (fs.NodeGetattrer)((*HelloRoot)(nil))
var _ = (fs.NodeOnAdder)((*HelloRoot)(nil))

var _ = (fs.NodeReader)((*file)(nil))
var _ = (fs.NodeOpener)((*file)(nil))

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
	server, err := fs.Mount(flag.Arg(0), &HelloRoot{}, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}
	fmt.Println("test")
	server.Wait()
}
