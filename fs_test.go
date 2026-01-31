package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_newDir(t *testing.T) {
	t.Run("initializes children map", func(t *testing.T) {
		testFS := &FS{
			KeyDir: map[string]string{},
		}

		dir := testFS.newDir("/test")

		assert.NotNil(t, dir.children, "children map should be initialized")
	})
}
