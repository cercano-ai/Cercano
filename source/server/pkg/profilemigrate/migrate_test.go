package profilemigrate

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"

	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

// setHome points HOME at a fresh temp dir so nothing touches the real home,
// and captures the migration's stderr line.
func setHome(t *testing.T) (string, *bytes.Buffer) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CERCANO_PROFILE", "")
	// config.Load applies these; keep the developer's environment (and any
	// real API key) out of the comparisons.
	for _, k := range []string{"CERCANO_PORT", "GEMINI_API_KEY", "OLLAMA_URL", "CERCANO_EMBEDDING_MODEL",
		"CERCANO_LOCAL_MODEL", "CERCANO_LOCAL_RUNTIME", "CERCANO_OPEN_MODEL", "CERCANO_OPEN_RUNTIME"} {
		t.Setenv(k, "")
	}
	var out bytes.Buffer
	prev := logOut
	logOut = &out
	t.Cleanup(func() { logOut = prev })
	return home, &out
}

func noAgent(home string) Options {
	return Options{
		Home:             home,
		LegacyAgentAlive: func(context.Context) (bool, error) { return false, nil },
		StopLegacyAgent:  func(context.Context) error { return errors.New("no agent to stop") },
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func markerPath() string { return filepath.Join(paths.ProfilesRoot(), ".migrated-v1") }

// legacyConfigRootFiles are every file Cercano kept directly in
// ~/.config/cercano before profiles, paired with its place in the default
// profile.
var legacyConfigRootFiles = []string{
	"config.yaml",
	"permissions.yaml",
	"catalog-cache.json",
	"telemetry.db", "telemetry.db-wal", "telemetry.db-shm",
	"conversations.db", "conversations.db-wal", "conversations.db-shm",
	"watchdog-audit.db", "watchdog-audit.db-wal", "watchdog-audit.db-shm",
	"crash.log",
	"mcp.yaml",
	"mcp.json",
	"wizard_state.yaml",
	"failures.jsonl",
	"turn-routing.jsonl",
	"stream-anomalies.jsonl",
	"hook_state.json",
}

type seeded struct {
	old, new, content string
}

func seedLegacy(t *testing.T, home string) []seeded {
	t.Helper()
	root := paths.ConfigRoot()
	def := paths.ProfileDir("default")
	var all []seeded
	for _, name := range legacyConfigRootFiles {
		content := "legacy " + name
		if name == "config.yaml" {
			content = "port: \"50052\"\n"
		}
		all = append(all, seeded{filepath.Join(root, name), filepath.Join(def, name), content})
	}
	all = append(all,
		seeded{filepath.Join(root, "scripts", "fetch.js"), filepath.Join(def, "scripts", "fetch.js"), "script"},
		seeded{filepath.Join(paths.StateRoot(), "enterprise", "managed", "policy.json"), filepath.Join(def, "enterprise", "managed", "policy.json"), "{}"},
		seeded{filepath.Join(paths.StateRoot(), "enterprise", "active-bundle.json"), filepath.Join(def, "enterprise", "active-bundle.json"), "bundle"},
		seeded{filepath.Join(home, ".cercano-dispatch.log"), filepath.Join(def, "logs", "dispatch.log"), "dispatch"},
		seeded{filepath.Join(root, "tui-perf.log"), filepath.Join(def, "logs", "tui-perf.log"), "perf"},
		seeded{filepath.Join(root, "search-perf.jsonl"), filepath.Join(def, "logs", "search-perf.jsonl"), "search"},
		seeded{filepath.Join(root, "colorprobe.log"), filepath.Join(def, "logs", "colorprobe.log"), "probe"},
	)
	for _, s := range all {
		writeFile(t, s.old, s.content)
	}
	return all
}

func TestMigrateMovesLegacyFiles(t *testing.T) {
	home, out := setHome(t)
	all := seedLegacy(t, home)
	// Machine-wide files must stay where they are.
	venv := filepath.Join(paths.ConfigRoot(), "venv", "bin", "python3")
	update := filepath.Join(paths.ConfigRoot(), "update_check.json")
	models := filepath.Join(paths.StateRoot(), "models", "m.gguf")
	for _, p := range []string{venv, update, models} {
		writeFile(t, p, "machine")
	}

	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, s := range all {
		if exists(s.old) {
			t.Errorf("%s still at the legacy path", s.old)
		}
		if got := readFile(t, s.new); got != s.content {
			t.Errorf("%s = %q, want %q", s.new, got, s.content)
		}
	}
	if exists(filepath.Join(paths.StateRoot(), "enterprise")) {
		t.Error("legacy enterprise directory still exists")
	}
	if exists(filepath.Join(paths.ConfigRoot(), "scripts")) {
		t.Error("legacy scripts directory still exists")
	}
	for _, p := range []string{venv, update, models} {
		if got := readFile(t, p); got != "machine" {
			t.Errorf("machine-wide %s changed: %q", p, got)
		}
	}
	if !exists(markerPath()) {
		t.Fatal("marker not written")
	}
	if got := out.String(); got != "Moved existing Cercano data into the default profile\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestMigratePreservesConversations(t *testing.T) {
	home, _ := setHome(t)
	legacy := filepath.Join(paths.ConfigRoot(), "conversations.db")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", legacy)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"PRAGMA journal_mode=WAL",
		"CREATE TABLE conversations (id TEXT PRIMARY KEY, title TEXT)",
		"INSERT INTO conversations VALUES ('c1', 'kept across the move')",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	moved := filepath.Join(paths.ProfileDir("default"), "conversations.db")
	db, err = sql.Open("sqlite", moved)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var title string
	if err := db.QueryRow("SELECT title FROM conversations WHERE id = 'c1'").Scan(&title); err != nil {
		t.Fatalf("query moved db: %v", err)
	}
	if title != "kept across the move" {
		t.Fatalf("title = %q", title)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	home, out := setHome(t)
	all := seedLegacy(t, home)
	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	mtimes := map[string]time.Time{}
	for _, s := range all {
		fi, err := os.Stat(s.new)
		if err != nil {
			t.Fatal(err)
		}
		mtimes[s.new] = fi.ModTime()
	}
	out.Reset()
	probed := false
	opts := noAgent(home)
	opts.LegacyAgentAlive = func(context.Context) (bool, error) { probed = true; return false, nil }
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if probed {
		t.Error("second Run probed for a legacy agent; the marker should short-circuit")
	}
	for _, s := range all {
		fi, err := os.Stat(s.new)
		if err != nil {
			t.Fatal(err)
		}
		if !fi.ModTime().Equal(mtimes[s.new]) {
			t.Errorf("%s modified by second Run", s.new)
		}
		if got := readFile(t, s.new); got != s.content {
			t.Errorf("%s = %q after second Run", s.new, got)
		}
	}
	if out.Len() != 0 {
		t.Errorf("second Run printed %q", out.String())
	}
}

const legacyConfigWithRuntime = `# my settings
ollama_url: http://localhost:11434
port: "50052"
active_cloud_profile: work
cloud_profiles:
    - name: work
      flavor: messages
      route: api
      base_url: https://api.example.com
      model: big-model
locus_mode: cloud_primary
llama_server:
    enabled: true
    binary: /opt/llama-server
    extra_args:
        - --flash-attn
        - "on"
mistralrs:
    enabled: true
    default_model: some/model
compaction:
    enabled: true
`

func TestMigrateSplitsRuntimeSettings(t *testing.T) {
	home, _ := setHome(t)
	legacy := filepath.Join(paths.ConfigRoot(), "config.yaml")
	writeFile(t, legacy, legacyConfigWithRuntime)
	before, err := config.Load(legacy)
	if err != nil {
		t.Fatalf("load legacy config: %v", err)
	}

	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mc, err := config.LoadMachine()
	if err != nil {
		t.Fatalf("LoadMachine: %v", err)
	}
	if got := strings.Join(mc.LlamaServer.ExtraArgs, " "); got != "--flash-attn on" {
		t.Errorf("machine llama_server.extra_args = %q", got)
	}
	if mc.LlamaServer.Binary != "/opt/llama-server" || mc.MistralRS.DefaultModel != "some/model" {
		t.Errorf("machine sections = %+v / %+v", mc.LlamaServer, mc.MistralRS)
	}

	profilePath := filepath.Join(paths.ProfileDir("default"), "config.yaml")
	raw := map[string]any{}
	if err := yaml.Unmarshal([]byte(readFile(t, profilePath)), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"llama_server", "mistralrs"} {
		if _, ok := raw[key]; ok {
			t.Errorf("profile config still has %s", key)
		}
	}
	if !strings.Contains(readFile(t, profilePath), "# my settings") {
		t.Error("profile config lost its comment")
	}
	after, err := config.Load(profilePath)
	if err != nil {
		t.Fatalf("load migrated config: %v", err)
	}
	// The effective settings, runtime sections included, are unchanged.
	wantYAML, _ := yaml.Marshal(before)
	gotYAML, _ := yaml.Marshal(after)
	if string(wantYAML) != string(gotYAML) {
		t.Errorf("effective settings changed by the split")
	}
	if after.ActiveCloudProfile != "work" || after.LocusMode != "cloud_primary" || len(after.CloudProfiles) != 1 {
		t.Errorf("profile settings lost: active=%q locus=%q profiles=%d", after.ActiveCloudProfile, after.LocusMode, len(after.CloudProfiles))
	}
	if strings.Join(after.LlamaServer.ExtraArgs, " ") != "--flash-attn on" {
		t.Errorf("effective llama_server.extra_args = %v", after.LlamaServer.ExtraArgs)
	}
}

// A machine.yaml that exists before the migration ran was not written from
// this home's settings (an earlier test run or build wrote it). It is set
// aside, and the runtime settings come from the legacy config.
func TestMigrateSetsAsideExistingMachineConfig(t *testing.T) {
	home, _ := setHome(t)
	writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), legacyConfigWithRuntime)
	existing := "llama_server:\n    binary: \"\"\n    port: 0\n"
	writeFile(t, paths.MachineConfigPath(), existing)

	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, paths.MachineConfigPath()+".pre-migration"); got != existing {
		t.Errorf("machine.yaml.pre-migration = %q, want the earlier machine.yaml", got)
	}
	mc, err := config.LoadMachine()
	if err != nil {
		t.Fatalf("LoadMachine: %v", err)
	}
	if mc.LlamaServer.Binary != "/opt/llama-server" || mc.MistralRS.DefaultModel != "some/model" {
		t.Errorf("machine sections not split from the legacy config: %+v / %+v", mc.LlamaServer, mc.MistralRS)
	}
	if strings.Contains(readFile(t, filepath.Join(paths.ProfileDir("default"), "config.yaml")), "llama_server") {
		t.Error("profile config still has llama_server")
	}
	if !exists(markerPath()) {
		t.Error("marker not written")
	}
}

