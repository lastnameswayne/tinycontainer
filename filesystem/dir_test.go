package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fs"
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

		oldURL := _fileserverURL
		_fileserverURL = server.URL
		defer func() { _fileserverURL = oldURL }()

		testFS := &FS{
			KeyDir: map[string]string{},
			client: server.Client(),
		}

		parentDir := &Directory{
			path:     "/test",
			fs:       testFS,
			children: map[string]*Directory{},
			keyDir:   map[string]string{},
		}

		childDir := &Directory{
			path:     "/test/subdir",
			fs:       testFS,
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
		testFS := &FS{
			KeyDir: map[string]string{},
		}

		emptyDir := &Directory{
			path:     "/empty",
			fs:       testFS,
			children: map[string]*Directory{},
		}

		ctx := context.Background()
		stream, errno := emptyDir.Readdir(ctx)
		require.Equal(t, syscall.Errno(0), errno)

		entries := collectEntries(t, stream)
		assert.Empty(t, entries)
	})

	t.Run("multiple files are listed", func(t *testing.T) {
		testFS := &FS{
			KeyDir: map[string]string{},
		}

		dir := &Directory{
			path:     "/encodings",
			fs:       testFS,
			children: map[string]*Directory{},
		}

		// Add multiple files like Python's encodings package
		fileNames := []string{"__init__.py", "utf_8.py", "latin_1.py", "aliases.py"}

		ctx := context.Background()
		stream, errno := dir.Readdir(ctx)
		require.Equal(t, syscall.Errno(0), errno)

		entries := collectEntries(t, stream)
		assert.Len(t, entries, len(fileNames))

		for _, entry := range entries {
			assert.Contains(t, fileNames, entry.Name)
			assert.Equal(t, uint32(fuse.S_IFREG), entry.Mode)
		}
	})
}

// collectEntries drains a DirStream into a slice
func collectEntries(t *testing.T, stream fs.DirStream) []fuse.DirEntry {
	t.Helper()
	entries := []fuse.DirEntry{}
	for stream.HasNext() {
		entry, errno := stream.Next()
		require.Equal(t, syscall.Errno(0), errno)
		entries = append(entries, entry)
	}
	return entries
}
