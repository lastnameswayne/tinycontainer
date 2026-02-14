package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"syscall"
	"time"

	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// FS is the root filesystem
type FS struct {
	fusefs.Inode

	root   *Directory
	path   string
	client *http.Client
}

func (r *FS) OnAdd(ctx context.Context) {
	p := r.EmbeddedInode()
	rf := r.newDir("app")
	p.AddChild("app", r.NewPersistentInode(ctx, rf, fusefs.StableAttr{Mode: syscall.S_IFDIR}), false)

	r.initLinuxDirs(ctx, rf, []string{
		"home", "lib", "media", "mnt", "opt",
		"proc", "dev", "sys", "lib64",
	})
}

func (r *FS) initLinuxDirs(ctx context.Context, parent *Directory, names []string) {
	for _, name := range names {
		dir := r.newDir(name)
		dir.parent = parent
		node := r.NewPersistentInode(ctx, dir, fusefs.StableAttr{Mode: syscall.S_IFDIR})
		parent.AddChild(name, node, false)
		parent.children[name] = dir
	}
}

var _ = (fusefs.NodeStatfser)((*FS)(nil))

const _cacheDir = "filecache"
const _timeout = 5 * time.Minute

func NewFS(path string) *FS {
	// Create local filecache directory
	if err := os.MkdirAll(_cacheDir, 0755); err != nil {
		fmt.Println("Error creating cache directory:", err)
	}

	fs := &FS{
		path: path,
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			ResponseHeaderTimeout: _timeout,
		},
		Timeout: _timeout,
	}
	fs.client = client
	fs.root = fs.newDir(path)
	return fs
}

func (fs *FS) newDir(path string) *Directory {
	n := time.Now()
	now := uint64(n.UnixMilli())
	children := map[string]*Directory{}
	return &Directory{
		attr: fuse.Attr{
			Atime: now,
			Mtime: now,
			Ctime: now,
			Mode:  uint32(os.ModeDir),
		},
		children: children,
		path:     path,
		fs:       fs,
		keyDir:   make(map[string]string),
	}
}

func (f *FS) Root() (*Directory, error) {
	return f.root, nil
}

func (f *FS) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	// Remote filesystems are effectively unbounded, so just returning a large, fixed value
	*out = fuse.StatfsOut{
		Bsize:  4096,
		Blocks: 1 << 30,
		Bavail: 1 << 30,
		Bfree:  1 << 30,
	}
	return 0
}
