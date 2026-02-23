package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DirectoryReadDir(t *testing.T) {
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
