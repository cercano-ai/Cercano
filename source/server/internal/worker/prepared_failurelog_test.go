package worker

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	providers "cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/secrets"
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
	if !reflect.DeepEqual(cmd.Args, []string{policy.binary, "worker", "--socket", filepath.Join(policy.dir, "worker.sock"), "--prepared", "--failure-log", path, "--routing-log", policy.routingLogPath()}) || cmd.Env != nil {
		t.Fatalf("command=%v env=%v", cmd.Args, cmd.Env)
	}
	ws := failureLogTestWorker(WithPrepared(), WithFailureLogPath(cmd.Args[6]), WithRoutingLogPath(cmd.Args[8]))
	for i := 0; i < 2; i++ {
		deps, err := ws.buildDeps(context.Background(), &proto.StartTurn{Config: &proto.ConfigSnapshot{}}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		deps.FailureLog.Log("isolation_test", nil)
		deps.RoutingLog.Log("routing_isolation_test", nil)
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
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("HOME touched: %v %v", entries, err)
	}
	data, err = os.ReadFile(policy.routingLogPath())
	if err != nil || strings.Count(string(data), "routing_isolation_test") != 2 {
		t.Fatalf("private routing log=%q err=%v", data, err)
	}
	ordinary := failureLogTestWorker()
	deps, err := ordinary.buildDeps(context.Background(), &proto.StartTurn{Config: &proto.ConfigSnapshot{}}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	deps.FailureLog.Log("ordinary_test", nil)
	deps.RoutingLog.Log("ordinary_routing_test", nil)
	deps.FailureLog.Close()
	deps.RoutingLog.Close()
	data, err = os.ReadFile(filepath.Join(home, ".config", "cercano", "failures.jsonl"))
	if err != nil || !strings.Contains(string(data), "ordinary_test") {
		t.Fatalf("ordinary default changed: %q %v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(home, ".config", "cercano", "turn-routing.jsonl"))
	if err != nil || !strings.Contains(string(data), "ordinary_routing_test") {
		t.Fatalf("ordinary routing default changed: %q %v", data, err)
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
		ws := failureLogTestWorker(WithPrepared(), WithFailureLogPath(path), WithRoutingLogPath(filepath.Join(dir, "turn-routing.jsonl")))
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

// Authorities are explicit private test resources, never normal-HOME fallbacks.
func preparedLogTestOptions(t *testing.T) []Option {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return []Option{WithPrepared(), WithFailureLogPath(filepath.Join(dir, "failures.jsonl")), WithRoutingLogPath(filepath.Join(dir, "turn-routing.jsonl"))}
}

func TestPreparedRoutingLogInvalidAuthority(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(dir, 0700)
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
	public := filepath.Join(dir, "public")
	os.Mkdir(public, 0755)
	loose := filepath.Join(dir, "loose")
	os.WriteFile(loose, nil, 0644)
	os.Chmod(loose, 0644)
	for _, path := range []string{"", "relative", dir + "/./routing", filepath.Join(dir, "missing", "routing"), filepath.Join(public, "routing"), link, filepath.Join(alias, "routing"), dir, loose} {
		ws := failureLogTestWorker(WithPrepared(), WithFailureLogPath(filepath.Join(dir, "failures.jsonl")), WithRoutingLogPath(path))
		called := false
		ws.providerFactory = func(*proto.StartTurn) (providers.Resolver, error) { called = true; return nil, nil }
		ws.toolsFactory = func(*proto.StartTurn) (runner.ToolSvc, error) { called = true; return modeTestTools{}, nil }
		if _, err := ws.buildDeps(context.Background(), &proto.StartTurn{Config: &proto.ConfigSnapshot{}}, nil, nil, nil, nil, nil, nil, nil, nil, nil); err == nil {
			t.Errorf("accepted routing authority %q", path)
		}
		if called {
			t.Errorf("constructed dependencies before rejecting %q", path)
		}
	}
	data, _ := os.ReadFile(target)
	if string(data) != "untouched" {
		t.Fatal("symlink target changed")
	}
	if _, err := os.Lstat(filepath.Join(dir, "failures.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("opened failure log before routing validation: %v", err)
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("HOME touched: %v %v", entries, err)
	}
}

type preparedAssemblyNoNetwork struct{ t *testing.T }

func (trap preparedAssemblyNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	trap.t.Error("unexpected HTTP request during dependency construction")
	return nil, context.Canceled
}

// Exercise the successful production dependency assembly (neither factory is
// replaced), with an empty provider snapshot and a memory-only secret store.
// No turn, HTTP server, inference, credentials, or tool invocation is needed.
func TestPreparedRoutingLogFullAssembly(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Empty snapshot constructs no cloud/Ollama clients. Trap any accidental
	// default HTTP use as well; dependency construction must not make requests.
	previousTransport := http.DefaultTransport
	http.DefaultTransport = preparedAssemblyNoNetwork{t}
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	policy, err := NewSpawnPolicy(policyFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	ws := NewWithSecretStore(secrets.NewMemory(), WithPrepared(), WithFailureLogPath(policy.failureLogPath()), WithRoutingLogPath(policy.routingLogPath()))
	deps, err := ws.buildDeps(context.Background(), &proto.StartTurn{Config: &proto.ConfigSnapshot{}}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer deps.FailureLog.Close()
	defer deps.RoutingLog.Close()
	if deps.Tools == nil || deps.Tools.Registry() == nil {
		t.Fatal("real tool assembly missing")
	}
	deps.FailureLog.Log("assembled_failure", nil)
	deps.RoutingLog.Log("assembled_route", nil)
	for path, event := range map[string]string{ws.failureLogPath: "assembled_failure", ws.routingLogPath: "assembled_route"} {
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), event) {
			t.Fatalf("private log %s: %q %v", path, data, err)
		}
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("HOME touched: %v %v", entries, err)
	}
}
