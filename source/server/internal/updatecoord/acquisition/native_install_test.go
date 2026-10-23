package acquisition

import (
	"archive/zip"
	"bytes"
	"cercano/source/server/internal/updatecoord/activation"
	"cercano/source/server/internal/updatecoord/enrollment"
	"cercano/source/server/internal/updatecoord/selection"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/installcontext"
	"context"
	"crypto/sha256"
	"debug/macho"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/updatecoord/archivecheck"
)

// TestNativeTemporaryInstall is opt-in because it builds six real executables.
// Normal startup requires a second explicit opt-in after isolation review.
// It stops before updater orchestration and publication.
// All executing artifacts are acquired and staged, never the build outputs.
func TestNativeTemporaryInstall(t *testing.T) {
	if os.Getenv("CERCANO_NATIVE_INSTALL_TEST") != "1" {
		t.Skip("set CERCANO_NATIVE_INSTALL_TEST=1 for native build/acquire/stage")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("requires native macOS arm64")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mkdir := func(name string) string {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
		return p
	}
	home, config, tmp := mkdir("home"), mkdir("config"), mkdir("tmp")
	versions := mkdir("installation/versions")
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=" + config, "XDG_CACHE_HOME=" + mkdir("cache"), "TMPDIR=" + tmp, "PATH=/usr/bin:/bin", "LANG=C"}
	// HOME does not isolate the macOS login keychain. Deny securityd and all
	// network access at the OS boundary as well; fail rather than run unsandboxed.
	profile := `(version 1)(allow default)(deny network*)(deny mach-lookup (global-name "com.apple.securityd") (global-name "com.apple.securityd.xpc"))(deny file-read* (subpath "` + filepath.Join(os.Getenv("HOME"), "Library/Keychains") + `"))`
	sandbox := "/usr/bin/sandbox-exec"
	run := func(path string, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		argv := append([]string{"-p", profile, path}, args...)
		cmd := exec.CommandContext(ctx, sandbox, argv...)
		cmd.Env, cmd.Dir = env, home
		b, err := cmd.CombinedOutput()
		return string(b), err
	}
	if output, err := run("/usr/bin/true"); err != nil {
		t.Fatalf("sandbox unavailable: %v: %s", err, output)
	}
	_, here, _, _ := runtime.Caller(0)
	server := filepath.Clean(filepath.Join(filepath.Dir(here), "../../.."))
	cli := filepath.Clean(filepath.Join(server, "../clients/cli"))
	layout := archivecheck.CompleteReleaseLayout()
	fixtureVersions := []string{"0.0.0-fixture.1", "0.0.0-fixture.2"}
	specs := make([]targetSpec, 0, 2)
	originals := make(map[string]map[string][32]byte)
	for _, version := range fixtureVersions {
		var archive bytes.Buffer
		zw := zip.NewWriter(&archive)
		originals[version] = make(map[string][32]byte)
		for i, member := range layout.Required {
			dir, pkg := server, "./cmd/cercano"
			if i == 1 {
				dir, pkg = cli, "."
			}
			if i == 2 {
				pkg = "./cmd/cercano-updater"
			}
			output := filepath.Join(mkdir("build/"+version), filepath.Base(member))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-ldflags=-X main.version="+version, "-o", output, pkg)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64")
			b, err := cmd.CombinedOutput()
			cancel()
			if err != nil {
				t.Fatalf("build %s: %v\n%s", member, err, b)
			}
			image, err := macho.Open(output)
			if err != nil {
				t.Fatal(err)
			}
			if image.Cpu != macho.CpuArm64 {
				t.Fatal("not arm64", member)
			}
			image.Close()
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			originals[version][member] = sha256.Sum256(data)
			w, err := zw.Create(member)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = w.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		specs = append(specs, targetSpec{path: "fixture/darwin-arm64/" + version + ".zip", data: archive.Bytes()})
	}
	fx := newFixtureSpecs(t, time.Now().Add(time.Hour), nil, specs...)
	defer fx.server.Close()
	// Trust only this ephemeral TLS server; reject any redirect beyond it.
	client := fx.server.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return fmt.Errorf("fixture redirects forbidden") }
	var stagedDirs []string
	for i, spec := range specs {
		version := fixtureVersions[i]
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		receipt, err := acquire(ctx, Options{TrustedRoot: fx.root, MetadataBaseURL: fx.server.URL + "/metadata/", TargetBaseURL: fx.server.URL + "/targets/", CacheDir: mkdir("tuf/" + version), OutputDir: mkdir("downloads/" + version)}, spec.path, client)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		opts := archivecheck.Options{Format: archivecheck.Zip, Layout: layout, Bounds: archivecheck.Bounds{MaxMembers: 3, MaxCompressedBytes: 256 << 20, MaxUncompressedBytes: 512 << 20, MaxMemberBytes: 256 << 20}, Identity: &archivecheck.Identity{Length: receipt.Length, SHA256: receipt.SHA256}}
		staged, err := archivecheck.StageFile(ctx, receipt.OutputPath, versions, opts, archivecheck.StageOptions{FileMode: 0400, ExecutableMode: 0500, DirMode: 0700, Executables: layout.Required, StagingPattern: "release-" + version + "-"})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		stagedDirs = append(stagedDirs, staged.Dir)
		for _, member := range layout.Required {
			path := filepath.Join(staged.Dir, member)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(data)
			if digest != originals[version][member] {
				t.Fatal("staged image changed", member)
			}
			arg := "version"
			if strings.HasSuffix(member, "-cli") {
				arg = "--version"
			}
			output, err := run(path, arg)
			want := filepath.Base(member) + " v" + version + "\n"
			if err != nil || output != want {
				t.Fatalf("%s: %v: got %q want %q", member, err, output, want)
			}
			t.Logf("%s %s sha256=%x version=%q", version, member, digest, strings.TrimSpace(output))
		}
		output, err := run(filepath.Join(staged.Dir, "bin/cercano-updater"), "execute")
		if err == nil || !strings.Contains(output, "usage: cercano-updater version | execute --input") {
			t.Fatalf("updater must refuse missing protected handoff: %v %s", err, output)
		}
	}
	// Install stable images from the acquired first release, not build outputs.
	installRoot := filepath.Dir(versions)
	for _, name := range []string{"cercano", "cercano-cli"} {
		data, err := os.ReadFile(filepath.Join(stagedDirs[0], "bin", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(installRoot, name), data, 0500); err != nil {
			t.Fatal(err)
		}
	}
	record, err := enrollment.Create(context.Background(), installRoot)
	if err != nil {
		t.Fatal(err)
	}
	stateRoot := filepath.Join(home, "Library", "Application Support")
	stateDir := filepath.Join(stateRoot, "Cercano", "updater", record.InstallID)
	if err = os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	locators, err := installcontext.LocatorArgs(installRoot, record.InstallID)
	if err != nil {
		t.Fatal(err)
	}
	for i, dir := range stagedDirs {
		selected := activation.Selection{SchemaVersion: 1, InstallID: record.InstallID, Generation: int64(i + 1), SelectedVersion: fixtureVersions[i], StagedVersionDir: filepath.Base(dir), VerifiedArtifactSHA256: strings.Repeat("a", 64)}
		raw, err := json.Marshal(selected)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(stateDir, selection.FileName), raw, 0600); err != nil {
			t.Fatal(err)
		}
		for _, dbState := range []string{"missing", "corrupt", "inaccessible"} {
			db := filepath.Join(stateDir, "state.db")
			if dbState != "missing" {
				if err = os.WriteFile(db, []byte("not SQL"), 0600); err != nil {
					t.Fatal(err)
				}
				if dbState == "inaccessible" {
					if err = os.Chmod(db, 0000); err != nil {
						t.Fatal(err)
					}
				}
			}
			for _, name := range []string{"cercano", "cercano-cli"} {
				arg := "version"
				if name == "cercano-cli" {
					arg = "--version"
				}
				for _, stable := range []bool{true, false} {
					path, args := filepath.Join(installRoot, name), []string{arg}
					if !stable {
						path = filepath.Join(dir, "bin", name)
						args = append(append([]string{}, locators...), arg)
					}
					output, err := run(path, args...)
					want := name + " v" + fixtureVersions[i] + "\n"
					if err != nil || output != want {
						t.Fatalf("stable=%t DB=%s %s: %v got %q want %q", stable, dbState, name, err, output, want)
					}
					t.Logf("stable=%t DB=%s %s", stable, dbState, strings.TrimSpace(output))
				}
			}
			if os.Getenv("CERCANO_NATIVE_NORMAL_START_TEST") == "1" {
				for _, stable := range []bool{true, false} {
					path, args := filepath.Join(installRoot, "cercano"), []string{"agent"}
					if !stable {
						path = filepath.Join(dir, "bin/cercano")
						args = append(append([]string{}, locators...), "agent")
					}
					t.Run(fmt.Sprintf("normal/%s/%s/stable=%t", fixtureVersions[i], dbState, stable), func(t *testing.T) {
						cliPath, cliArgs := filepath.Join(installRoot, "cercano-cli"), []string{}
						if !stable {
							cliPath, cliArgs = filepath.Join(dir, "bin/cercano-cli"), locators
						}
						nativeNormalInteraction(t, path, args, cliPath, cliArgs, fixtureVersions[i], env, home, profile)
						if dbState == "missing" {
							if _, err := os.Stat(db); !os.IsNotExist(err) {
								t.Fatalf("normal startup created bookkeeping: %v", err)
							}
						} else {
							if dbState == "inaccessible" {
								info, err := os.Stat(db)
								if err != nil || info.Mode().Perm() != 0 {
									t.Fatalf("inaccessible bookkeeping permissions changed: %v", err)
								}
								if err := os.Chmod(db, 0600); err != nil {
									t.Fatal(err)
								}
								defer os.Chmod(db, 0000)
							}
							b, err := os.ReadFile(db)
							if err != nil || string(b) != "not SQL" {
								t.Fatalf("normal startup changed corrupt bookkeeping: %q %v", b, err)
							}
						}
					})
				}
			}
			if dbState != "missing" {
				if err = os.Remove(db); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	for _, dir := range stagedDirs {
		if _, err := os.Stat(dir); err != nil {
			t.Fatal("old/candidate not retained", err)
		}
	}
	t.Log("staged two complete current-source fixture releases; protected stable and child offline version startup without SQL. Normal agent RPC evidence is reported only by enabled normal subtests. CLI PTY evidence is reported only by enabled cli-pty subtests. No updater drain, publication or compatibility proof.")
}

// nativeNormalInteraction is deliberately separately gated. No auto-launch RPC client:
// every RPC targets the allocated endpoint and must prove this exact child PID.
func nativeNormalInteraction(t *testing.T, image string, args []string, cliImage string, cliArgs []string, version string, baseEnv []string, home, denyProfile string) {
	// Exercise the client's no-credential attestation/transport policy as well
	// as the explicit child setting; never merely trust a reachable endpoint.
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	const answer = "native fixture response"
	const prompt = "Reply with the native fixture response; do not use tools."
	var chats atomic.Int64
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			io.WriteString(w, `{"models":[{"name":"native-fixture","model":"native-fixture","size":1,"details":{"family":"qwen2","parameter_size":"1B","quantization_level":"Q4_0"}}]}`)
		case "/api/show":
			io.WriteString(w, `{"capabilities":["completion","tools"],"model_info":{"general.architecture":"qwen2","qwen2.context_length":32768}}`)
		case "/api/chat":
			var body struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&body); err != nil {
				t.Errorf("fixture request: %v", err)
				http.Error(w, "bad request", 400)
				return
			}
			for _, m := range body.Messages {
				if strings.Contains(m.Content, prompt) {
					chats.Add(1)
					break
				}
			}
			io.WriteString(w, `{"model":"native-fixture","message":{"role":"assistant","content":"native fixture response"},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":4}`+"\n")
		default:
			t.Errorf("unexpected provider operation (no downloads allowed): %s %s", r.Method, r.URL.Path)
			http.Error(w, "fixture endpoint not supported", 404)
		}
	}))
	defer fixture.Close()
	_, fixturePort, err := net.SplitHostPort(strings.TrimPrefix(fixture.URL, "http://"))
	if err != nil || fixturePort == config.Defaults().Port {
		t.Fatal("unsafe fixture endpoint")
	}
	// Reserve an ephemeral port, never the developer's default listener. The
	// production agent binds :port; sandbox bind permission must account for that,
	// while inbound traffic is restricted to the loopback local endpoint.
	reservation, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := reservation.Addr().String()
	_, port, _ := net.SplitHostPort(addr)
	if port == config.Defaults().Port {
		reservation.Close()
		t.Fatal("allocated default agent port")
	}
	cfg := config.Defaults()
	cfg.Port, cfg.OpenRuntime, cfg.OllamaURL, cfg.OpenModel = port, "ollama", fixture.URL, "native-fixture"
	// Exercise the existing in-process execution mode for this first startup
	// slice; do not spawn worker groups whose cleanup would outlive a failed test.
	cfg.LocusMode, cfg.ExecutionMode = "open_only", "in_process"
	for _, tier := range []config.Tier{config.TierEveryday, config.TierMostCapable, config.TierFastLight, config.TierFastLightText, config.TierEmbedding} {
		cfg.Models.SetOverride("ollama", tier, "native-fixture")
	}
	cfg.Agent.ShutdownOnLastClient = false
	cfg.CloudProfiles = nil
	cfg.ActiveCloudProfile, cfg.CloudAPIKey = "", ""
	cfg.LlamaServer.Enabled, cfg.MistralRS.Enabled = false, false
	cfg.LlamaServer.ModelDirs = []string{filepath.Join(home, "models")}
	cfg.MistralRS.ModelDirs = []string{filepath.Join(home, "models")}
	configPath := filepath.Join(home, ".config", "cercano", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	// Allow only the fixture's exact remote endpoint. No default agent/Ollama,
	// external HTTP, DNS, proxy or inherited credential environment is allowed.
	// Seatbelt's address grammar accepts localhost, not literal 127.0.0.1.
	profile := denyProfile + fmt.Sprintf(`(allow network-outbound (remote ip "localhost:%s"))(allow network-bind (local ip "*:%s"))(allow network-inbound (local ip "localhost:%s"))`, fixturePort, port, port)
	// Keep the allowlist explicit: never append os.Environ(), even for locale
	// or proxy convenience. Check the caller cannot accidentally broaden it.
	allowed := map[string]bool{"HOME": true, "XDG_CONFIG_HOME": true, "XDG_CACHE_HOME": true, "TMPDIR": true, "PATH": true, "LANG": true}
	for _, entry := range baseEnv {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || !allowed[key] {
			t.Fatalf("unexpected child environment entry: %q", key)
		}
	}
	env := append(append([]string{}, baseEnv...), "CERCANO_DISABLE_CREDENTIALS=1")
	logFile, err := os.CreateTemp(home, "normal-start-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd := exec.Command("/usr/bin/sandbox-exec", append([]string{"-p", profile, image}, args...)...)
	cmd.Env, cmd.Dir, cmd.Stdout, cmd.Stderr = env, home, logFile, logFile
	reservation.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	reaped := false
	defer func() {
		if !reaped { // Failure containment only: exact owned child, never pkill/group sweep.
			_ = cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("owned child was not reaped")
			}
		}
		if t.Failed() {
			b, _ := os.ReadFile(logFile.Name())
			t.Logf("normal-start log:\n%s", b)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var client *agentclient.Client
	for ctx.Err() == nil {
		select {
		case err := <-done:
			reaped = true
			t.Fatalf("agent exited before ready: %v", err)
		default:
		}
		client, err = agentclient.DialExisting(ctx, addr)
		if err == nil {
			probe, stop := context.WithTimeout(ctx, time.Second)
			health, healthErr := client.Health(probe)
			stop()
			if healthErr == nil {
				if health.Pid != int32(cmd.Process.Pid) || health.Version != version {
					client.Close()
					t.Fatalf("foreign endpoint health: %+v", health)
				}
				if health.Ready {
					break
				}
			}
			client.Close()
			client = nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if client == nil {
		t.Fatalf("readiness deadline: %v", ctx.Err())
	}
	defer client.Close()
	id := fmt.Sprintf("native-%d", time.Now().UnixNano())
	stream, err := client.StreamChat(ctx, id, prompt, home)
	if err != nil {
		t.Fatal(err)
	}
	completed := false
	for msg := range stream {
		if msg.Type == agentclient.TypeError {
			t.Fatalf("chat: %v", msg.Err)
		}
		if msg.Type == agentclient.TypePermissionRequired || msg.Type == agentclient.TypeToolExecStart {
			t.Fatal("fixture must not execute tools")
		}
		if msg.Type == agentclient.TypeDone {
			completed = strings.Contains(msg.Final, answer)
		}
	}
	if !completed || chats.Load() == 0 {
		t.Fatal("no completed real local provider interaction")
	}
	turns, err := client.ResumeConversation(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	userSaved, assistantSaved := false, false
	for _, turn := range turns {
		userSaved = userSaved || (turn.Role == "user" && strings.Contains(turn.Content, prompt))
		assistantSaved = assistantSaved || (turn.Role == "assistant" && strings.Contains(turn.Content, answer))
	}
	if !userSaved || !assistantSaved {
		t.Fatalf("interaction not persisted: %+v", turns)
	}
	t.Run("cli-pty", func(t *testing.T) {
		nativeCLIInteraction(t, client, cmd.Process.Pid, version, cliImage, cliArgs, env, home, denyProfile, port)
	})
	stop, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = client.ShutdownAgent(stop, "bounded native harness complete")
	stopCancel()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		reaped = true
		if err != nil {
			t.Fatalf("graceful exit: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("graceful shutdown deadline exceeded")
	}
	t.Logf("normal startup PID=%d version=%s; local RPC chat persisted; graceful child exit/reap", cmd.Process.Pid, version)
}

// The CLI has no endpoint/no-auto-launch flags: it reads cfg.Port from the
// shared config. Verify the owned endpoint immediately before and after use;
// deny every agent executable by basename in the CLI sandbox, so a failed
// connection cannot turn this slice into an auto-launch test.
func nativeCLIInteraction(t *testing.T, client *agentclient.Client, pid int, version, image string, args, env []string, home, denyProfile, port string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	verify := func() {
		probe, stop := context.WithTimeout(ctx, 3*time.Second)
		defer stop()
		h, err := client.Health(probe)
		if err != nil || h.Pid != int32(pid) || h.Version != version || !h.Ready {
			t.Fatalf("CLI endpoint ownership: %+v %v", h, err)
		}
	}
	verify()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatalf("normal opt-in requires Python 3 stdlib PTY: %v", err)
	}
	prompt := fmt.Sprintf("Inert native PTY check %d; do not use tools.", time.Now().UnixNano())
	// No network-bind allowance; only the verified agent can be reached.
	profile := denyProfile + fmt.Sprintf(`(allow network-outbound (remote ip "localhost:%s"))(deny process-exec (regex #"/cercano$"))`, port)
	const helper = `
import os, pty, fcntl, termios, struct, select, subprocess, sys, time, re
prompt = sys.argv[1]
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 45, 160, 0, 0))
p = None
output = bytearray()
try:
    p = subprocess.Popen(sys.argv[2:], stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    start = time.monotonic()
    sent = False
    quit_at = None
    while time.monotonic() - start < 55:
        if select.select([master], [], [], 0.1)[0]:
            try: chunk = os.read(master, 65536)
            except OSError: chunk = b''
            output.extend(chunk)
            # Answer terminal capability queries without a terminal emulator.
            if b'\x1b[6n' in chunk: os.write(master, b'\x1b[1;1R')
            if b'\x1b]11;?' in chunk: os.write(master, b'\x1b]11;rgb:0000/0000/0000\x1b\\')
        plain = re.sub(rb'\x1b\[[0-?]*[ -/]*[@-~]', b'', bytes(output))
        if not sent and time.monotonic() - start > 3:
            os.write(master, prompt.encode() + b'\r')
            sent = True
        if sent and quit_at is None and b'native fixture response' in plain:
            # Allow the completed stream to settle before voluntary slash exit.
            quit_at = time.monotonic() + 1
        if quit_at is not None and time.monotonic() >= quit_at:
            os.write(master, b'/quit\r')
            quit_at = float('inf')
        if p.poll() is not None:
            assert p.returncode == 0, ('CLI exit', p.returncode)
            assert quit_at is not None, 'CLI exited without rendering fixture answer'
            assert b'auto-launched' not in plain, 'unexpected auto-launch'
            print('PTY voluntarily exited and reaped PID=%d' % p.pid)
            break
    else:
        raise RuntimeError('PTY interaction deadline exceeded')
finally:
    if p is not None and p.poll() is None:
        p.kill()
        p.wait(timeout=5)
    os.close(master)
    sys.stdout.buffer.write(output)
`
	argv := []string{"-c", helper, prompt, "/usr/bin/sandbox-exec", "-p", profile, image}
	argv = append(argv, args...)
	cmd := exec.CommandContext(ctx, python, argv...)
	cmd.Env, cmd.Dir = append(append([]string{}, env...), "TERM=xterm-256color"), home
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PTY prerequisite/interaction failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "cercano: connecting to localhost:"+port) || !strings.Contains(string(output), "PTY voluntarily exited and reaped PID=") {
		t.Fatalf("missing terminal connection/exit evidence: %s", output)
	}
	verify()
	conversations, err := client.ListConversations(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range conversations {
		turns, err := client.ResumeConversation(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		user, assistant := false, false
		for _, turn := range turns {
			user = user || (turn.Role == "user" && strings.Contains(turn.Content, prompt))
			assistant = assistant || (turn.Role == "assistant" && strings.Contains(turn.Content, "native fixture response"))
		}
		if user && assistant {
			t.Logf("CLI image=%s agent PID=%d version=%s conversation=%s: PTY answer/connection notice, parent RPC stored prompt/answer, voluntary /quit and exact reap", image, pid, version, c.ID)
			return
		}
	}
	t.Fatal("PTY prompt and response not found in parent RPC persisted history")
}
