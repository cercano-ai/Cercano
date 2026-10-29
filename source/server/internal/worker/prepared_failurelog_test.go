package worker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	providers "cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/runner"
	"cercano/source/server/pkg/proto"
)

func TestPreparedFailureLogMissingAuthority(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	ws := NewWithFactories(func(*proto.StartTurn) (providers.Resolver, error) { return nil, nil }, func(*proto.StartTurn) (runner.ToolSvc, error) { return modeTestTools{}, nil }, WithPrepared())
	_, err := ws.buildDeps(context.Background(), &proto.StartTurn{Config: &proto.ConfigSnapshot{}}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil {
		t.Error("prepared dependency construction accepted missing failure-log authority")
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "cercano", "failures.jsonl")); !os.IsNotExist(err) {
		t.Errorf("ordinary HOME touched: %v", err)
	}
}

func failureLogTestWorker(options ...Option) *WorkerServer {
	return NewWithFactories(func(*proto.StartTurn) (providers.Resolver, error) { return nil, nil }, func(*proto.StartTurn) (runner.ToolSvc, error) { return modeTestTools{}, nil }, options...)
}

func TestPreparedFailureLogConsumption(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Build the real pinned policy, but only inspect its command: no child starts.
	policy, err := NewSpawnPolicy(policyFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	cmd := workerCommandWithPolicy(policy.binary, filepath.Join(policy.dir, "worker.sock"), policy)
	path := policy.failureLogPath()
	if !reflect.DeepEqual(cmd.Args, []string{policy.binary, "worker", "--socket", filepath.Join(policy.dir, "worker.sock"), "--prepared", "--failure-log", path}) || cmd.Env != nil {
		t.Fatalf("command=%v env=%v", cmd.Args, cmd.Env)
	}
	ws := failureLogTestWorker(WithPrepared(), WithFailureLogPath(cmd.Args[len(cmd.Args)-1]))
	for i := 0; i < 2; i++ {
		deps, err := ws.buildDeps(context.Background(), &proto.StartTurn{Config: &proto.ConfigSnapshot{}}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		deps.FailureLog.Log("isolation_test", nil)
		deps.RoutingLog.Close()
		if err := deps.FailureLog.Close(); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Count(string(data), "isolation_test") != 2 {
		t.Fatalf("private log=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "cercano", "failures.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("ordinary HOME failure log touched: %v", err)
	}
	ordinary := failureLogTestWorker()
	deps, err := ordinary.buildDeps(context.Background(), &proto.StartTurn{Config: &proto.ConfigSnapshot{}}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	deps.FailureLog.Log("ordinary_test", nil)
	deps.FailureLog.Close()
	deps.RoutingLog.Close()
	data, err = os.ReadFile(filepath.Join(home, ".config", "cercano", "failures.jsonl"))
	if err != nil || !strings.Contains(string(data), "ordinary_test") {
		t.Fatalf("ordinary default changed: %q %v", data, err)
	}
}

func TestPreparedFailureLogInvalidAuthority(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(dir, 0700)
	public := filepath.Join(dir, "public")
	os.Mkdir(public, 0755)
	target := filepath.Join(dir, "target")
	os.WriteFile(target, []byte("untouched"), 0600)
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	loose := filepath.Join(dir, "loose")
	os.WriteFile(loose, nil, 0644)
	os.Chmod(loose, 0644)
	for _, path := range []string{"", "relative", filepath.Join(dir, "missing", "failures"), filepath.Join(public, "failures"), link, filepath.Join(alias, "failures"), dir, loose} {
		ws := failureLogTestWorker(WithPrepared(), WithFailureLogPath(path))
		called := false
		ws.providerFactory = func(*proto.StartTurn) (providers.Resolver, error) { called = true; return nil, nil }
		if _, err := ws.buildDeps(context.Background(), &proto.StartTurn{Config: &proto.ConfigSnapshot{}}, nil, nil, nil, nil, nil, nil, nil, nil, nil); err == nil {
			t.Errorf("accepted %q", path)
		}
		if called {
			t.Errorf("constructed providers before rejecting %q", path)
		}
	}
	data, _ := os.ReadFile(target)
	if string(data) != "untouched" {
		t.Fatal("symlink target changed")
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("HOME touched: %v %v", entries, err)
	}
}
