package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_isScript(t *testing.T) {
	assert.True(t, isScript("swayne123457_app.py"))
}
