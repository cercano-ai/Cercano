package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/secrets"
)

func TestProductionAuthorityResolution(t *testing.T) {
	root := t.TempDir()
	want := productionAuthorityPaths{filepath.Join(root, "config.yaml"), filepath.Join(root, "override.db")}
	configCalls, conversationCalls := 0, 0
	resolvers := productionAuthorityResolvers{
		configPath:       func() string { configCalls++; return want.config },
		conversationPath: func() (string, error) { conversationCalls++; return want.conversations, nil },
	}
	got, err := resolvers.resolve()
	if err != nil || got != want || configCalls != 1 || conversationCalls != 1 {
		t.Fatalf("resolution = %+v, %v; calls %d/%d", got, err, configCalls, conversationCalls)
	}
	sentinel := errors.New("fixture resolution failure")
	resolvers.conversationPath = func() (string, error) { return "", sentinel }
	if _, err := resolvers.resolve(); !errors.Is(err, sentinel) {
		t.Fatalf("lost resolver failure: %v", err)
	}
	if _, err := (productionAuthorityResolvers{}).resolve(); err == nil {
		t.Fatal("missing resolvers accepted")
	}
}

func TestProductionCredentialChildEnvironment(t *testing.T) {
	if os.Getenv("CERCANO_TEST_CREDENTIAL_CHILD") == "1" {
		// Only an explicit invocation-local memory opener is reachable in this
		// enabled child. HOME alone is not native backend isolation.
		if os.Getenv(secrets.DisableEnv) != "0" {
			t.Fatal("child admission not explicitly enabled")
		}
		// Enabled negative control: incompatible/unprepared storage must stop
		// before even a native-shaped opener can be entered.
		_, _, preflightErr := newProductionDevelopmentHostWithDependencies(context.Background(), developmentHostOptions{State: t.TempDir()}, nil, developmentProductionDependencies{
			openSecrets: func() (secrets.Store, error) { t.Fatal("native opener trap entered before preflight"); return nil, nil },
		})
		if preflightErr == nil {
			t.Fatal("unprepared store accepted")
		}
		calls := 0
		d := productionCredentialDependencies{openSecrets: func() (secrets.Store, error) { calls++; return secrets.NewMemory(), nil }}
		ordinary, err := d.open(secrets.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err := ordinary.Set("fixture", "not-a-secret"); err != nil {
			t.Fatal(err)
		}
		prepared, err := d.open(secrets.Options{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := prepared.Set("fixture", "not-a-secret"); !errors.Is(err, secrets.ErrReadOnly) {
			t.Fatalf("freeze: %v", err)
		}
		if calls != 2 {
			t.Fatalf("fake opener calls %d", calls)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProductionCredentialChildEnvironment$")
	// Do not inherit provider credentials or ambient configuration overrides.
	cmd.Env = []string{"CERCANO_TEST_CREDENTIAL_CHILD=1", secrets.DisableEnv + "=0", "HOME=" + t.TempDir(), "TMPDIR=" + t.TempDir()}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("owned child: %v\n%s", err, output)
	}
}

func TestProductionCredentialErrors(t *testing.T) {
	if _, err := (productionCredentialDependencies{}).open(secrets.Options{}); err == nil {
		t.Fatal("missing opener accepted")
	}
	d := productionCredentialDependencies{openSecrets: func() (secrets.Store, error) { return nil, nil }}
	if _, err := d.open(secrets.Options{}); err == nil {
		t.Fatal("nil store accepted")
	}
}
