package candidate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// No process is launched: these fixtures exercise the additional file identity
// receipt retained by the existing owned-launch binding.
func TestSelectedImageReceipt(t *testing.T) {
	for _, change := range []string{"unchanged", "replacement", "size", "timestamp"} {
		t.Run(change, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "inert")
			if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			bound, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "replacement":
				// Keep the old inode allocated so an immediate inode reuse is impossible.
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			case "size":
				if err := os.WriteFile(path, []byte("changed-size"), 0600); err != nil {
					t.Fatal(err)
				}
			case "timestamp":
				stamp := bound.ModTime().Add(time.Hour)
				if err := os.Chtimes(path, stamp, stamp); err != nil {
					t.Fatal(err)
				}
			}
			current, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if sameSelectedImage(bound, current) != (change == "unchanged") {
				t.Fatal("incorrect image receipt result")
			}
		})
	}
	if _, err := (*Candidate)(nil).SelectedIdentity(); err == nil {
		t.Fatal("nil candidate accepted")
	}
}
