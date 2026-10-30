package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
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

// An unavailable detector must fail closed before even opening a missing DB.
// No live discovery: both outcomes are controlled by the injected preflight.
func TestOfflineUpgradeDiscoveryErrorBeforeOpen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing.db")
	discoveryErr := errors.New("synthetic discovery unavailable")
	calls := 0
	err := runUpgradeStoreChecked(context.Background(), []string{"--database", p, "--all-legacy-agents-stopped"}, &bytes.Buffer{}, func(context.Context) error {
		calls++
		return discoveryErr
	})
	if !errors.Is(err, discoveryErr) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	entries, err := os.ReadDir(filepath.Dir(p))
	if err != nil || len(entries) != 0 {
		t.Fatalf("database opened before preflight: entries=%v err=%v", entries, err)
	}
}

// Lower-level detector diagnostic only; not migration safety acceptance.
func TestOfflineUpgradeCheckOnlyNeverOpensStore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing.db")
	calls := 0
	err := runUpgradeStoreChecked(context.Background(), []string{"--database", p, "--all-legacy-agents-stopped", "--check-only"}, &bytes.Buffer{}, func(context.Context) error {
		calls++
		return nil
	})
	entries, readErr := os.ReadDir(filepath.Dir(p))
	if err != nil || calls != 1 || readErr != nil || len(entries) != 0 {
		t.Fatalf("err=%v calls=%d entries=%v readErr=%v", err, calls, entries, readErr)
	}
}

// No native discovery or shutdown: the automatic path must refuse before either.
func TestAutomaticUpgradeRefusesWithoutSpawnDrainBarrier(t *testing.T) {
	for _, checkOnly := range []bool{false, true} {
		t.Run(fmt.Sprint(checkOnly), func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "history.db")
			before := []byte("untouched store")
			if err := os.WriteFile(p, before, 0644); err != nil {
				t.Fatal(err)
			}
			previous := upgradeHostStop
			upgradeHostStop = func(context.Context, netip.AddrPort) error { t.Fatal("requested shutdown"); return nil }
			defer func() { upgradeHostStop = previous }()
			args := []string{"--database", p, "--all-legacy-agents-stopped", "--stop-hosts-config", filepath.Join(dir, "missing-config")}
			if checkOnly {
				args = append(args, "--check-only")
			}
			err := runUpgradeStoreChecked(context.Background(), args, &bytes.Buffer{}, func(context.Context) error { t.Fatal("requested discovery"); return nil })
			if err == nil || !strings.Contains(err.Error(), "host lacks verified worker-spawn/drain shutdown control; upgrade not performed") {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(p)
			info, _ := os.Stat(p)
			entries, _ := os.ReadDir(dir)
			if !bytes.Equal(before, after) || info.Mode().Perm() != 0644 || len(entries) != 1 {
				t.Fatal("store changed")
			}
		})
	}
}
