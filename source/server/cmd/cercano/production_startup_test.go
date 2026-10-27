package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

func fixtureStartupPolicy(t *testing.T, legacy bool) productionStartupPolicy {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := canonicalGenerationStartup()
	p.authorities = productionAuthorityResolvers{
		configPath:       func() string { return filepath.Join(root, "config.yaml") },
		conversationPath: func() (string, error) { return filepath.Join(root, "override.db"), nil },
	}
	if legacy {
		p.legacy = true
		p.contract = conversation.CoexistenceDeclaration{}
	}
	return p
}

func TestProductionStartupLegacyOverride(t *testing.T) {
	p := fixtureStartupPolicy(t, true)
	paths, _ := p.authorities.resolve()
	// Exercise the supported ordinary override without touching a home authority.
	t.Setenv("CERCANO_CONVERSATIONS_DB", paths.conversations)
	var order []string
	p.authorities.configPath = func() string { order = append(order, "config"); return paths.config }
	p.authorities.conversationPath = func() (string, error) { order = append(order, "conversations"); return conversation.DefaultPath() }
	a, err := p.admit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	st, err := a.conversationStore(nil, func(path string) (conversation.Store, error) {
		calls++
		if strings.Join(order, ",") != "config,conversations" {
			t.Fatalf("store opened before authority resolution: %v", order)
		}
		if path != paths.conversations {
			t.Fatalf("opened %s", path)
		}
		return conversation.Open(path)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if calls != 1 || a.paths != paths {
		t.Fatalf("admission %+v, opens %d", a, calls)
	}
}

func TestProductionStartupGenerationRejectsLegacyBeforeEffects(t *testing.T) {
	p := fixtureStartupPolicy(t, false)
	paths, _ := p.authorities.resolve()
	st, err := conversation.Open(paths.conversations)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	before, err := os.ReadFile(paths.conversations)
	if err != nil {
		t.Fatal(err)
	}
	// This calls the real shared startup entry. Enterprise and runtime are below
	// admission; neither config nor the service callback may be reached.
	err = runAgentWithStartup(context.Background(), p, func(string) (config.Config, error) {
		t.Fatal("config stage entered")
		return config.Config{}, nil
	}, func(config.Config, productionStartupAdmission) {
		t.Fatal("service stage entered")
	})
	if err == nil || !strings.Contains(err.Error(), "generation authority admission") {
		t.Fatalf("admission: %v", err)
	}
	after, err := os.ReadFile(paths.conversations)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("legacy database changed during rejection")
	}
	if _, err := os.Stat(paths.config); !os.IsNotExist(err) {
		t.Fatalf("config fallback created: %v", err)
	}
}

func TestProductionStartupPreparedExactStore(t *testing.T) {
	p := fixtureStartupPolicy(t, false)
	paths, _ := p.authorities.resolve()
	if err := os.Chmod(filepath.Dir(paths.conversations), 0700); err != nil {
		t.Fatal(err)
	}
	if err := conversation.PrepareDevelopmentStore(context.Background(), paths.conversations); err != nil {
		t.Fatal(err)
	}
	a, err := p.admit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	st, err := conversation.OpenCoexistence(context.Background(), paths.conversations, p.contract)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	trap := func(string) (conversation.Store, error) { t.Fatal("legacy opener reached"); return nil, nil }
	got, err := a.conversationStore(st, trap)
	if err != nil || got != st {
		t.Fatalf("exact store identity lost: %v", err)
	}
	if _, err := a.conversationStore(nil, trap); err == nil {
		t.Fatal("missing supplied store accepted")
	}
	// Compatible authority alone is not service parity or permission to activate.
	_, _, err = startGRPCServerWithStartup(config.Defaults(), "invalid address", nil, productionCredentialDependencies{openSecrets: func() (secrets.Store, error) {
		t.Fatal("credential stage entered")
		return nil, nil
	}}, a)
	if err == nil || !strings.Contains(err.Error(), "service parity") {
		t.Fatalf("activation gate: %v", err)
	}
}

// Exercise the same pre-config entry used by main, not the later gRPC gate.
func TestRunAgentStartupAdmissionBeforeConfigAndServices(t *testing.T) {
	for _, compatible := range []bool{false, true} {
		t.Run(fmt.Sprint("compatible=", compatible), func(t *testing.T) {
			p := fixtureStartupPolicy(t, false)
			paths, _ := p.authorities.resolve()
			if err := os.Chmod(filepath.Dir(paths.config), 0700); err != nil {
				t.Fatal(err)
			}
			if compatible {
				if err := conversation.PrepareDevelopmentStore(context.Background(), paths.conversations); err != nil {
					t.Fatal(err)
				}
			} else {
				st, err := conversation.Open(paths.conversations)
				if err != nil {
					t.Fatal(err)
				}
				st.Close()
			}
			// Legacy config spelling would be migrated by Load; never let it open.
			original := []byte("port: \"54321\"\nopen_model: fixture-model\n")
			if err := os.WriteFile(paths.config, original, 0600); err != nil {
				t.Fatal(err)
			}
			before := startupFixtureFiles(t, filepath.Dir(paths.config))
			var order []string
			p.authorities.configPath = func() string { order = append(order, "resolve-config"); return paths.config }
			p.authorities.conversationPath = func() (string, error) {
				order = append(order, "resolve-conversations")
				return paths.conversations, nil
			}
			configLoads, serviceStarts := 0, 0
			err := runAgentWithStartup(context.Background(), p, func(string) (config.Config, error) {
				configLoads++
				return config.Config{}, nil
			}, func(config.Config, productionStartupAdmission) {
				// runServerMode owns crash logging and all later enterprise/runtime,
				// credential, store opening and task reconciliation stages.
				serviceStarts++
			})
			if configLoads != 0 || serviceStarts != 0 {
				t.Fatalf("config loads=%d, native/store service starts=%d", configLoads, serviceStarts)
			}
			want := "generation authority admission"
			if compatible {
				want = "service parity"
			}
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("got %v, want %s", err, want)
			}
			if got := strings.Join(order, ","); got != "resolve-config,resolve-conversations" {
				t.Fatalf("order: %s", got)
			}
			if after := startupFixtureFiles(t, filepath.Dir(paths.config)); !reflect.DeepEqual(before, after) {
				t.Fatal("startup mutated fixture files")
			}
		})
	}
}

func startupFixtureFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// SQLite read-only preflight may create transient WAL coordination files.
		// Compare durable authority/config bytes, not shared-memory locks.
		if entry.IsDir() || strings.HasSuffix(path, "-shm") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, "-wal") && len(data) == 0 {
			return nil // Empty read-only WAL sidecar contains no durable changes.
		}
		files[path] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestRunAgentStartupLegacyConfigAndOverrideOrder(t *testing.T) {
	p := fixtureStartupPolicy(t, true)
	paths, _ := p.authorities.resolve()
	t.Setenv("CERCANO_CONVERSATIONS_DB", paths.conversations)
	if err := os.WriteFile(paths.config, []byte("port: \"54321\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var order []string
	p.authorities.configPath = func() string { order = append(order, "config-path"); return paths.config }
	p.authorities.conversationPath = func() (string, error) { order = append(order, "conversation-path"); return conversation.DefaultPath() }
	err := runAgentWithStartup(context.Background(), p, func(path string) (config.Config, error) {
		order = append(order, "load")
		if path != paths.config {
			t.Fatalf("load path: %s", path)
		}
		return config.Load(path)
	}, func(cfg config.Config, a productionStartupAdmission) {
		order = append(order, "server")
		if cfg.Port != "54321" {
			t.Fatalf("loaded config lost: %s", cfg.Port)
		}
		if a.paths != paths || !a.legacy {
			t.Fatalf("admission lost: %+v", a)
		}
		st, err := a.conversationStore(nil, func(path string) (conversation.Store, error) {
			order = append(order, "store")
			if path != paths.conversations {
				t.Fatalf("override lost: %s", path)
			}
			return conversation.Open(path)
		})
		if err != nil {
			t.Fatal(err)
		}
		st.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(order, ","); got != "config-path,load,conversation-path,server,store" {
		t.Fatalf("order: %s", got)
	}
}

func TestRunAgentStartupLegacyWarningDefaults(t *testing.T) {
	p := fixtureStartupPolicy(t, true)
	capture, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	previous := os.Stderr
	os.Stderr = capture
	defer func() { os.Stderr = previous }()
	loads, runs := 0, 0
	err = runAgentWithStartup(context.Background(), p, func(string) (config.Config, error) {
		loads++
		return config.Config{Port: "invalid-partial"}, fmt.Errorf("fixture load failure")
	}, func(cfg config.Config, a productionStartupAdmission) {
		runs++
		if !reflect.DeepEqual(cfg, config.Defaults()) {
			t.Fatal("load failure did not use defaults")
		}
	})
	if err != nil || loads != 1 || runs != 1 {
		t.Fatalf("err=%v loads=%d runs=%d", err, loads, runs)
	}
	warning, err := os.ReadFile(capture.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(warning) != "[WARN] Failed to load config: fixture load failure (using defaults)\n" {
		t.Fatalf("warning: %q", warning)
	}
}

func TestRunAgentStartupPinsAuthorityAcrossDispatch(t *testing.T) {
	p := fixtureStartupPolicy(t, true)
	paths, _ := p.authorities.resolve()
	configResolves, dbResolves, loads, dispatches, opens := 0, 0, 0, 0, 0
	p.authorities.configPath = func() string {
		configResolves++
		if configResolves > 1 {
			t.Fatal("config authority re-resolved")
		}
		return paths.config
	}
	p.authorities.conversationPath = func() (string, error) {
		dbResolves++
		if dbResolves > 1 {
			t.Fatal("DB authority re-resolved")
		}
		return paths.conversations, nil
	}
	err := runAgentWithStartup(context.Background(), p, func(path string) (config.Config, error) {
		loads++
		if path != paths.config {
			t.Fatalf("config path %q", path)
		}
		return config.Defaults(), nil
	}, func(cfg config.Config, a productionStartupAdmission) {
		dispatches++
		if a.paths != paths || !a.legacy {
			t.Fatalf("dispatch authority: %+v", a)
		}
		// Changing ambient defaults after admission cannot redirect store opening.
		t.Setenv("CERCANO_CONVERSATIONS_DB", filepath.Join(t.TempDir(), "wrong.db"))
		_, err := a.conversationStore(nil, func(path string) (conversation.Store, error) {
			opens++
			if path != paths.conversations {
				t.Fatalf("store path %q", path)
			}
			return nil, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		// An explicitly supplied generation admission cannot silently fall back
		// to the legacy convenience wrapper at the actual downstream boundary.
		a.legacy = false
		_, _, err = startGRPCServerWithStartup(cfg, "invalid", nil, productionCredentialDependencies{
			openSecrets: func() (secrets.Store, error) { t.Fatal("credentials opened"); return nil, nil },
		}, a)
		if err == nil || !strings.Contains(err.Error(), "activation disabled") {
			t.Fatalf("downstream gate: %v", err)
		}
	})
	if err != nil || configResolves != 1 || dbResolves != 1 || loads != 1 || dispatches != 1 || opens != 1 {
		t.Fatalf("err=%v resolves=%d/%d loads=%d dispatches=%d opens=%d", err, configResolves, dbResolves, loads, dispatches, opens)
	}
}
