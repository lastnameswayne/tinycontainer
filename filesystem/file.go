package main

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// file represents a file in the filesystem
type file struct {
	fusefs.Inode
	Data     []byte
	attr     fuse.Attr
	path     string
	key      string // fileserver key, used for event logging
	loadOnce sync.Once
	loadErr  syscall.Errno
}

var _ = (fusefs.NodeReader)((*file)(nil))
var _ = (fusefs.NodeOpener)((*file)(nil))

func (f *file) Read(ctx context.Context, fh fusefs.FileHandle, dest []byte, offset int64) (fuse.ReadResult, syscall.Errno) {
	start := time.Now()
	if f.Data == nil {
		log.Printf("READ called with nil Data, path=%s size=%d", f.path, f.attr.Size)
		emitEvent("read", f.key, 0, tierError, start, true, 0)
		return fuse.ReadResultData(nil), syscall.EIO
	}
	if offset < 0 || int(offset) >= len(f.Data) {
		emitEvent("read", f.key, 0, tierMem, start, true, 0)
		return fuse.ReadResultData(nil), 0
	}
	end := int(offset) + len(dest)
	end = min(end, len(f.Data))
	emitEvent("read", f.key, int64(end-int(offset)), tierMem, start, true, 0)
	return fuse.ReadResultData(f.Data[offset:end]), 0
}

func (f *file) Getattr(ctx context.Context, fh fusefs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0777
	out.Nlink = 1 // Hardcoding to 1, assume no hard-links
	out.Size = f.attr.Size
	out.Mtime = f.attr.Mtime
	const bs = 512
	out.Blksize = bs
	out.Blocks = (out.Size + bs - 1) / bs
	return 0
}

func (f *file) Open(ctx context.Context, flags uint32) (fusefs.FileHandle, uint32, syscall.Errno) {
	start := time.Now()
	tier := tierMem
	f.loadOnce.Do(func() {
		if f.Data != nil {
			return // pre-populated at construction time, nothing to load
		}
		reader, err := os.Open(f.path)
		if err != nil {
			f.loadErr = syscall.EIO
			return
		}
		defer reader.Close()
		content, err := io.ReadAll(reader)
		if err != nil {
			f.loadErr = syscall.EIO
			return
		}
		f.Data = content
		tier = tierDisk
		diskCache.touched(filepath.Base(f.path))
	})

	if f.loadErr != 0 {
		emitEvent("open", f.key, 0, tierError, start, true, 0)
		return nil, 0, f.loadErr
	}
	emitEvent("open", f.key, int64(len(f.Data)), tier, start, true, 0)
	return f, uint32(0), 0
}
