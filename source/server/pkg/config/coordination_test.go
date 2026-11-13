package config

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func coordinationFixture(t *testing.T) (string, *ConfigCoordinator, ConfigSnapshot) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("port: '1111'\nollama_url: http://fixture.invalid\nunknown_future: {keep: true}\nllama_server:\n  context_size: 4096\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewConfigCoordinator(path)
	if err != nil {
		t.Fatal(err)
	}
	base, err := s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return path, s, base
}
func change(path, value string) ConfigChange {
	return ConfigChange{Path: []string{path}, YAML: []byte(value)}
}
func TestCoordinatedDistinctAndConflictingChanges(t *testing.T) {
	path, a, base := coordinationFixture(t)
	b, err := NewConfigCoordinator(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 2)
	for i, s := range []*ConfigCoordinator{a, b} {
		wg.Add(1)
		go func(i int, s *ConfigCoordinator) {
			defer wg.Done()
			<-start
			c := change("port", "'2222'")
			if i == 1 {
				c = change("ollama_url", "http://second.invalid")
			}
			_, err := s.Change(context.Background(), base, c)
			errs <- err
		}(i, s)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["port"] != "2222" || raw["ollama_url"] != "http://second.invalid" || raw["unknown_future"] == nil {
		t.Fatal("lost distinct or unknown field")
	}
	if _, err := a.Change(context.Background(), base, change("port", "'3333'")); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("stale field: %v", err)
	}
	if _, err := a.Save(context.Background(), base, Defaults()); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("stale full save: %v", err)
	}
	// Windows has no Unix permission bits (Go reports 0666 for writable files).
	if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("mode changed")
	}
}
func TestCoordinatedPublicationFailure(t *testing.T) {
	path, s, base := coordinationFixture(t)
	before, _ := os.ReadFile(path)
	sentinel := errors.New("injected publication failure")
	s.publish = func(string, []byte, os.FileMode) error { return sentinel }
	next, err := s.Change(context.Background(), base, change("port", "'2222'"))
	if !errors.Is(err, sentinel) || next.Revision != (ConfigRevision{}) {
		t.Fatal("failure returned a committed revision")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed write changed authority")
	}
	// Exercise the actual publisher's rename failure and temporary-file cleanup.
	dst := filepath.Join(filepath.Dir(path), "directory")
	if err := os.Mkdir(dst, 0700); err != nil {
		t.Fatal(err)
	}
	if err := publishConfig(dst, []byte("fixture"), 0600); err == nil {
		t.Fatal("rename unexpectedly succeeded")
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*"))
	if len(matches) != 0 {
		t.Fatal("temporary files leaked")
	}
}
func TestCoordinatedAuthorityFailures(t *testing.T) {
	for _, mode := range []string{"malformed", "missing", "hardlink", "dangling", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			path, s, base := coordinationFixture(t)
			switch mode {
			case "malformed":
				if err := os.WriteFile(path, []byte("port: ["), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".alias"); err != nil {
					t.Fatal(err)
				}
			case "dangling", "redirect":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				target := path + ".target"
				if mode == "redirect" {
					if err := os.WriteFile(target, base.data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Change(context.Background(), base, change("port", "'2222'")); err == nil {
				t.Fatal("unsafe authority accepted")
			}
		})
	}
}
func TestCoordinatedSymlinkAndExplicitOverride(t *testing.T) {
	path, a, base := coordinationFixture(t)
	alias := path + ".alias"
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	b, err := NewConfigCoordinator(alias)
	if err != nil {
		t.Fatal(err)
	}
	if a.path != b.path {
		t.Fatal("alias lock authority differs")
	}
	next, err := b.Change(context.Background(), base, ConfigChange{Path: []string{"llama_server", "context_size"}, Delete: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Change(context.Background(), next, ConfigChange{Path: []string{"llama_server", "context_size"}, YAML: []byte("0")}); err == nil {
		t.Fatal("invalid override accepted")
	}
	info, err := os.Lstat(alias)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced")
	}
	data, _ := os.ReadFile(path)
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if c.LlamaServer.ContextSize != nil {
		t.Fatal("deleted override restored")
	}
}
func TestCoordinatedFullSaveAndCancellation(t *testing.T) {
	_, s, base := coordinationFixture(t)
	c := Defaults()
	c.Port = "2222"
	next, err := s.Save(context.Background(), base, c)
	if err != nil {
		t.Fatal(err)
	}
	if next.Revision == base.Revision {
		t.Fatal("revision unchanged")
	}
	if _, err := s.Save(context.Background(), next, c.WithSharedReadOnly()); err == nil {
		t.Fatal("readonly save allowed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Read(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

// Each child holds its own stale base; only the target-side OS lock can
// serialize their fresh reads and preserve both fields across atomic rename.
func TestCoordinatedProcesses(t *testing.T) {
	if path := os.Getenv("CERCANO_CONFIG_COORDINATION_FIXTURE"); path != "" {
		s, err := NewConfigCoordinator(path)
		if err != nil {
			t.Fatal(err)
		}
		base, err := s.Read(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		field := os.Getenv("CERCANO_CONFIG_COORDINATION_FIELD")
		if err := os.WriteFile(path+"."+field+".ready", nil, 0600); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			if _, err := os.Stat(path + ".go"); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("fixture barrier timeout")
			}
			time.Sleep(time.Millisecond)
		}
		value := "'2222'"
		if field == "ollama_url" {
			value = "http://child.invalid"
		}
		if _, err := s.Change(context.Background(), base, change(field, value)); err != nil {
			t.Fatal(err)
		}
		return
	}
	path, _, _ := coordinationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmds := make([]*exec.Cmd, 0, 2)
	outputs := make([]*bytes.Buffer, 0, 2)
	for _, field := range []string{"port", "ollama_url"} {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCoordinatedProcesses$", "-test.count=1")
		cmd.Env = append(os.Environ(), "CERCANO_CONFIG_COORDINATION_FIXTURE="+path, "CERCANO_CONFIG_COORDINATION_FIELD="+field)
		out := new(bytes.Buffer)
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
		outputs = append(outputs, out)
	}
	for _, field := range []string{"port", "ollama_url"} {
		for {
			if _, err := os.Stat(path + "." + field + ".ready"); err == nil {
				break
			}
			if ctx.Err() != nil {
				t.Fatal("child ready timeout")
			}
			time.Sleep(time.Millisecond)
		}
	}
	if err := os.WriteFile(path+".go", nil, 0600); err != nil {
		t.Fatal(err)
	}
	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("child: %v: %s", err, outputs[i])
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Config
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Port != "2222" || got.OllamaURL != "http://child.invalid" {
		t.Fatal("cross-process update lost")
	}
}

func TestCoordinatedSaveMatchesLegacySerialization(t *testing.T) {
	path, s, base := coordinationFixture(t)
	c := Defaults()
	legacy := filepath.Join(t.TempDir(), "legacy.yaml")
	if err := saveFixture(t, c, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(context.Background(), base, c); err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(legacy)
	got, _ := os.ReadFile(path)
	if !bytes.Equal(want, got) {
		t.Fatal("coordinated serialization differs from Save")
	}
	if _, err := c.WithSharedReadOnly().PrepareConfigCoordination(path); err == nil {
		t.Fatal("readonly preparation allowed")
	}
}
