package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResetSetupBoundary(t *testing.T) {
	for _, kind := range []string{"fault", "stale", "competing-creation"} {
		t.Run(kind, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.yaml")
			if kind != "competing-creation" {
				if err := os.WriteFile(p, []byte("port: 1234\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			co, err := NewConfigCoordinator(p)
			if err != nil {
				t.Fatal(err)
			}
			base, err := co.Read(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			want := ErrConfigConflict
			if kind == "fault" {
				want = os.ErrPermission
				co.publish = func(string, []byte, os.FileMode) error { return os.ErrPermission }
			} else {
				if err := os.WriteFile(p, []byte("port: 5678\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(p)
			if _, err := co.ResetSetup(context.Background(), base); !errors.Is(err, want) {
				t.Fatalf("error=%v want=%v", err, want)
			}
			after, _ := os.ReadFile(p)
			if string(after) != string(before) {
				t.Fatal("rejected reset changed authority")
			}
		})
	}
}
