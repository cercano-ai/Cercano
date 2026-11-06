package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gproto "google.golang.org/protobuf/proto"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
)

// A copied test executable supplies the exact installed-layout launch entrypoint.
// Hosts below use the ordinary composition helper, not a second host graph.
func TestInstalledSessionLaunchChild(t *testing.T) {
	if os.Getenv("CERCANO_INSTALLED_SESSION_CHILD") != "1" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := runDevelopmentLaunch(ctx, []string{"--installed", "--state", os.Getenv("CERCANO_INSTALLED_STATE"), "--runtime-root", os.Getenv("CERCANO_INSTALLED_RUNTIME")}, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestInstalledSessionAdmissionAndPinnedReconnect(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "is-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "TMPDIR"} {
		t.Setenv(key, root)
	}
	o, _ := developmentProductionFixture(t)
	runtime := filepath.Join(root, "rt")
	if err := os.Mkdir(runtime, 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	build := fmt.Sprintf("installed-%x", sha256.Sum256(body))
	hosts := filepath.Join(o.State, "hosts")
	if err := os.Mkdir(hosts, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "cercano")
	start := func(version string) string {
		generation := filepath.Join(root, "Cellar", "cercano-agent", version)
		binary := filepath.Join(generation, "bin", "cercano")
		if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(binary, body, 0700); err != nil {
			t.Fatal(err)
		}
		host := "installed-" + version
		base := filepath.Join(hosts, fmt.Sprintf("%x", sha256.Sum256([]byte(generation))))
		token := base + ".token"
		if err := devhostauth.Create(token); err != nil {
			t.Fatal(err)
		}
		auth, err := devhostauth.Load(host, token)
		if err != nil {
			t.Fatal(err)
		}
		options := o
		options.Generation, options.Host, options.RuntimeRoot, options.TokenFile, options.Installed = generation, host, runtime, token, true
		originalHost, originalBuild := o.Identity.Host, o.Identity.Build
		// Generated messages contain runtime state and must not be copied by value.
		identity := gproto.Clone(o.Identity).(*proto.LifecycleIdentity)
		identity.Host, identity.Build = host, build
		if o.Identity.Host != originalHost || o.Identity.Build != originalBuild {
			t.Fatal("installed generation mutated the source identity")
		}
		options.Identity = identity
		binding := developmentBinding(options)
		options.startupAdmission.bounded = &binding
		startOrdinaryGenerationFixture(t, options, auth, func(_ *server.Server, _ developmentHostOptions, endpoint string, _ func()) {
			// Fixture publication only: admission below still authenticates readiness,
			// acquires a real owner and persists before publishing discovery.
			record := launchSelection{Endpoint: endpoint, Identity: identity, TokenFile: token, ConfigAuthority: options.ConfigAuthority}
			if err := launchExclusive(base+".json", record); err != nil {
				t.Fatal(err)
			}
		})
		return binary
	}
	launch := func() launchSelection {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, alias, "-test.run=^TestInstalledSessionLaunchChild$")
		cmd.Env = append(os.Environ(), "CERCANO_INSTALLED_SESSION_CHILD=1", "CERCANO_INSTALLED_STATE="+o.State, "CERCANO_INSTALLED_RUNTIME="+runtime)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("launch: %v: %s", err, output)
		}
		var selection launchSelection
		if err := launchRead(strings.TrimSpace(string(output)), &selection); err != nil {
			t.Fatalf("selection %q: %v", output, err)
		}
		return selection
	}
	dial := func(s launchSelection) *agentclient.Client {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, err := agentclient.DialAuthenticatedPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: s.Endpoint, HostInstance: s.Identity.Host, Build: s.Identity.Build, Contract: s.Identity.Contract, SchemaVersion: s.Identity.Schema, ConversationID: s.Conversation, OwnershipEpoch: s.Epoch}, s.TokenFile)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	a := start("A")
	if err := os.Symlink(a, alias); err != nil {
		t.Fatal(err)
	}
	sa := launch()
	ca := dial(sa)
	b := start("B")
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(b, alias); err != nil {
		t.Fatal(err)
	}
	// An installed B cannot adopt a live legacy database. Rejection must be
	// before host records, target publication or any conversion.
	legacy := filepath.Join(root, "legacy")
	if err := os.Mkdir(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(legacy, "conversations.db")
	st, err := conversation.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := os.Chmod(db, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "config.yaml"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	probeCtx, stopProbe := context.WithTimeout(context.Background(), 10*time.Second)
	cmd := exec.CommandContext(probeCtx, alias, "-test.run=^TestInstalledSessionLaunchChild$")
	cmd.Env = append(os.Environ(), "CERCANO_INSTALLED_SESSION_CHILD=1", "CERCANO_INSTALLED_STATE="+legacy, "CERCANO_INSTALLED_RUNTIME="+runtime)
	output, probeErr := cmd.CombinedOutput()
	stopProbe()
	if probeErr == nil || !strings.Contains(string(output), "prepared development store required") {
		t.Fatalf("legacy admission: %v %s", probeErr, output)
	}
	after, err := os.ReadFile(db)
	if err != nil || string(before) != string(after) {
		t.Fatalf("legacy changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(legacy, "hosts")); !os.IsNotExist(err) {
		t.Fatalf("legacy spawned host: %v", err)
	}
	sb := launch()
	dial(sb)
	// Compatibility is checked against the persisted binding, never repaired
	// by discovering B or attaching through the legacy singleton.
	badCtx, stopBad := context.WithTimeout(context.Background(), 5*time.Second)
	bad, badErr := agentclient.DialAuthenticatedPinnedSession(badCtx, agentclient.PinnedSessionIntent{Endpoint: sa.Endpoint, HostInstance: sa.Identity.Host, Build: sa.Identity.Build, Contract: "incompatible-contract", SchemaVersion: sa.Identity.Schema, ConversationID: sa.Conversation, OwnershipEpoch: sa.Epoch}, sa.TokenFile)
	stopBad()
	if bad != nil {
		bad.Close()
	}
	if badErr == nil {
		t.Fatal("incompatible pinned contract accepted")
	}
	if sa.Identity.Host == sb.Identity.Host || sa.Conversation == sb.Conversation || sa.Generation == sb.Generation {
		t.Fatal("new session reused A")
	}
	current, err := os.ReadFile(filepath.Join(o.State, "current-target.json"))
	if err != nil {
		t.Fatal(err)
	}
	var discovered launchSelection
	if err := json.Unmarshal(current, &discovered); err != nil {
		t.Fatal(err)
	}
	if discovered.Identity.Host != sb.Identity.Host {
		t.Fatalf("discovery not B: %s", current)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ca.ReconnectPinnedSession(ctx); err != nil {
		t.Fatalf("A reconnect after B: %v", err)
	}
	dial(sa) // Explicit persisted binding, not current-target discovery.
	controls, err := launchControls(ctx, sa)
	if err != nil {
		t.Fatal(err)
	}
	defer controls.Close()
	owners, err := controls.LifecycleList(ctx, &proto.LifecycleControlRequest{Target: sa.Identity, Management: true})
	if err != nil {
		t.Fatal(err)
	}
	if owners.Identity.Host != sa.Identity.Host {
		t.Fatal("A lost its serving host")
	}
}
