package agent

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"cercano/source/server/internal/llm"
)

func TestPermissionInvalidAuthority(t *testing.T) {
	for name, data := range map[string]string{
		"multi":          "mode: bypass\nmcp_allow: ['*']\n---\nmode: strict\n",
		"unknown":        "mode: typo\nmcp_allow: ['*']\n",
		"empty":          "",
		"omitted":        "mcp_allow: ['*']\n",
		"null":           "null\n",
		"bad_allow":      "mode: bypass\nmcp_allow: {tool: true}\n",
		"duplicate":      "mode: bypass\nmode: strict\n",
		"trailing_empty": "mode: bypass\n---\n",
		"syntax":         "mode: bypass\nmcp_allow: [\n",
	} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "permissions.yaml")
			write := func(s string) {
				t.Helper()
				if err := os.WriteFile(p, []byte(s), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(data)
			if _, err := LoadPermissionStore(p); err == nil {
				t.Error("initial invalid authority accepted")
			}
			for _, reader := range []string{"mode", "patterns", "allowed"} {
				write("mode: strict\nmcp_allow: [safe]\n")
				s, err := LoadPermissionStore(p)
				if err != nil {
					t.Fatal(err)
				}
				write(data)
				switch reader {
				case "mode":
					s.Mode()
				case "patterns":
					s.MCPAllowPatterns()
				case "allowed":
					s.IsMCPAllowed("evil")
				}
				if s.mode != ModeStrict || len(s.mcpAllow) != 1 || s.mcpAllow[0] != "safe" {
					t.Errorf("%s installed invalid policy: %s %v", reader, s.mode, s.mcpAllow)
				}
				for _, mutate := range []func() error{func() error { return s.SetMode(ModeBypass) }, func() error { return s.AddMCPAllow("*") }} {
					if err := mutate(); err == nil {
						t.Error("invalid authority mutation succeeded")
					}
					got, _ := os.ReadFile(p)
					if string(got) != data {
						t.Error("failed mutation changed disk")
					}
				}
			}
		})
	}
}

func TestPermissionEstablishedDisappearance(t *testing.T) {
	for _, committed := range []bool{false, true} {
		p := filepath.Join(t.TempDir(), "permissions.yaml")
		s, err := LoadPermissionStore(p)
		if err != nil {
			t.Fatal(err)
		}
		if committed {
			if err := s.SetMode(ModeStrict); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.WriteFile(p, []byte("mode: strict\n"), 0600); err != nil {
				t.Fatal(err)
			}
			s.MCPAllowPatterns()
		}
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		if err := s.AddMCPAllow("*"); err == nil {
			t.Error("missing established authority accepted")
		}
		if err := s.SetMode(ModeBypass); err == nil {
			t.Error("missing established authority overwritten")
		}
		if s.Mode() != ModeStrict {
			t.Error("strict policy relaxed")
		}
	}
}

// Characterization, not a promise of alias support: rename splits hardlinks.
func TestPermissionHardlinkCharacterization(t *testing.T) {
	p := filepath.Join(t.TempDir(), "permissions.yaml")
	if err := os.WriteFile(p, []byte("mode: strict\n"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := p + ".alias"
	if err := os.Link(p, alias); err != nil {
		t.Fatal(err)
	}
	a, err := LoadPermissionStore(p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadPermissionStore(alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetMode(ModeBypass); err != nil {
		t.Fatal(err)
	}
	if b.Mode() != ModeStrict || a.Mode() != ModeBypass {
		t.Fatal("hardlink publication behavior changed")
	}
}

func TestPermissionConcurrentMissingAuthority(t *testing.T) {
	p := filepath.Join(t.TempDir(), "permissions.yaml")
	if err := os.WriteFile(p, []byte("mode: strict\nmcp_allow: [safe]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var stores []*PermissionStore
	for i := 0; i < 8; i++ {
		s, err := LoadPermissionStore(p)
		if err != nil {
			t.Fatal(err)
		}
		stores = append(stores, s)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, s := range stores {
		wg.Add(1)
		go func(s *PermissionStore) {
			defer wg.Done()
			if err := s.AddMCPAllow("*"); err == nil {
				t.Error("missing authority recreated")
			}
			if err := s.SetMode(ModeBypass); err == nil {
				t.Error("missing authority overwritten")
			}
			if !GateDecision(s.Mode(), llm.PermW) || s.IsMCPAllowed("evil") {
				t.Error("policy relaxed")
			}
		}(s)
	}
	wg.Wait()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("authority recreated: %v", err)
	}
}

func TestPermissionReaderRefreshesWholePolicy(t *testing.T) {
	for _, reader := range []string{"mode", "patterns", "allowed"} {
		p := filepath.Join(t.TempDir(), "permissions.yaml")
		s, err := LoadPermissionStore(p)
		if err != nil {
			t.Fatal(err)
		}
		if s.Mode() != ModePermissive {
			t.Fatal("first-creation default changed")
		}
		if err := os.WriteFile(p, []byte("mode: strict\nmcp_allow: [safe]\n"), 0600); err != nil {
			t.Fatal(err)
		}
		switch reader {
		case "mode":
			s.Mode()
		case "patterns":
			s.MCPAllowPatterns()
		case "allowed":
			s.IsMCPAllowed("safe")
		}
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		if s.Mode() != ModeStrict || !s.IsMCPAllowed("safe") {
			t.Fatalf("%s failed to refresh whole policy", reader)
		}
	}
}
