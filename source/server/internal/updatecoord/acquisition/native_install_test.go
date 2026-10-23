package acquisition

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/macho"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/updatecoord/archivecheck"
)

// TestNativeTemporaryInstall is opt-in because it builds six real executables.
// It intentionally stops before enrollment/normal startup and publication.
// All executing artifacts are acquired and staged, never the build outputs.
func TestNativeTemporaryInstall(t *testing.T) {
	if os.Getenv("CERCANO_NATIVE_INSTALL_TEST") != "1" {
		t.Skip("set CERCANO_NATIVE_INSTALL_TEST=1 for native build/acquire/stage")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("requires native macOS arm64")
	}
	root := t.TempDir()
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
		if err == nil || !strings.Contains(output, "execution unwired") {
			t.Fatalf("updater must refuse: %v %s", err, output)
		}
	}
	for _, dir := range stagedDirs {
		if _, err := os.Stat(dir); err != nil {
			t.Fatal("old/candidate not retained", err)
		}
	}
	t.Log("staged two complete current-source fixture releases; no compatibility proof, enrollment, normal launch, drain or publication. NEXT: independent protected layout/selection startup, then old-updater launch and bounded handoff.")
}