// Without runtime sections in the legacy config there is nothing to split,
// so an existing machine.yaml stays.
func TestMigrateKeepsMachineConfigWithoutLegacySections(t *testing.T) {
	home, _ := setHome(t)
	writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), "port: \"50052\"\n")
	existing := "llama_server:\n    binary: /already/here\n"
	writeFile(t, paths.MachineConfigPath(), existing)

	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, paths.MachineConfigPath()); got != existing {
		t.Errorf("machine.yaml changed: %q", got)
	}
	if exists(paths.MachineConfigPath() + ".pre-migration") {
		t.Error("machine.yaml set aside with nothing to split")
	}
}

func TestMigrateLeavesUnparseableConfigAlone(t *testing.T) {
	home, _ := setHome(t)
	bad := "llama_server: [unclosed\n"
	writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), bad)
	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, filepath.Join(paths.ProfileDir("default"), "config.yaml")); got != bad {
		t.Errorf("config rewritten: %q", got)
	}
	if exists(paths.MachineConfigPath()) {
		t.Error("machine.yaml written from an unparseable config")
	}
	if !exists(markerPath()) {
		t.Error("marker not written")
	}
}

func TestMigrateStopsLegacyAgentFirst(t *testing.T) {
	prevWait := stopWait
	stopWait = 300 * time.Millisecond
	t.Cleanup(func() { stopWait = prevWait })

	t.Run("agent stops", func(t *testing.T) {
		home, _ := setHome(t)
		writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), "port: \"50052\"\n")
		alive := true
		stopped := false
		opts := Options{
			Home:             home,
			LegacyAgentAlive: func(context.Context) (bool, error) { return alive, nil },
			StopLegacyAgent: func(context.Context) error {
				if !exists(filepath.Join(paths.ConfigRoot(), "config.yaml")) {
					t.Error("a file moved before the legacy agent was stopped")
				}
				stopped = true
				alive = false
				return nil
			},
		}
		if err := Run(context.Background(), opts); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !stopped {
			t.Fatal("StopLegacyAgent not called")
		}
		if !exists(filepath.Join(paths.ProfileDir("default"), "config.yaml")) || !exists(markerPath()) {
			t.Fatal("migration did not complete after the agent stopped")
		}
	})

	t.Run("agent stays alive", func(t *testing.T) {
		home, _ := setHome(t)
		legacy := filepath.Join(paths.ConfigRoot(), "config.yaml")
		writeFile(t, legacy, "port: \"50052\"\n")
		stopCalls := 0
		opts := Options{
			Home:             home,
			LegacyAgentAlive: func(context.Context) (bool, error) { return true, nil },
			StopLegacyAgent:  func(context.Context) error { stopCalls++; return nil },
		}
		err := Run(context.Background(), opts)
		if err == nil || !strings.Contains(err.Error(), "quit Cercano") {
			t.Fatalf("Run err = %v, want a quit-and-retry error", err)
		}
		if stopCalls != 1 {
			t.Errorf("StopLegacyAgent called %d times", stopCalls)
		}
		if !exists(legacy) || exists(filepath.Join(paths.ProfileDir("default"), "config.yaml")) {
			t.Error("files moved while the legacy agent was alive")
		}
		if exists(markerPath()) {
			t.Error("marker written while the legacy agent was alive")
		}
	})
}

