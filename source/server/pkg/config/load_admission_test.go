package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAdmissionRejectsUnsavableDocuments(t *testing.T) {
	for i, input := range []string{"port: 1234\n---\nport: 5678\n", "# comment only\n", "", "port: [secret-fixture\n"} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(p, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			c, err := Load(p)
			if err == nil {
				_, saveErr := SaveSnapshot(c, p, c.Snapshot())
				t.Fatalf("Load accepted invalid authority; subsequent Save error: %v", saveErr)
			}
			if !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), "reload") || strings.Contains(err.Error(), "secret-fixture") {
				t.Fatalf("unsafe/unhelpful error: %v", err)
			}
			data, _ := os.ReadFile(p)
			if string(data) != input {
				t.Fatal("invalid file changed")
			}
		})
	}
}

func TestLoadAdmissionAbsentAndUnreadable(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	next, err := SaveSnapshot(c, p, c.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveSnapshot(c.WithSnapshot(next), p, next); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if _, err := Load(directory); err == nil || !strings.Contains(err.Error(), directory) || !strings.Contains(err.Error(), "reload") {
		t.Fatalf("unreadable target error: %v", err)
	}
}
