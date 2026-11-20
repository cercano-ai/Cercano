package main

import (
	"bytes"
	"cercano/source/server/internal/conversation"
	"context"
	"database/sql"
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
	if len(entries) != 2 || entries[0].Name() != "current-target.lock" || entries[1].Name() != "history.db" {
		t.Fatalf("unexpected files beyond migration coordination lock and original database: %v", entries)
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
	if err != nil || len(entries) != 1 || entries[0].Name() != "current-target.lock" {
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
	if err != nil || calls != 1 || readErr != nil || len(entries) != 1 || entries[0].Name() != "current-target.lock" {
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

// Exercises the actual CLI upgrade and startup admission without live discovery
// or process shutdown. The fixture does not call current schema initializers.
func TestProductionV2CLIStartupRoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"production", "fixture", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "history.db")
			ddl, err := os.ReadFile("../../internal/conversation/testdata/production-v2.sql")
			if err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", p)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(string(ddl)); err != nil {
				t.Fatal(err)
			}
			if kind != "production" {
				contract := "unknown-contract"
				if kind == "fixture" {
					contract = conversation.CurrentCoexistenceDeclaration().Contract
				}
				if _, err = db.Exec("UPDATE coexistence_metadata SET contract=?", contract); err != nil {
					t.Fatal(err)
				}
			}
			db.Close()
			before, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			declaration := conversation.DevelopmentCoexistenceDeclaration()
			if err := conversation.PreflightCoexistence(ctx, p, declaration); err == nil {
				t.Fatal("v2 admitted before upgrade")
			}
			if store, err := conversation.OpenCoexistence(ctx, p, declaration); err == nil {
				store.Close()
				t.Fatal("v2 opened before upgrade")
			}
			after, err := os.ReadFile(p)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("startup refusal changed database", err)
			}
			var out bytes.Buffer
			err = runUpgradeStoreChecked(ctx, []string{"--database", p, "--all-legacy-agents-stopped", "--interrupt-tasks"}, &out, func(context.Context) error { return nil })
			if kind != "production" {
				if err == nil {
					t.Fatal("unsupported marker upgraded")
				}
				after, e := os.ReadFile(p)
				if e != nil || !bytes.Equal(before, after) {
					t.Fatal("unsupported upgrade changed database", e)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "declared version upgrades preserve task statuses and ownership") {
				t.Fatal(out.String())
			}
			if err := conversation.PreflightCoexistence(ctx, p, declaration); err != nil {
				t.Fatal(err)
			}
			store, err := conversation.OpenCoexistence(ctx, p, declaration)
			if err != nil {
				t.Fatalf("v3 startup open: %v", err)
			}
			store.Close()
			db, err = sql.Open("sqlite", p)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var status, owner, epoch, contract string
			var version, uv, n int
			if err := db.QueryRow("SELECT status FROM delegated_tasks WHERE id='task'").Scan(&status); err != nil || status != "running" {
				t.Fatalf("task %q %v", status, err)
			}
			if err := db.QueryRow("SELECT owner,epoch FROM conversation_ownership WHERE conversation_id='legacy'").Scan(&owner, &epoch); err != nil || owner != "old-owner" || epoch != "epoch-7" {
				t.Fatalf("ownership %q %q %v", owner, epoch, err)
			}
			if err := db.QueryRow("SELECT contract,schema_version FROM coexistence_metadata").Scan(&contract, &version); err != nil || contract != declaration.Contract || version != declaration.SchemaVersion {
				t.Fatalf("marker %q %d %v", contract, version, err)
			}
			if err := db.QueryRow("PRAGMA user_version").Scan(&uv); err != nil || uv != version {
				t.Fatalf("user_version %d %v", uv, err)
			}
			if err := db.QueryRow("SELECT count(*) FROM execution_bindings").Scan(&n); err != nil || n != 0 {
				t.Fatalf("bindings %d %v", n, err)
			}
		})
	}
}
