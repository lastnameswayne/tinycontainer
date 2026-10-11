package main

import (
	"os"
	"path"
	"strings"
)

// pycEnabled lets Python use the image's precompiled .pyc files: __pycache__ lookups are
// allowed. SWAY_PYC=0 restores the old behaviour, where every
// import is compiled from source.
var pycEnabled = os.Getenv("SWAY_PYC") != "0"

// fileMtime is the modification time to report for a file.
func fileMtime(modTime int64) uint64 {
	if !pycEnabled || modTime < 0 {
		return 0
	}
	return uint64(modTime)
}

func isScript(filename string) bool {
	// use heurestics to decide
	if path.Ext(filename) != ".py" {
		return false
	}

	parts := strings.Split(filename, "_")
	if len(parts) != 2 {
		return false
	}

	app := parts[1]
	if app != "app.py" {
		return false
	}
	return true
}
