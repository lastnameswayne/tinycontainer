package main

import (
	"context"
	"io"
	"os"
	"sync"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// file represents a file in the filesystem
type file struct {
	fs.Inode
	rc   io.Reader
	Data []byte
	attr fuse.Attr
	mu   sync.Mutex
	path string
	fs   *FS
}

var _ = (fs.NodeReader)((*file)(nil))
var _ = (fs.NodeOpener)((*file)(nil))

func (f *file) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	end := int(off) + len(dest)
	end = min(end, len(f.Data))
	return fuse.ReadResultData(f.Data[off:end]), 0
}

// TODO: stop hardcoding
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
	if f.Data != nil {
		return f, uint32(0), 0
	}

	reader, err := os.Open(f.path)
	if err != nil {
		panic("cant open")
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, 0, syscall.EIO
	}
	f.Data = content

	return f, uint32(0), 0
}
