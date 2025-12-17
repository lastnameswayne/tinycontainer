package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleGet(t *testing.T) {
	t.Run("returns 400 when filepath param is missing", func(t *testing.T) {
		s := NewServerWithDir(t.TempDir())

		req := httptest.NewRequest(http.MethodGet, "/fetch", nil)
		rec := httptest.NewRecorder()

		s.handleGet(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "filepath is required")
	})

	t.Run("returns 400 when filepath param is empty string", func(t *testing.T) {
		s := NewServerWithDir(t.TempDir())

		req := httptest.NewRequest(http.MethodGet, "/fetch?filepath=", nil)
		rec := httptest.NewRecorder()

		s.handleGet(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "filepath is required")
	})

	t.Run("returns 404 when filepath not in keydir", func(t *testing.T) {
		s := NewServerWithDir(t.TempDir())

		req := httptest.NewRequest(http.MethodGet, "/fetch?filepath=/usr/bin/python", nil)
		rec := httptest.NewRecorder()

		s.handleGet(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "Not found")
	})

	t.Run("returns KeyValue struct for existing file", func(t *testing.T) {
		testDir := t.TempDir()
		s := NewServerWithDir(testDir)

		// Store a KeyValue as JSON (matching new format)
		testHash := "abc123def456"
		entry := KeyValue{
			Key:   "/usr/bin/python",
			Value: []byte("hello world"),
			Name:  "python",
		}
		marshalledEntry, _ := json.Marshal(entry)
		err := os.WriteFile(testDir+"/"+testHash, marshalledEntry, 0644)
		require.NoError(t, err)

		s.keydir["/usr/bin/python"] = testHash

		req := httptest.NewRequest(http.MethodGet, "/fetch?filepath=/usr/bin/python", nil)
		rec := httptest.NewRecorder()

		s.handleGet(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "/usr/bin/python")
		assert.Contains(t, rec.Body.String(), "python")
		assert.Contains(t, rec.Body.String(), "104 101 108 108 111")
	})
}

func TestHandleSet(t *testing.T) {
	t.Cleanup(func() {
		os.Remove("index.json")
	})

	t.Run("stores single file with metadata", func(t *testing.T) {
		testDir := t.TempDir()
		s := NewServerWithDir(testDir)

		entry := KeyValue{
			Key:    "/usr/bin/python",
			Value:  []byte("python binary"),
			Name:   "python",
			Parent: "/usr/bin",
			Size:   13,
			Mode:   0755,
		}
		body, _ := json.Marshal(entry)

		req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		s.handleSet(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "Set successful")

		assert.Contains(t, s.keydir, "/usr/bin/python")

		hash := s.keydir["/usr/bin/python"]
		content, err := os.ReadFile(testDir + "/" + hash)
		require.NoError(t, err)

		var stored KeyValue
		err = json.Unmarshal(content, &stored)
		require.NoError(t, err)

		assert.Equal(t, "/usr/bin/python", stored.Key)
		assert.Equal(t, []byte("python binary"), stored.Value)
		assert.Equal(t, "python", stored.Name)
		assert.Equal(t, "/usr/bin", stored.Parent)
		assert.Equal(t, int64(13), stored.Size)
		assert.Equal(t, int64(0755), stored.Mode)
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		s := NewServerWithDir(t.TempDir())

		req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		s.handleSet(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Invalid JSON")
	})

	t.Run("file can be fetched after upload", func(t *testing.T) {
		testDir := t.TempDir()
		s := NewServerWithDir(testDir)

		entry := KeyValue{
			Key:   "/test/hello.txt",
			Value: []byte("hello world"),
			Name:  "hello.txt",
		}
		body, _ := json.Marshal(entry)
		req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		s.handleSet(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		req = httptest.NewRequest(http.MethodGet, "/fetch?filepath=/test/hello.txt", nil)
		rec = httptest.NewRecorder()
		s.handleGet(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "/test/hello.txt")
		assert.Contains(t, rec.Body.String(), "hello.txt")
	})
}

func TestHandleSetBatch(t *testing.T) {
	t.Cleanup(func() {
		os.Remove("index.json")
	})

	t.Run("stores multiple files", func(t *testing.T) {
		testDir := t.TempDir()
		s := NewServerWithDir(testDir)

		entries := []KeyValue{
			{Key: "/usr/bin/python", Value: []byte("python binary"), Name: "python", Parent: "/usr/bin", Mode: 0755},
			{Key: "/usr/lib/os.py", Value: []byte("os module"), Name: "os.py", Parent: "/usr/lib", Mode: 0644},
			{Key: "/etc/config.txt", Value: []byte("config"), Name: "config.txt", Parent: "/etc", Mode: 0644},
		}
		body, _ := json.Marshal(entries)

		req := httptest.NewRequest(http.MethodPost, "/batch-upload", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		s.handleSetBatch(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "Stored 3 files")

		assert.Contains(t, s.keydir, "/usr/bin/python")
		assert.Contains(t, s.keydir, "/usr/lib/os.py")
		assert.Contains(t, s.keydir, "/etc/config.txt")

		for _, entry := range entries {
			hash := s.keydir[entry.Key]
			content, err := os.ReadFile(testDir + "/" + hash)
			require.NoError(t, err)

			var stored KeyValue
			err = json.Unmarshal(content, &stored)
			require.NoError(t, err)

			assert.Equal(t, entry.Key, stored.Key)
			assert.Equal(t, entry.Value, stored.Value)
			assert.Equal(t, entry.Name, stored.Name)
			assert.Equal(t, entry.Parent, stored.Parent)
			assert.Equal(t, entry.Mode, stored.Mode)
		}
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		s := NewServerWithDir(t.TempDir())

		req := httptest.NewRequest(http.MethodPost, "/batch-upload", bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		s.handleSetBatch(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Invalid JSON")
	})

	t.Run("handles empty array", func(t *testing.T) {
		s := NewServerWithDir(t.TempDir())

		body, _ := json.Marshal([]KeyValue{})
		req := httptest.NewRequest(http.MethodPost, "/batch-upload", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		s.handleSetBatch(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "Stored 0 files")
	})

	t.Run("files can be fetched after batch upload", func(t *testing.T) {
		testDir := t.TempDir()
		s := NewServerWithDir(testDir)

		entries := []KeyValue{
			{Key: "/test/hello.txt", Value: []byte("hello world"), Name: "hello.txt", Parent: "/test"},
		}
		body, _ := json.Marshal(entries)
		req := httptest.NewRequest(http.MethodPost, "/batch-upload", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		s.handleSetBatch(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		req = httptest.NewRequest(http.MethodGet, "/fetch?filepath=/test/hello.txt", nil)
		rec = httptest.NewRecorder()
		s.handleGet(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "/test/hello.txt")
		assert.Contains(t, rec.Body.String(), "hello.txt")
	})
}
