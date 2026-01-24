package main

import (
	"context"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DirectoryReadDir(t *testing.T) {
	t.Run("lists files and directories", func(t *testing.T) {
		// Create a minimal FS
		testFS := &FS{
			KeyDir: map[string]string{},
		}

		// Create parent directory
		parentDir := &Directory{
			path:     "/test",
			fs:       testFS,
			children: map[string]*Directory{},
		}

		// Add a child directory
		childDir := &Directory{
			path:     "/test/subdir",
			fs:       testFS,
			children: map[string]*Directory{},
		}
		parentDir.children["subdir"] = childDir

		// Call Readdir
		ctx := context.Background()
		stream, errno := parentDir.Readdir(ctx)
		require.Equal(t, syscall.Errno(0), errno)

		// Collect all entries
		entries := collectEntries(t, stream)

		// Should have 2 entries: 1 dir + 1 file
		assert.Len(t, entries, 2)

		// Check we have both a directory and a file
		entryMap := make(map[string]fuse.DirEntry)
		for _, e := range entries {
			entryMap[e.Name] = e
		}

		assert.Contains(t, entryMap, "subdir")
		assert.Equal(t, uint32(fuse.S_IFDIR), entryMap["subdir"].Mode)

		assert.Contains(t, entryMap, "__init__.py")
		assert.Equal(t, uint32(fuse.S_IFREG), entryMap["__init__.py"].Mode)
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

		// Verify all files are present with correct mode
		for _, entry := range entries {
			assert.Contains(t, fileNames, entry.Name)
			assert.Equal(t, uint32(fuse.S_IFREG), entry.Mode)
		}
	})
}

func Test_newDir(t *testing.T) {
	t.Run("initializes children map", func(t *testing.T) {
		testFS := &FS{
			KeyDir: map[string]string{},
		}

		dir := testFS.newDir("/test")

		assert.NotNil(t, dir.children, "children map should be initialized")
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

func Test_isScript(t *testing.T) {
	assert.True(t, isScript("swayne123457_app.py"))
}