func TestMigrateRetriesAfterPartialFailure(t *testing.T) {
	home, out := setHome(t)
	all := seedLegacy(t, home)
	obstacle := filepath.Join(paths.ProfileDir("default"), "permissions.yaml")
	writeFile(t, filepath.Join(obstacle, "occupied"), "x")

	if err := Run(context.Background(), noAgent(home)); err == nil {
		t.Fatal("Run succeeded despite the obstacle")
	}
	if exists(markerPath()) {
		t.Fatal("marker written after a failed move")
	}
	if !exists(filepath.Join(paths.ConfigRoot(), "permissions.yaml")) {
		t.Fatal("blocked file vanished from the legacy path")
	}

	if err := os.RemoveAll(obstacle); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("retry Run: %v", err)
	}
	for _, s := range all {
		if exists(s.old) {
			t.Errorf("%s still at the legacy path", s.old)
		}
		if got := readFile(t, s.new); got != s.content {
			t.Errorf("%s = %q", s.new, got)
		}
	}
	if !exists(markerPath()) {
		t.Fatal("marker missing after retry")
	}
}

func TestFreshInstallJustWritesMarker(t *testing.T) {
	home, out := setHome(t)
	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !exists(markerPath()) {
		t.Fatal("marker not written")
	}
	entries, err := os.ReadDir(paths.ProfilesRoot())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != ".migrated-v1" {
			t.Errorf("fresh install created %s", e.Name())
		}
	}
	if exists(paths.MachineConfigPath()) {
		t.Error("fresh install wrote machine.yaml")
	}
	if out.Len() != 0 {
		t.Errorf("fresh install printed %q", out.String())
	}
}

