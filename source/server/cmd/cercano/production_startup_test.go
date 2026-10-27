package main

import (
	"context"
	"os"
	"path/filepath"
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
	// admission; even the supplied credential trap must remain unreachable.
	_, _, err = startGRPCServerWithStartup(config.Defaults(), "invalid address", nil,
		productionCredentialDependencies{openSecrets: func() (secrets.Store, error) {
			t.Fatal("credential stage entered")
			return nil, nil
		}}, p)
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
	_, _, err = startGRPCServerWithStartup(config.Defaults(), "invalid address", nil, productionCredentialDependencies{}, p)
	if err == nil || !strings.Contains(err.Error(), "service parity") {
		t.Fatalf("activation gate: %v", err)
	}
}
