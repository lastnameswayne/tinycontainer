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
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
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
	rc io.Reader
}

type Directory struct {
	fs.Inode
	rc     io.Reader
	KeyDir map[string]string
	File   *file
	attr   fuse.Attr
}

// Open
// Read
// Release
// Readir
// Readdirplus
// Stat

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

func (r *FS) OnAdd(ctx context.Context) {
	p := r.EmbeddedInode()
	rf := Directory{rc: r.rc, KeyDir: map[string]string{}}
	p.AddChild("data", r.NewPersistentInode(ctx, &rf, fs.StableAttr{Mode: syscall.S_IFDIR}), false)

	ch := r.NewPersistentInode(
		ctx, &fs.MemRegularFile{
			Data: []byte("file.txt"),
			Attr: fuse.Attr{
				Mode: 0644,
			},
		}, fs.StableAttr{Ino: 2})
	rf.AddChild("file.txt", ch, false)

}

var _ = (fs.NodeLookuper)((*Directory)(nil))

//the worker executes the containers

func (d *Directory) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	fmt.Println("called lookup on directory for", d.attr.String(), name)
	for k, _ := range d.KeyDir {
		fmt.Println("key", k)
	}
	hash, ok := d.KeyDir[name]
	if ok {
		if _, err := os.Stat("tarmnt/data/" + hash); errors.Is(err, os.ErrNotExist) {
			//file does not exist on the SSD
			fmt.Println("need to call NFS", hash)
			//need to call NFS which has it
			//we need a NFS with access to the tar file
			//when we get the file from NFS we store it here in this FS
			//this function should never read from the tar file directly
			//the NFS should have all the entire docker image
			//and then return the content here in bytes and we save it
			requestUrl := fmt.Sprintf("http://localhost:8443/get?filepath=%s", name)
			res, err := http.Get(requestUrl)
			if err != nil {
				fmt.Printf("error making http request: %s\n", err)
				os.Exit(1)
			}
			filecontent, err := io.ReadAll(res.Body)
			if err != nil {
				fmt.Printf("error reading body: %s\n", err)
				os.Exit(1)
			}

			file := &file{
				Data: filecontent,
				rc:   d.rc,
			}

			df := d.NewPersistentInode(
				ctx, file,
				fs.StableAttr{Ino: 0})
			return df, syscall.ENOENT
		}

		//read file at hash
		fmt.Println("reading file tarmnt/data/", hash)
		reader, err := os.Open("tarmnt/data/" + hash)
		if err != nil {
			return nil, syscall.ENOENT
		}

		fmt.Println("reader")
		file := &file{
			rc: reader,
		}

		fmt.Println("new node")
		df := d.NewPersistentInode(
			ctx, file,
			fs.StableAttr{Ino: 0})

		return df, 0
	}

	fmt.Println("reading")
	reader := tar.NewReader(d.rc)
	for {
		fmt.Println("here")
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
		attr := attrFromHeader(header)

		if base == name {
			if header.Typeflag == tar.TypeDir {
				// Handle directory
				dir := &Directory{
					rc:     d.rc,
					KeyDir: d.KeyDir,
					attr:   attr,
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
					Data: []byte(content),
					rc:   d.rc,
				}

				df := d.NewPersistentInode(
					ctx, file,
					fs.StableAttr{Ino: 0})

				success := d.AddChild(encoded, df, true)
				fmt.Println("added succesfully", encoded, success)

				return df, 0
			}
		}
	}

	return nil, syscall.ENOENT
}

func (f *Directory) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	fmt.Println("CALLING READ on directory")

	return nil, 0
}

func (d *Directory) Getattr() fuse.Attr {
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
	Attr fuse.Attr
	mu   sync.Mutex
}

func (f *file) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0777
	out.Nlink = 1
	out.Size = f.Attr.Size
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

var _ = (fs.NodeOnAdder)((*FS)(nil))
var _ = (fs.NodeLookuper)((*Directory)(nil))
var _ = (fs.NodeReader)((*Directory)(nil))

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
	server.Wait()
}