// A profiles/default directory left by tests or earlier builds is not a
// migration signal; only the marker is.
func TestExistingDefaultDirDoesNotBlockMigration(t *testing.T) {
	home, _ := setHome(t)
	def := paths.ProfileDir("default")
	writeFile(t, filepath.Join(def, "failures.jsonl"), "stray test event")
	writeFile(t, filepath.Join(def, "conversations.db-wal"), "stray wal")
	writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), "port: \"50099\"\n")
	writeFile(t, filepath.Join(paths.ConfigRoot(), "failures.jsonl"), "real event")
	writeFile(t, filepath.Join(paths.ConfigRoot(), "conversations.db"), "real db")

	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, filepath.Join(def, "config.yaml")); got != "port: \"50099\"\n" {
		t.Errorf("config.yaml = %q", got)
	}
	if got := readFile(t, filepath.Join(def, "failures.jsonl")); got != "real event" {
		t.Errorf("failures.jsonl = %q", got)
	}
	if got := readFile(t, filepath.Join(def, "conversations.db")); got != "real db" {
		t.Errorf("conversations.db = %q", got)
	}
	// A stray WAL must never be paired with the moved database.
	if exists(filepath.Join(def, "conversations.db-wal")) {
		t.Error("stray conversations.db-wal left beside the moved database")
	}
	if got := readFile(t, filepath.Join(def, "failures.jsonl.pre-migration")); got != "stray test event" {
		t.Errorf("stray file not kept aside: %q", got)
	}
	if !exists(markerPath()) {
		t.Error("marker not written")
	}
}

