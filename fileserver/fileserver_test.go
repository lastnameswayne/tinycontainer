package main

import (
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

	t.Run("returns hash and content for existing file", func(t *testing.T) {
		testDir := t.TempDir()
		s := NewServerWithDir(testDir)

		testHash := "abc123def456"
		testContent := []byte("hello world")
		err := os.WriteFile(testDir+"/"+testHash, testContent, 0644)
		require.NoError(t, err)

		s.keydir["/usr/bin/python"] = testHash

		req := httptest.NewRequest(http.MethodGet, "/fetch?filepath=/usr/bin/python", nil)
		rec := httptest.NewRecorder()

		s.handleGet(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), testHash+"|||")
		assert.Contains(t, rec.Body.String(), "hello world")
	})

	t.Run("handles filepath with special characters", func(t *testing.T) {
		testDir := t.TempDir()
		s := NewServerWithDir(testDir)

		testHash := "special123"
		testContent := []byte("special content")
		err := os.WriteFile(testDir+"/"+testHash, testContent, 0644)
		require.NoError(t, err)

		s.keydir["/usr/lib/python3.10/encodings/__init__.py"] = testHash

		req := httptest.NewRequest(http.MethodGet, "/fetch?filepath=/usr/lib/python3.10/encodings/__init__.py", nil)
		rec := httptest.NewRecorder()

		s.handleGet(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "special content")
	})
}
