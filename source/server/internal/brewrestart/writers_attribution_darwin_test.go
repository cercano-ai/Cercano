//go:build darwin && cgo

package brewrestart

import (
	"context"
	"syscall"
	"testing"
)

func TestPreparedWriterAttributionBranches(t *testing.T) {
	for _, tt := range []struct {
		name, path           string
		argvErr, identityErr error
		wantErr, fallback    bool
	}{
		{"denied-unrelated", "/unrelated/cercano", syscall.EPERM, nil, false, true},
		{"denied-matching", "/generation/cercano", syscall.EPERM, nil, true, true},
		{"both-denied", "", syscall.EPERM, syscall.EPERM, true, true},
		{"disappeared", "", syscall.ESRCH, nil, false, false},
		{"fallback-disappeared", "", syscall.EPERM, syscall.ESRCH, true, true},
		{"empty-identity", "", syscall.EPERM, nil, true, true},
		{"relative-identity", "cercano", syscall.EPERM, nil, true, true},
		{"malformed-argv-identity", "relative", nil, nil, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			err := verifyPreparedWritersAbsent(context.Background(), "/generation", func() ([]int, error) { return []int{42}, nil }, func(pid int) (string, []string, error) {
				if pid != 42 {
					t.Fatal(pid)
				}
				return tt.path, nil, tt.argvErr
			}, func(pid int) (Identity, error) { called = true; return Identity{Executable: tt.path}, tt.identityErr })
			if (err != nil) != tt.wantErr || called != tt.fallback {
				t.Fatalf("err=%v fallback=%v", err, called)
			}
		})
	}
}
