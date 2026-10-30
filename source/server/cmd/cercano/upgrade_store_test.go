package main

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// Shutdown is injected: these tests never discover or signal ambient processes.
func TestAutomaticUpgradeStopThenCheck(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	t.Setenv("CERCANO_PORT", "")
	for _, tc := range []struct{ name, config, stopErr, checkErr, want string }{
		{"success", "port: 54321", "", "", ""},
		{"missing", "", "", "", "existing regular config"},
		{"invalid", "[", "", "", "load upgrade shutdown config"},
		{"zero", "port: '0'", "", "", "loopback endpoint"},
		{"remote", "port: '1.2.3.4:80'", "", "", "loopback endpoint"},
		{"failed-stop", "port: 54321", "identity changed", "", "shutdown failed"},
		{"remaining", "port: 54321", "", "known process remains", "known process remains"},
		{"unknown", "port: 54321", "", "discovery unavailable", "discovery unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			db, cfg := filepath.Join(dir, "missing.db"), filepath.Join(dir, "config.yaml")
			if tc.config != "" {
				if err := os.WriteFile(cfg, []byte(tc.config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			previous := upgradeHostStop
			defer func() { upgradeHostStop = previous }()
			var calls []string
			upgradeHostStop = func(ctx context.Context, endpoint netip.AddrPort) error {
				calls = append(calls, "stop")
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 30*time.Second || endpoint.String() != "127.0.0.1:54321" {
					t.Fatalf("invalid shutdown: %v %v", endpoint, deadline)
				}
				if tc.stopErr != "" {
					return errors.New(tc.stopErr)
				}
				return nil
			}
			err := runUpgradeStoreChecked(context.Background(), []string{"--database", db, "--all-legacy-agents-stopped", "--check-only", "--stop-hosts-config", cfg}, &bytes.Buffer{}, func(context.Context) error {
				if strings.Join(calls, ",") != "stop" {
					t.Fatal(calls)
				}
				calls = append(calls, "check")
				if tc.checkErr != "" {
					return errors.New(tc.checkErr)
				}
				return nil
			})
			if tc.want == "" {
				if err != nil || strings.Join(calls, ",") != "stop,check" {
					t.Fatalf("%v %v", err, calls)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal(err)
			}
			if tc.config == "" || tc.name == "invalid" || tc.name == "zero" || tc.name == "remote" {
				if len(calls) != 0 {
					t.Fatal(calls)
				}
			}
			if tc.stopErr != "" && strings.Join(calls, ",") != "stop" {
				t.Fatal(calls)
			}
			if _, err := os.Stat(db); !os.IsNotExist(err) {
				t.Fatal("database accessed", err)
			}
			matches, _ := filepath.Glob(db + "*")
			if len(matches) != 0 {
				t.Fatal(matches)
			}
		})
	}
}