func TestMigrateMovesXDGWizardState(t *testing.T) {
	t.Run("moved", func(t *testing.T) {
		home, _ := setHome(t)
		xdg := filepath.Join(home, "xdg")
		t.Setenv("XDG_CONFIG_HOME", xdg)
		src := filepath.Join(xdg, "cercano", "wizard_state.yaml")
		writeFile(t, src, "step: 3\n")
		if err := Run(context.Background(), noAgent(home)); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := readFile(t, filepath.Join(paths.ProfileDir("default"), "wizard_state.yaml")); got != "step: 3\n" {
			t.Errorf("wizard state = %q", got)
		}
		if exists(src) {
			t.Error("XDG wizard state still at the old path")
		}
	})
	t.Run("destination wins", func(t *testing.T) {
		home, _ := setHome(t)
		xdg := filepath.Join(home, "xdg")
		t.Setenv("XDG_CONFIG_HOME", xdg)
		src := filepath.Join(xdg, "cercano", "wizard_state.yaml")
		writeFile(t, src, "step: 3\n")
		dst := filepath.Join(paths.ProfileDir("default"), "wizard_state.yaml")
		writeFile(t, dst, "step: 9\n")
		if err := Run(context.Background(), noAgent(home)); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := readFile(t, dst); got != "step: 9\n" {
			t.Errorf("wizard state overwritten: %q", got)
		}
		if !exists(src) {
			t.Error("XDG wizard state removed although it was not moved")
		}
	})
}

func TestMigrateCopiesAcrossFilesystems(t *testing.T) {
	home, _ := setHome(t)
	ent := filepath.Join(paths.StateRoot(), "enterprise")
	writeFile(t, filepath.Join(ent, "managed", "policy.json"), "{\"a\":1}")
	writeFile(t, filepath.Join(ent, "connection.lock"), "")
	if err := os.Chmod(filepath.Join(ent, "managed", "policy.json"), 0o640); err != nil {
		t.Fatal(err)
	}
	dispatch := filepath.Join(home, ".cercano-dispatch.log")
	writeFile(t, dispatch, "dispatch")

	prev := rename
	t.Cleanup(func() { rename = prev })
	crossed := 0
	rename = func(src, dst string) error {
		if strings.HasPrefix(src, paths.StateRoot()) || src == dispatch {
			crossed++
			return &os.LinkError{Op: "rename", Old: src, New: dst, Err: errCrossDevice}
		}
		return os.Rename(src, dst)
	}

	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if crossed == 0 {
		t.Fatal("injected rename never consulted")
	}
	def := paths.ProfileDir("default")
	if got := readFile(t, filepath.Join(def, "enterprise", "managed", "policy.json")); got != "{\"a\":1}" {
		t.Errorf("policy.json = %q", got)
	}
	if !exists(filepath.Join(def, "enterprise", "connection.lock")) {
		t.Error("connection.lock not copied")
	}
	if fi, err := os.Stat(filepath.Join(def, "enterprise", "managed", "policy.json")); err == nil && fi.Mode().Perm() != 0o640 && runtime.GOOS != "windows" {
		t.Errorf("policy.json mode = %v, want 0640", fi.Mode().Perm())
	}
	if exists(ent) {
		t.Error("source enterprise dir not removed after copy")
	}
	if got := readFile(t, filepath.Join(def, "logs", "dispatch.log")); got != "dispatch" {
		t.Errorf("dispatch.log = %q", got)
	}
	if exists(dispatch) {
		t.Error("source dispatch log not removed after copy")
	}
}

