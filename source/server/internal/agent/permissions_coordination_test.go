package agent

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestPermissionStoresDisjointUpdates(t *testing.T) {
	p := filepath.Join(t.TempDir(), "permissions.yaml")
	a, err := LoadPermissionStore(p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadPermissionStore(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetMode(ModeStrict); err != nil {
		t.Fatal(err)
	}
	if err := b.AddMCPAllow("first__*"); err != nil {
		t.Fatal(err)
	}
	if got := a.Mode(); got != ModeStrict {
		t.Fatalf("stale allowlist writer overwrote mode: %s", got)
	}
	if err := a.AddMCPAllow("second__*"); err != nil {
		t.Fatal(err)
	}
	if !b.IsMCPAllowed("first__x") || !b.IsMCPAllowed("second__x") {
		t.Fatal("lost disjoint allowlist update")
	}
}

func TestPermissionStoresConcurrent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "permissions.yaml")
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		s, err := LoadPermissionStore(p)
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := s.AddMCPAllow(fmt.Sprintf("tool_%d", i)); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	s, err := LoadPermissionStore(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.MCPAllowPatterns()) != 12 {
		t.Fatal(s.MCPAllowPatterns())
	}
}

// Only the test executable is launched, against the parent's private fixture.
func TestPermissionStoreProcessHelper(t *testing.T) {
	p := os.Getenv("CERCANO_PERMISSION_TEST_PATH")
	if p == "" {
		return
	}
	s, err := LoadPermissionStore(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(ModeStrict); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := s.AddMCPAllow(fmt.Sprintf("%s_%d", os.Getenv("CERCANO_PERMISSION_TEST_ID"), i)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPermissionStoresProcesses(t *testing.T) {
	p := filepath.Join(t.TempDir(), "permissions.yaml")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPermissionStoreProcessHelper$")
			cmd.Env = append(os.Environ(), "CERCANO_DISABLE_CREDENTIALS=1", "CERCANO_PERMISSION_TEST_PATH="+p, fmt.Sprintf("CERCANO_PERMISSION_TEST_ID=%d", i))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("helper: %v %s", err, out)
			}
		}(i)
	}
	wg.Wait()
	s, err := LoadPermissionStore(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.MCPAllowPatterns()) != 40 {
		t.Fatal(s.MCPAllowPatterns())
	}
}

func TestPermissionMutationMalformedAndInvalid(t *testing.T) {
	for _, text := range []string{"mode: [", "mode: unknown", "mcp_allow: {}", "mode: strict\nmode: bypass", "- bypass", "mode: strict\n---\nmode: bypass"} {
		t.Run(text, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "permissions.yaml")
			s, _ := LoadPermissionStore(p)
			if err := os.WriteFile(p, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			if err := s.SetMode(ModeBypass); err == nil {
				t.Fatal("accepted malformed state")
			}
			if err := s.AddMCPAllow("*"); err == nil {
				t.Fatal("accepted malformed state")
			}
			got, _ := os.ReadFile(p)
			if string(got) != text {
				t.Fatal("modified malformed file")
			}
			if s.mode != ModePermissive || len(s.mcpAllow) != 0 {
				t.Fatal("modified cache on error")
			}
		})
	}
	s := NewStaticPermissionStore(ModeStrict)
	if err := s.SetMode(ModeBypass); err == nil || s.Mode() != ModeStrict {
		t.Fatal("static setter must fail without applying")
	}
	if err := s.AddMCPAllow("*"); err == nil || s.IsMCPAllowed("anything") {
		t.Fatal("static allow must fail without applying")
	}
}

func TestPermissionModesAndUnknownFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "permissions.yaml")
	if err := os.WriteFile(p, []byte("mode: strict\nmcp_allow: [old]\nfuture: {enabled: true}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a, _ := LoadPermissionStore(p)
	b, _ := LoadPermissionStore(p)
	for _, mode := range []PermissionMode{ModeBypass, ModePermissive, ModeStrict} {
		if err := a.SetMode(mode); err != nil {
			t.Fatal(err)
		}
		if b.Mode() != mode {
			t.Fatal("stale reader")
		}
		if !reflect.DeepEqual(b.MCPAllowPatterns(), []string{"old"}) {
			t.Fatal("lost allowlist")
		}
	}
	before, _ := os.ReadFile(p)
	if err := a.SetMode("invalid"); err == nil {
		t.Fatal("accepted invalid mode")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("invalid mode changed disk")
	}
	var f permsFile
	if err := yaml.Unmarshal(after, &f); err != nil {
		t.Fatal(err)
	}
	if f.Extra["future"] == nil {
		t.Fatal("lost unknown field")
	}
	info, _ := os.Stat(p)
	if info.Mode().Perm() != 0600 {
		t.Fatal("changed access bits")
	}
	if err := b.AddMCPAllow("old"); err != nil {
		t.Fatal(err)
	}
	after, _ = os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("duplicate should not republish")
	}
}

func TestPermissionFailedPublication(t *testing.T) {
	p := filepath.Join(t.TempDir(), "permissions.yaml")
	s, _ := LoadPermissionStore(p)
	if err := s.SetMode(ModeStrict); err != nil {
		t.Fatal(err)
	}
	// Force rename failure after the fresh read, without global hooks or chmod
	// assumptions (which would fail when tests run as root).
	err := s.mutate(func(f *permsFile) bool {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
		f.Mode = string(ModeBypass)
		f.MCPAllow = []string{"*"}
		return true
	})
	if err == nil {
		t.Fatal("publication unexpectedly succeeded")
	}
	if s.mode != ModeStrict || len(s.mcpAllow) != 0 {
		t.Fatal("installed unpublished policy")
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".permissions-*"))
	if len(matches) != 0 {
		t.Fatal("leaked publication files", matches)
	}
}

func TestPermissionStoreSymlinkAuthority(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.yaml")
	alias := filepath.Join(dir, "permissions.yaml")
	a, _ := LoadPermissionStore(target)
	if err := a.SetMode(ModeStrict); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	b, err := LoadPermissionStore(alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.AddMCPAllow("tool"); err != nil {
		t.Fatal(err)
	}
	if !a.IsMCPAllowed("tool") || a.Mode() != ModeStrict {
		t.Fatal("wrong authority updated")
	}
	info, err := os.Lstat(alias)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("replaced symlink")
	}
}
