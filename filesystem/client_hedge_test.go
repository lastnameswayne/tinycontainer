package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_HedgedFetch(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
				return
			}
		}
		json.NewEncoder(w).Encode(KeyValue{Key: "app/x", Value: []byte("hi"), Size: 2, HashValue: "h"})
	}))
	defer server.Close()

	fs := &FS{client: server.Client(), fileserverURL: server.URL, notFoundSet: map[string]struct{}{}}
	d := fs.newDir("app")

	t.Run("slow first request is beaten by the hedge", func(t *testing.T) {
		old := hedgeAfter
		hedgeAfter = 50 * time.Millisecond
		defer func() { hedgeAfter = old }()

		start := time.Now()
		entry, hedge, err := d.getEntryFromFileServer("x")
		require.NoError(t, err)
		assert.Equal(t, "hi", string(entry.Value))
		assert.Equal(t, 2, hedge)
		assert.Less(t, time.Since(start), time.Second)
		assert.Equal(t, int32(2), calls.Load())
	})

	t.Run("fast request never hedges", func(t *testing.T) {
		old := hedgeAfter
		hedgeAfter = 500 * time.Millisecond
		defer func() { hedgeAfter = old }()

		_, hedge, err := d.getEntryFromFileServer("x")
		require.NoError(t, err)
		assert.Equal(t, 0, hedge)
		assert.Equal(t, int32(3), calls.Load())
	})
}
