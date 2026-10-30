package config

import (
	"bytes"
	"context"
	"errors"
	"gopkg.in/yaml.v3"
	"os"
	"testing"
)

func aliasFixture(t *testing.T, text string) (string, *ConfigCoordinator, ConfigSnapshot) {
	t.Helper()
	path, s, _ := coordinationFixture(t)
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	base, err := s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return path, s, base
}

func TestCoordinatedReviewInvalidSave(t *testing.T) {
	path, s, base := coordinationFixture(t)
	before, _ := os.ReadFile(path)
	cfg := Defaults()
	cfg.ToolLoop.MaxIterations = -100
	next, err := s.Save(context.Background(), base, cfg)
	if err == nil {
		t.Error("invalid Save published")
	}
	if next.Revision != (ConfigRevision{}) {
		t.Error("invalid Save returned revision")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Error("invalid Save changed bytes")
	}
	if _, err := s.Read(context.Background()); err != nil {
		t.Errorf("Read after rejected Save: %v", err)
	}
}

func TestCoordinatedReviewAliasConflicts(t *testing.T) {
	for _, mapping := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			name := "scalar"
			if mapping {
				name = "mapping"
			}
			if reverse {
				name += "-reverse"
			}
			t.Run(name, func(t *testing.T) {
				text := "unknown_endpoint: &endpoint http://old.invalid\nollama_url: *endpoint\n"
				a := change("unknown_endpoint", "&endpoint http://new.invalid")
				b := change("ollama_url", "http://literal.invalid")
				if mapping {
					text = "unknown_loop: &loop {max_iterations: 20, context_limit: 0}\ntool_loop: *loop\n"
					a = ConfigChange{Path: []string{"unknown_loop", "max_iterations"}, YAML: []byte("30")}
					b = ConfigChange{Path: []string{"tool_loop", "max_iterations"}, YAML: []byte("40")}
				}
				_, s, base := aliasFixture(t, text)
				if reverse {
					a, b = b, a
				}
				if _, err := s.Change(context.Background(), base, a); err != nil {
					t.Fatal(err)
				}
				if _, err := s.Change(context.Background(), base, b); !errors.Is(err, ErrConfigConflict) {
					t.Fatalf("stale alias edit: %v", err)
				}
			})
		}
	}
}

func TestCoordinatedReviewAliasDisjoint(t *testing.T) {
	for _, target := range []string{"unknown_loop", "tool_loop"} {
		t.Run(target, func(t *testing.T) {
			path, s, base := aliasFixture(t, "unknown_loop: &loop {max_iterations: 20, future: old}\ntool_loop: *loop\nport: '1111'\n")
			ctx := context.Background()
			if _, err := s.Change(ctx, base, ConfigChange{Path: []string{target, "max_iterations"}, YAML: []byte("30")}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Change(ctx, base, ConfigChange{Path: []string{target, "future"}, YAML: []byte("new")}); err != nil {
				t.Fatalf("disjoint descendant: %v", err)
			}
			if _, err := s.Change(ctx, base, change("port", "'2222'")); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Read(ctx); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var decoded map[string]any
			if err := yaml.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"unknown_loop", "tool_loop"} {
				m := decoded[key].(map[string]any)
				if m["max_iterations"] != 30 || m["future"] != "new" {
					t.Fatalf("alias semantics lost: %v", decoded)
				}
			}
			if !bytes.Contains(data, []byte("&loop")) || !bytes.Contains(data, []byte("*loop")) {
				t.Fatal("anchors/aliases stripped")
			}
		})
	}
}

func TestCoordinatedReviewInvalidAliasResults(t *testing.T) {
	for _, c := range []ConfigChange{
		change("unknown_endpoint", "http://unanchored.invalid"),
		{Path: []string{"unknown_endpoint"}, Delete: true},
		change("unknown_endpoint", "&endpoint [*endpoint]"),
		change("ollama_url", "*missing"),
		change("tool_loop", "{max_iterations: -100}"),
		change("unknown_endpoint", "one\n---\ntwo"),
	} {
		t.Run(string(c.YAML), func(t *testing.T) {
			path, s, base := aliasFixture(t, "unknown_endpoint: &endpoint http://old.invalid\nollama_url: *endpoint\n")
			before, _ := os.ReadFile(path)
			next, err := s.Change(context.Background(), base, c)
			if err == nil || next.Revision != (ConfigRevision{}) {
				t.Fatalf("invalid result admitted: %v", err)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("invalid result published")
			}
			if _, err := s.Read(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCoordinatedReviewAliasGuards(t *testing.T) {
	for _, text := range []string{"unknown: &cycle [*cycle]\n", "unknown: *missing\n"} {
		if _, err := configDocument([]byte(text)); err == nil {
			t.Fatal("invalid alias document accepted")
		}
	}
	n := &yaml.Node{Kind: yaml.AliasNode}
	n.Alias = n
	if _, err := resolveConfigAlias(n); err == nil {
		t.Fatal("alias cycle not guarded")
	}
}
