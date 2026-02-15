package main

import (
	"context"
	"syscall"
	"testing"

	"github.com/hanwen/go-fuse/v2/fuse"
)

func TestFileRead(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		offset   int64
		destLen  int
		expected []byte
	}{
		{
			name:     "read from start",
			data:     []byte("hello world"),
			offset:   0,
			destLen:  5,
			expected: []byte("hello"),
		},
		{
			name:     "read from middle",
			data:     []byte("hello world"),
			offset:   6,
			destLen:  5,
			expected: []byte("world"),
		},
		{
			name:     "read beyond end",
			data:     []byte("hello"),
			offset:   3,
			destLen:  10,
			expected: []byte("lo"),
		},
		{
			name:     "read empty data",
			data:     []byte{},
			offset:   0,
			destLen:  5,
			expected: []byte{},
		},
		{
			name:     "read beyond data",
			data:     []byte("hello"),
			offset:   10,
			destLen:  1,
			expected: []byte{},
		},
		{
			name:     "negative offset",
			data:     []byte("hello"),
			offset:   -1,
			destLen:  1,
			expected: []byte{},
		},
	}

	t.Run("nil data returns EIO", func(t *testing.T) {
		f := &file{
			path: "/nonexistent",
			attr: fuse.Attr{Size: 100},
		}
		dest := make([]byte, 100)
		_, errno := f.Read(context.Background(), nil, dest, 0)
		if errno != syscall.EIO {
			t.Errorf("expected EIO for nil Data, got %v", errno)
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &file{Data: tt.data}
			dest := make([]byte, tt.destLen)
			result, errno := f.Read(context.Background(), nil, dest, tt.offset)

			if errno != 0 {
				t.Errorf("expected errno 0, got %d", errno)
			}

			resultData, _ := result.Bytes(dest)
			if string(resultData) != string(tt.expected) {
				t.Errorf("expected %q, got %q", tt.expected, resultData)
			}
		})
	}
}
