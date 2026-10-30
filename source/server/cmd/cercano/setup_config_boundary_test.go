package main

import (
	"cercano/source/server/pkg/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupConfigBoundary(t *testing.T) {
	for _, kind := range []string{"malformed", "unreadable", "permission", "absent"} {
		t.Run(kind, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.yaml")
			switch kind {
			case "malformed":
				if err := os.WriteFile(p, []byte("cloud_api_key: [NONSECRET_FIXTURE"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unreadable":
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			case "permission":
				if err := os.WriteFile(p, []byte("port: '1234'"), 0000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(p, 0600) })
				if _, err := os.ReadFile(p); err == nil {
					t.Skip("permission denial unavailable for this uid")
				}
			}
			c, err := loadSetupConfig(p)
			if kind != "absent" {
				if err == nil {
					t.Fatal("invalid authority admitted")
				}
				if !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), "permissions") || strings.Contains(err.Error(), "NONSECRET_FIXTURE") {
					t.Fatalf("unsafe diagnostic: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			next, err := config.SaveSnapshot(c, p, c.Snapshot())
			if err != nil {
				t.Fatal(err)
			}
			c = c.WithSnapshot(next)
			c.Port = "12345"
			next, err = config.SaveSnapshot(c, p, c.Snapshot())
			if err != nil {
				t.Fatal(err)
			}
			c = c.WithSnapshot(next)
			got, err := config.Load(p)
			if err != nil || got.Port != c.Port || got.Snapshot().Revision != next.Revision {
				t.Fatal("setup second save lost revision")
			}
		})
	}
}
