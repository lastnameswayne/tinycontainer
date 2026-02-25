package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"

	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// FS is the root filesystem
type FS struct {
	fusefs.Inode

	root        *Directory
	path        string
	client      *http.Client
	notFoundMu  sync.RWMutex
	notFoundSet map[string]struct{} // paths known not to exist; cleared at the start of each run. Using this to avoid re-fetches to the fileserver.
}

func (f *FS) ClearNotFound() {
	f.notFoundMu.Lock()
	f.notFoundSet = make(map[string]struct{})
	f.notFoundMu.Unlock()
}

func (f *FS) addNotFound(path string) {
	f.notFoundMu.Lock()
	f.notFoundSet[path] = struct{}{}
	f.notFoundMu.Unlock()
}

func (f *FS) isNotFound(path string) bool {
	f.notFoundMu.RLock()
	_, ok := f.notFoundSet[path]
	f.notFoundMu.RUnlock()
	return ok
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
		log.Printf("error creating cache directory: %v", err)
	}

	fs := &FS{
		path:        path,
		notFoundSet: make(map[string]struct{}),
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
	now := uint64(time.Now().Unix())
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
		rootFS:   fs,
		keyDir:   make(map[string]cachedMetadata),
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
