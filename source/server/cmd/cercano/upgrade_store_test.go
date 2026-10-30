package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineUpgradeKnownAgentBeforeChanges(t *testing.T) {
	p := filepath.Join(t.TempDir(), "history.db")
	before := []byte("untouched existing database")
	if err := os.WriteFile(p, before, 0600); err != nil {
		t.Fatal(err)
	}
	err := runUpgradeStoreChecked(context.Background(), []string{"--database", p, "--all-legacy-agents-stopped"}, &bytes.Buffer{}, func(context.Context) error { return refuseKnownUpgradeAgents("123 /some/legacy/cercano\n", 456) })
	if err == nil || !strings.Contains(err.Error(), "pid=123") {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("modified database before refusal")
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Fatal("created database sidecars")
	}
}
func TestOfflineUpgradeDetectionLimits(t *testing.T) {
	if err := refuseKnownUpgradeAgents("123 /path/cercano\n124 /path/custom-writer\n", 123); err != nil {
		t.Fatal(err)
	}
	if err := refuseKnownUpgradeAgents("124 /path/cercano-agent\n", 123); err == nil {
		t.Fatal("known agent accepted")
	}
	if err := runUpgradeStore(context.Background(), []string{"--database", "/unused"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "ALL legacy") {
		t.Fatal(err)
	}
}
