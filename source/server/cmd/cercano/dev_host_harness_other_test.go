//go:build !unix

package main

import (
	"os/exec"
	"testing"
)

func devHostDump(cmd *exec.Cmd)       {}
func devHostBoundBuild(cmd *exec.Cmd) {}

func devHostWorkersGone(t *testing.T, root string) {
	t.Error("worker teardown verification requires Unix")
}
