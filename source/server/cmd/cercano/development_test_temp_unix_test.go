//go:build !windows

package main

import (
	"os"
	"path/filepath"
)

func developmentProductionTempParent() string {
	// Worker resource roots must fit 38 bytes after resolving aliases, leaving
	// room for generated Unix socket names. Darwin's OS temp parent is too long.
	parent, err := filepath.EvalSymlinks(os.TempDir())
	if err == nil && len(parent)+1+len("cdp-")+10 <= 38 {
		return ""
	}
	return "/tmp"
}
