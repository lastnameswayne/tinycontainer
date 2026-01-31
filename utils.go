package main

import (
	"path"
	"strings"
)

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