func TestMigrateSkipsProbeWithoutLegacyData(t *testing.T) {
	home, _ := setHome(t)
	// Not legacy data: machine-wide files and a stray profile directory.
	writeFile(t, filepath.Join(paths.ConfigRoot(), "update_check.json"), "{}")
	writeFile(t, filepath.Join(paths.ProfileDir("default"), "failures.jsonl"), "stray")
	opts := Options{
		Home: home,
		LegacyAgentAlive: func(context.Context) (bool, error) {
			t.Error("probed for a legacy agent on an install with no legacy data")
			return true, nil
		},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !exists(markerPath()) {
		t.Fatal("marker not written")
	}
}

func TestSetAsideReportsErrors(t *testing.T) {
	setHome(t)
	dir := t.TempDir()
	// The set-aside name exceeds the file name limit, so Lstat on it fails
	// with something other than not-exist.
	path := filepath.Join(dir, strings.Repeat("a", 250))
	writeFile(t, path, "x")
	done := make(chan error, 1)
	go func() { done <- setAside(path) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("setAside succeeded with an unusable set-aside name")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("setAside looped instead of returning the error")
	}
}

// writeWALOnlyDB creates a WAL-mode database at dir/name whose last row lives
// only in the -wal file, as after a crash: the files are copied while the
// writer still holds them, so nothing is checkpointed.
func writeWALOnlyDB(t *testing.T, dir, name string) {
	t.Helper()
	work := t.TempDir()
	src := filepath.Join(work, name)
	db, err := sql.Open("sqlite", src)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA wal_autocheckpoint=0",
		"CREATE TABLE conversations (id TEXT PRIMARY KEY, title TEXT)",
		"INSERT INTO conversations VALUES ('c1', 'only in the wal')",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		data, err := os.ReadFile(src + suffix)
		if err != nil {
			t.Fatalf("read %s%s: %v", name, suffix, err)
		}
		if suffix == "-wal" && len(data) == 0 {
			t.Fatal("empty WAL; the row was checkpointed")
		}
		if err := os.WriteFile(filepath.Join(dir, name+suffix), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func queryTitle(t *testing.T, path string) (string, error) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var title string
	err = db.QueryRow("SELECT title FROM conversations WHERE id = 'c1'").Scan(&title)
	return title, err
}

func TestMigrateKeepsUncheckpointedWAL(t *testing.T) {
	home, _ := setHome(t)
	writeWALOnlyDB(t, paths.ConfigRoot(), "conversations.db")
	// Control: the database file alone does not have the row.
	bare := filepath.Join(t.TempDir(), "bare.db")
	data, _ := os.ReadFile(filepath.Join(paths.ConfigRoot(), "conversations.db"))
	if err := os.WriteFile(bare, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := queryTitle(t, bare); err == nil {
		t.Fatal("control: the row is in the main database file, not only the WAL")
	}

	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	title, err := queryTitle(t, filepath.Join(paths.ProfileDir("default"), "conversations.db"))
	if err != nil || title != "only in the wal" {
		t.Fatalf("moved db: title = %q, err = %v", title, err)
	}
}

// A run that moved the database but died before its -wal: the retry moves
// the -wal beside it and sets nothing of the database aside.
func TestMigrateRetryMovesRemainingWAL(t *testing.T) {
	home, _ := setHome(t)
	writeWALOnlyDB(t, paths.ConfigRoot(), "conversations.db")
	def := paths.ProfileDir("default")
	if err := os.MkdirAll(def, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(paths.ConfigRoot(), "conversations.db"), filepath.Join(def, "conversations.db")); err != nil {
		t.Fatal(err)
	}

	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if exists(filepath.Join(paths.ConfigRoot(), "conversations.db"+suffix)) {
			t.Errorf("conversations.db%s left at the legacy path", suffix)
		}
		if exists(filepath.Join(def, "conversations.db"+suffix+".pre-migration")) {
			t.Errorf("conversations.db%s was set aside", suffix)
		}
	}
	title, err := queryTitle(t, filepath.Join(def, "conversations.db"))
	if err != nil || title != "only in the wal" {
		t.Fatalf("title = %q, err = %v", title, err)
	}
}

// seedEnterprise writes a small enterprise tree and makes every rename out of
// ~/.cercano report a cross-device error.
func seedEnterpriseCrossDevice(t *testing.T) (src string, files map[string]string) {
	t.Helper()
	src = filepath.Join(paths.StateRoot(), "enterprise")
	files = map[string]string{
		"active-bundle.json":  "bundle",
		"managed/policy.json": "{}",
		"managed/extra.json":  "extra",
		"connection.lock":     "",
	}
	for rel, content := range files {
		writeFile(t, filepath.Join(src, rel), content)
	}
	prev := rename
	t.Cleanup(func() { rename = prev })
	rename = func(from, to string) error {
		if strings.HasPrefix(from, paths.StateRoot()+string(filepath.Separator)) {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: errCrossDevice}
		}
		return os.Rename(from, to)
	}
	return src, files
}

func assertEnterpriseMoved(t *testing.T, src string, files map[string]string) {
	t.Helper()
	dst := filepath.Join(paths.ProfileDir("default"), "enterprise")
	for rel, content := range files {
		if got := readFile(t, filepath.Join(dst, rel)); got != content {
			t.Errorf("%s = %q, want %q", rel, got, content)
		}
	}
	for _, leftover := range []string{src, src + trashSuffix, dst + tempSuffix, dst + ".pre-migration"} {
		if exists(leftover) {
			t.Errorf("%s left behind", leftover)
		}
	}
	if !exists(markerPath()) {
		t.Error("marker not written")
	}
}

func TestCrossDeviceMoveRecoversFromEachCrashPoint(t *testing.T) {
	for _, step := range []string{stepCopied, stepSourceTrashed, stepInstalled} {
		t.Run(step, func(t *testing.T) {
			home, _ := setHome(t)
			src, files := seedEnterpriseCrossDevice(t)
			prev := afterStep
			t.Cleanup(func() { afterStep = prev })
			afterStep = func(s string) error {
				if s == step {
					return errors.New("simulated crash at " + s)
				}
				return nil
			}
			if err := Run(context.Background(), noAgent(home)); err == nil {
				t.Fatal("Run succeeded through a simulated crash")
			}
			if exists(markerPath()) {
				t.Fatal("marker written after a crash")
			}
			afterStep = prev
			if err := Run(context.Background(), noAgent(home)); err != nil {
				t.Fatalf("retry Run: %v", err)
			}
			assertEnterpriseMoved(t, src, files)
		})
	}
}

func TestCrossDeviceMoveRecoversFromPartialSourceRemoval(t *testing.T) {
	home, _ := setHome(t)
	src, files := seedEnterpriseCrossDevice(t)
	prev := removeAll
	t.Cleanup(func() { removeAll = prev })
	removeAll = func(path string) error {
		if strings.HasSuffix(path, trashSuffix) {
			// Delete part of the trashed source, then fail.
			_ = os.Remove(filepath.Join(path, "managed", "policy.json"))
			return errors.New("simulated crash while removing the source")
		}
		return os.RemoveAll(path)
	}
	if err := Run(context.Background(), noAgent(home)); err == nil {
		t.Fatal("Run succeeded although removing the source failed")
	}
	if exists(markerPath()) {
		t.Fatal("marker written before the source was removed")
	}
	removeAll = prev
	if err := Run(context.Background(), noAgent(home)); err != nil {
		t.Fatalf("retry Run: %v", err)
	}
	// The complete copy survives; the half-removed source never replaces it.
	assertEnterpriseMoved(t, src, files)
}
