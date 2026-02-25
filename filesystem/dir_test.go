package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"

	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DirectoryReadDir(t *testing.T) {
	t.Run("lists children from fileserver", func(t *testing.T) {

	})
	t.Run("lists children when server returns 404", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		oldURL := fileserverURL
		fileserverURL = server.URL
		defer func() { fileserverURL = oldURL }()

		testFS := &FS{
			client: server.Client(),
		}

		parentDir := &Directory{
			path:     "/test",
			rootFS:   testFS,
			children: map[string]*Directory{},
			keyDir:   map[string]cachedMetadata{},
		}

		childDir := &Directory{
			path:     "/test/subdir",
			rootFS:   testFS,
			children: map[string]*Directory{},
		}
		parentDir.children["subdir"] = childDir

		ctx := context.Background()
		stream, errno := parentDir.Readdir(ctx)
		require.Equal(t, syscall.Errno(0), errno)

		entries := collectEntries(t, stream)

		assert.Len(t, entries, 1)
		assert.Equal(t, "subdir", entries[0].Name)
		assert.Equal(t, uint32(fuse.S_IFDIR), entries[0].Mode)
	})

	t.Run("empty directory returns empty stream", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		oldURL := fileserverURL
		fileserverURL = server.URL
		defer func() { fileserverURL = oldURL }()

		testFS := &FS{client: server.Client()}

		emptyDir := &Directory{
			path:     "/empty",
			rootFS:   testFS,
			children: map[string]*Directory{},
		}

		ctx := context.Background()
		stream, errno := emptyDir.Readdir(ctx)
		require.Equal(t, syscall.Errno(0), errno)

		entries := collectEntries(t, stream)
		assert.Empty(t, entries)
	})

	t.Run("multiple files are listed", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		oldURL := fileserverURL
		fileserverURL = server.URL
		defer func() { fileserverURL = oldURL }()

		testFS := &FS{client: server.Client()}

		dir := &Directory{
			path:     "/encodings",
			rootFS:   testFS,
			children: map[string]*Directory{},
		}

		// Server returns 404, so only local children are listed
		child1 := &Directory{path: "/encodings/utf_8", rootFS: testFS, children: map[string]*Directory{}}
		child2 := &Directory{path: "/encodings/latin_1", rootFS: testFS, children: map[string]*Directory{}}
		dir.children["utf_8"] = child1
		dir.children["latin_1"] = child2

		ctx := context.Background()
		stream, errno := dir.Readdir(ctx)
		require.Equal(t, syscall.Errno(0), errno)

		entries := collectEntries(t, stream)
		assert.Len(t, entries, 2)

		names := []string{}
		for _, entry := range entries {
			names = append(names, entry.Name)
			assert.Equal(t, uint32(fuse.S_IFDIR), entry.Mode)
		}
		assert.Contains(t, names, "utf_8")
		assert.Contains(t, names, "latin_1")
	})
}

func newTestDir(serverURL string) (*Directory, *http.Client) {
	client := &http.Client{}
	testFS := &FS{
		client:      client,
		notFoundSet: make(map[string]struct{}),
	}
	dir := &Directory{
		path:     "/app",
		rootFS:   testFS,
		children: map[string]*Directory{},
		keyDir:   map[string]cachedMetadata{},
	}
	fileserverURL = serverURL
	return dir, client
}

func Test_DirectoryLookup(t *testing.T) {
	t.Run("pyc files return ENOENT without hitting server", func(t *testing.T) {
		var requestCount atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
		}))
		defer server.Close()

		oldURL := fileserverURL
		dir, _ := newTestDir(server.URL)
		defer func() { fileserverURL = oldURL }()

		_, errno := dir.Lookup(context.Background(), "something.pyc.123", &fuse.EntryOut{})

		assert.Equal(t, syscall.ENOENT, errno)
		assert.Equal(t, int64(0), requestCount.Load())
	})

	t.Run("server 404 returns ENOENT", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		oldURL := fileserverURL
		dir, _ := newTestDir(server.URL)
		defer func() { fileserverURL = oldURL }()

		_, errno := dir.Lookup(context.Background(), "missing.so", &fuse.EntryOut{})

		assert.Equal(t, syscall.ENOENT, errno)
	})

	t.Run("not-found is cached: second lookup does not hit server", func(t *testing.T) {
		var requestCount atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		oldURL := fileserverURL
		dir, _ := newTestDir(server.URL)
		defer func() { fileserverURL = oldURL }()

		ctx := context.Background()
		dir.Lookup(ctx, "missing.so", &fuse.EntryOut{})
		dir.Lookup(ctx, "missing.so", &fuse.EntryOut{})

		assert.Equal(t, int64(1), requestCount.Load())
	})

	t.Run("server error returns EIO", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		oldURL := fileserverURL
		dir, _ := newTestDir(server.URL)
		defer func() { fileserverURL = oldURL }()

		_, errno := dir.Lookup(context.Background(), "broken.so", &fuse.EntryOut{})

		assert.Equal(t, syscall.EIO, errno)
	})

	t.Run("memory cache hit returns inode without hitting server", func(t *testing.T) {
		var requestCount atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
		}))
		defer server.Close()

		oldURL := fileserverURL
		dir, _ := newTestDir(server.URL)
		defer func() { fileserverURL = oldURL }()

		childDir := &Directory{path: "/app/numpy", rootFS: dir.rootFS, children: map[string]*Directory{}}
		dir.children["numpy"] = childDir

		before := LookupStats.MemoryCacheHits.Load()
		inode, errno := dir.Lookup(context.Background(), "numpy", &fuse.EntryOut{})

		assert.Equal(t, syscall.Errno(0), errno)
		assert.NotNil(t, inode)
		assert.Equal(t, int64(1), LookupStats.MemoryCacheHits.Load()-before)
		assert.Equal(t, int64(0), requestCount.Load())
	})

	t.Run("concurrent lookups: after first round notFoundSet prevents further server hits", func(t *testing.T) {
		var requestCount atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		oldURL := fileserverURL
		dir, _ := newTestDir(server.URL)
		defer func() { fileserverURL = oldURL }()

		// First round: concurrent lookups all miss, some may race before notFoundSet is populated
		var wg sync.WaitGroup
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				dir.Lookup(context.Background(), "missing.so", &fuse.EntryOut{})
			}()
		}
		wg.Wait()

		// After the first round, notFoundSet is populated.
		// Every subsequent lookup should return ENOENT with zero server hits.
		countAfterFirstRound := requestCount.Load()
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errno := dir.Lookup(context.Background(), "missing.so", &fuse.EntryOut{})
				assert.Equal(t, syscall.ENOENT, errno)
			}()
		}
		wg.Wait()

		assert.Equal(t, countAfterFirstRound, requestCount.Load(), "second round must not hit server")
	})
}

// collectEntries drains a DirStream into a slice
func collectEntries(t *testing.T, stream fusefs.DirStream) []fuse.DirEntry {
	t.Helper()
	entries := []fuse.DirEntry{}
	for stream.HasNext() {
		entry, errno := stream.Next()
		require.Equal(t, syscall.Errno(0), errno)
		entries = append(entries, entry)
	}
	return entries
}
