//go:build !windows

package conversation

import (
	"os"
	"testing"
)

func makeNonprivateDevelopmentTestDirectory(t *testing.T, path string) func() {
	t.Helper()
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	// Override umask so the fixture really is nonprivate.
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		after, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before, after) || before.Mode() != after.Mode() {
			t.Error("refused parent identity or permissions mutated")
		}
	}
}
