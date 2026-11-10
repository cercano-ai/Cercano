//go:build darwin && cgo

package brewrestart

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreparedStoreDescriptors(t *testing.T) {
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		t.Run("store"+suffix, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			store := filepath.Join(root, "fixture store.db")
			path := store + suffix
			if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			// The descriptor's path differs; vnode identity must still match.
			alias := filepath.Join(root, "alias")
			if err := os.Link(path, alias); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(alias)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			found, code := preparedStoreDescriptors(os.Getpid(), store)
			if code != 0 || !found {
				t.Fatalf("open fixture store: found=%v code=%v", found, code)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			found, code = preparedStoreDescriptors(os.Getpid(), store)
			if code != 0 || found {
				t.Fatalf("closed fixture store: found=%v code=%v", found, code)
			}
		})
	}
}
