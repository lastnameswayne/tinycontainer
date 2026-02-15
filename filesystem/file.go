package main

import (
	"context"
	"io"
	"log"
	"os"
	"syscall"

	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// file represents a file in the filesystem
type file struct {
	fusefs.Inode
	Data []byte
	attr fuse.Attr
	path string
}

var _ = (fusefs.NodeReader)((*file)(nil))
var _ = (fusefs.NodeOpener)((*file)(nil))

func (f *file) Read(ctx context.Context, fh fusefs.FileHandle, dest []byte, offset int64) (fuse.ReadResult, syscall.Errno) {
	if f.Data == nil {
		log.Printf("READ called with nil Data, path=%s size=%d", f.path, f.attr.Size)
		return fuse.ReadResultData(nil), syscall.EIO
	}
	if offset < 0 || int(offset) >= len(f.Data) {
		return fuse.ReadResultData(nil), 0
	}
	end := int(offset) + len(dest)
	end = min(end, len(f.Data))
	return fuse.ReadResultData(f.Data[offset:end]), 0
}

func (f *file) Getattr(ctx context.Context, fh fusefs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0777
	out.Nlink = 1 // Hardcoding to 1, assume no hard-links
	out.Size = f.attr.Size
	const bs = 512
	out.Blksize = bs
	out.Blocks = (out.Size + bs - 1) / bs
	return 0
}

func (f *file) Open(ctx context.Context, flags uint32) (fusefs.FileHandle, uint32, syscall.Errno) {
	if f.Data != nil {
		return f, uint32(0), 0
	}

	reader, err := os.Open(f.path)
	if err != nil {
		return nil, 0, syscall.EIO
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, 0, syscall.EIO
	}
	f.Data = content

	return f, uint32(0), 0
}
