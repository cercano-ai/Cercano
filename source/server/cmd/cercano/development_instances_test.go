package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/internal/brewrestart"
	pb "cercano/source/server/pkg/proto"
)

func TestDevelopmentBlockedRecoveryConsent(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		record, dead, live, yes bool
		wantPrompt, wantStop    bool
		wantErr                 bool
	}{
		{"dead intent", false, true, false, false, false, false, false},
		{"dead record", true, true, false, false, false, false, false},
		{"uncertain intent no", false, false, false, false, true, false, true},
		{"uncertain intent yes", false, false, false, true, true, false, false},
		{"uncertain record no", true, false, false, false, true, false, true},
		{"uncertain record yes", true, false, false, true, true, false, false},
		{"live admission closed no", true, false, true, false, true, false, true},
		{"live admission closed yes", true, false, true, true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			generation := filepath.Join(root, "build")
			authority := filepath.Join(root, "config.yaml")
			base := filepath.Join(root, fmt.Sprintf("%x", sha256.Sum256([]byte(generation))))
			host := "0123456789abcdef0123456789abcdef"
			if err := launchExclusive(base+".starting", map[string]string{"host": host, "generation": generation, "config_authority": authority}); err != nil {
				t.Fatal(err)
			}
			var s launchSelection
			if tc.record {
				s = launchSelection{Identity: &pb.LifecycleIdentity{Host: host}, Endpoint: "127.0.0.1:1"}
				if err := launchExclusive(base+".json", s); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(base + ".starting")
			prompts, stops := 0, 0
			ops := developmentRecoveryOps{
				absent: func(context.Context, string) error {
					if tc.dead {
						return nil
					}
					return fmt.Errorf("uncertain")
				},
				prepare: func(context.Context, string, netip.AddrPort) (brewrestart.Identity, error) {
					if tc.live {
						return brewrestart.Identity{Executable: filepath.Join(generation, "cercano")}, nil
					}
					return brewrestart.Identity{}, fmt.Errorf("unverified")
				},
				stop: func(ctx context.Context, _ string, _ netip.AddrPort, _ brewrestart.Identity, validate func() error) error {
					stops++
					if _, ok := ctx.Deadline(); !ok {
						t.Fatal("stop missing deadline")
					}
					return validate()
				},
			}
			confirm := func(message string) bool {
				prompts++
				if tc.live {
					if !strings.Contains(message, "sessions will disconnect") {
						t.Fatal(message)
					}
				} else if !strings.Contains(message, "old conversations remain owned there") {
					t.Fatal(message)
				}
				return tc.yes
			}
			err := developmentReplaceBlockedWith(context.Background(), base, generation, authority, s, fmt.Errorf("admission closed"), confirm, ops)
			if (err != nil) != tc.wantErr || (prompts != 0) != tc.wantPrompt || (stops != 0) != tc.wantStop {
				t.Fatalf("err=%v prompts=%d stops=%d", err, prompts, stops)
			}
			after, _ := os.ReadFile(base + ".starting")
			if string(before) != string(after) {
				t.Fatal("old intent changed")
			}
		})
	}
}

func TestDevelopmentInstanceDiscoveryAndInventory(t *testing.T) {
	state, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hosts := filepath.Join(state, "hosts")
	os.Mkdir(hosts, 0700)
	generation := filepath.Join(state, "generation")
	authority := filepath.Join(state, "config.yaml")
	legacy := filepath.Join(hosts, fmt.Sprintf("%x", sha256.Sum256([]byte(generation))))
	if got, err := developmentInstanceBase(legacy); err != nil || got != legacy {
		t.Fatalf("legacy lookup %s %v", got, err)
	}
	first, host1, err := developmentNewInstance(legacy, generation, authority)
	if err != nil {
		t.Fatal(err)
	}
	second, host2, err := developmentNewInstance(legacy, generation, authority)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || host1 == host2 {
		t.Fatal("instance identity reused")
	}
	if got, err := developmentInstanceBase(legacy); err != nil || got != second {
		t.Fatalf("new lookup %s %v", got, err)
	}
	for _, base := range []string{first, second} {
		var intent map[string]string
		if err := launchRead(base+".starting", &intent); err != nil || !developmentInstanceMatches(base, intent) {
			t.Fatalf("invalid instance %s %v", base, err)
		}
	}
	db := filepath.Join(state, "conversations.db")
	seen := map[string]bool{}
	release, err := developmentUpgradeGuardWith(context.Background(), db, func(_ context.Context, host string) error { seen[host] = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	release()
	if !seen[host1] || !seen[host2] {
		t.Fatalf("inventory missed instance: %v", seen)
	}
	release, err = developmentUpgradeGuardWith(context.Background(), db, func(_ context.Context, host string) error {
		if host == host1 {
			return fmt.Errorf("uncertain old host")
		}
		return nil
	})
	if err == nil {
		release()
		t.Fatal("migration overlooked old instance")
	}
	// Source lookup is by exact host, never by the most recent generation
	// pointer. Both old and new immutable selections must remain resolvable.
	destination := &pb.LifecycleIdentity{Host: "destination", Store: db, Contract: "contract", Schema: 1}
	for base, host := range map[string]string{first: host1, second: host2} {
		id := &pb.LifecycleIdentity{Host: host, Store: db, Contract: "contract", Schema: 1}
		if err := launchExclusive(base+".json", launchSelection{Identity: id, TokenFile: base + ".token"}); err != nil {
			t.Fatal(err)
		}
		lock, err := developmentInstanceLock(base)
		if err != nil {
			t.Fatal(err)
		}
		guard := developmentRecoveryGuardWithVerifier(state, destination, func(_ context.Context, got string) error {
			if got != host {
				t.Fatalf("source confused: %s != %s", got, host)
			}
			return nil
		})
		if release, err := guard(context.Background(), host); err == nil {
			release()
			t.Fatal("source lock seized")
		}
		lock.Close()
		release, err := guard(context.Background(), host)
		if err != nil {
			t.Fatalf("exact source lookup: %v", err)
		}
		release()
	}
	// A pointer cannot authorize arbitrary paths or a different generation.
	os.WriteFile(legacy+".instance", []byte(`{"Base":"../elsewhere"}`), 0600)
	if _, err := developmentInstanceBase(legacy); err == nil {
		t.Fatal("pointer escaped hosts directory")
	}
}

func TestDevelopmentMigrationExcludesFirstLaunch(t *testing.T) {
	state, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	release, err := developmentUpgradeGuardWith(context.Background(), filepath.Join(state, "conversations.db"), func(context.Context, string) error { t.Fatal("no hosts yet"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	f, err := os.OpenFile(filepath.Join(state, "current-target.lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := devFlock(f, devLockExclusive|devLockNonBlock); err == nil {
		devFlock(f, devLockUnlock)
		t.Fatal("first launch not excluded from migration")
	}
}
