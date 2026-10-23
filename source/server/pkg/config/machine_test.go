package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/pkg/paths"
)

func machineTestHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("CERCANO_PROFILE", "")
}

func TestSaveSplitsRuntimeSettings(t *testing.T) {
	machineTestHome(t)
	cfg := Defaults()
	cfg.LlamaServer.ExtraArgs = []string{"--flash-attn"}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := Save(cfg, path); err != nil {
		t.Fatal(err)
	}
	prof, _ := os.ReadFile(path)
	for _, k := range []string{"llama_server:", "mistralrs:"} {
		if strings.Contains(string(prof), k) {
			t.Errorf("profile file contains %q:\n%s", k, prof)
		}
	}
	mach, err := os.ReadFile(paths.MachineConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"llama_server:", "mistralrs:", "--flash-attn"} {
		if !strings.Contains(string(mach), k) {
			t.Errorf("machine.yaml lacks %q:\n%s", k, mach)
		}
	}
	if fi, _ := os.Stat(paths.MachineConfigPath()); fi.Mode().Perm() != 0600 {
		t.Errorf("machine.yaml mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestProfilesShareRuntimeSettings(t *testing.T) {
	machineTestHome(t)
	pa := filepath.Join(t.TempDir(), "a.yaml")
	pb := filepath.Join(t.TempDir(), "b.yaml")
	a := Defaults()
	a.LlamaServer.ExtraArgs = []string{"--shared"}
	if err := Save(Defaults(), pb); err != nil {
		t.Fatal(err)
	}
	if err := Save(a, pa); err != nil {
		t.Fatal(err)
	}
	b, err := Load(pb)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.LlamaServer.ExtraArgs) != 1 || b.LlamaServer.ExtraArgs[0] != "--shared" {
		t.Errorf("profile b ExtraArgs = %v, want [--shared]", b.LlamaServer.ExtraArgs)
	}
}

func TestLoadWithoutMachineFileUsesDefaults(t *testing.T) {
	machineTestHome(t)
	mc, err := LoadMachine()
	if err != nil {
		t.Fatal(err)
	}
	d := Defaults()
	if mc.LlamaServer.Port != d.LlamaServer.Port || mc.MistralRS.Port != d.MistralRS.Port {
		t.Errorf("LoadMachine without file = %+v, want defaults", mc)
	}
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LlamaServer.Port != d.LlamaServer.Port {
		t.Errorf("Load llama port = %v, want default", cfg.LlamaServer.Port)
	}
}

func TestLoadLegacyProfileSectionsBeforeMigration(t *testing.T) {
	machineTestHome(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("llama_server:\n  extra_args: [\"--legacy\"]\n"), 0644)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.LlamaServer.ExtraArgs) != 1 || cfg.LlamaServer.ExtraArgs[0] != "--legacy" {
		t.Errorf("no machine.yaml: ExtraArgs = %v, want profile's", cfg.LlamaServer.ExtraArgs)
	}
	mc := machineDefaults()
	mc.LlamaServer.ExtraArgs = []string{"--machine"}
	if err := SaveMachine(mc); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.LlamaServer.ExtraArgs) != 1 || cfg.LlamaServer.ExtraArgs[0] != "--machine" {
		t.Errorf("with machine.yaml: ExtraArgs = %v, want machine's", cfg.LlamaServer.ExtraArgs)
	}
}

func TestSaveMachineCreatesPrivateDirAndNoTempLeft(t *testing.T) {
	machineTestHome(t)
	if err := SaveMachine(machineDefaults()); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Dir(paths.MachineConfigPath()))
	if err != nil || fi.Mode().Perm() != 0700 {
		t.Errorf("dir mode = %v err=%v, want 0700", fi.Mode().Perm(), err)
	}
	ents, _ := os.ReadDir(filepath.Dir(paths.MachineConfigPath()))
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

func loadPair(t *testing.T) (a, b Config, pa, pb string) {
	t.Helper()
	machineTestHome(t)
	pa = filepath.Join(t.TempDir(), "a.yaml")
	pb = filepath.Join(t.TempDir(), "b.yaml")
	if err := Save(Defaults(), pa); err != nil {
		t.Fatal(err)
	}
	if err := Save(Defaults(), pb); err != nil {
		t.Fatal(err)
	}
	var err error
	if a, err = Load(pa); err != nil {
		t.Fatal(err)
	}
	if b, err = Load(pb); err != nil {
		t.Fatal(err)
	}
	return
}

func TestStaleSaveDoesNotRevertOtherProfilesMachineSettings(t *testing.T) {
	a, b, pa, pb := loadPair(t)
	a.MistralRS.ISQ = "Q4K"
	if err := Save(a, pa); err != nil {
		t.Fatal(err)
	}
	// b was loaded before a's change and edits an unrelated field.
	b.Port = "50099"
	if err := Save(b, pb); err != nil {
		t.Fatal(err)
	}
	mc, err := LoadMachine()
	if err != nil {
		t.Fatal(err)
	}
	if mc.MistralRS.ISQ != "Q4K" {
		t.Errorf("ISQ = %q after stale save, want Q4K", mc.MistralRS.ISQ)
	}
}

func TestDifferentMachineFieldsFromTwoProfilesBothSurvive(t *testing.T) {
	a, b, pa, pb := loadPair(t)
	a.MistralRS.ISQ = "Q4K"
	b.LlamaServer.ExtraArgs = []string{"--from-b"}
	if err := Save(a, pa); err != nil {
		t.Fatal(err)
	}
	if err := Save(b, pb); err != nil {
		t.Fatal(err)
	}
	// a saves again without changes: must not revert b's field.
	if err := Save(a, pa); err != nil {
		t.Fatal(err)
	}
	mc, _ := LoadMachine()
	if mc.MistralRS.ISQ != "Q4K" || len(mc.LlamaServer.ExtraArgs) != 1 || mc.LlamaServer.ExtraArgs[0] != "--from-b" {
		t.Errorf("machine = ISQ %q extra %v, want Q4K and [--from-b]", mc.MistralRS.ISQ, mc.LlamaServer.ExtraArgs)
	}
}

func TestSaveWithoutChangesDoesNotCreateMachineFile(t *testing.T) {
	machineTestHome(t)
	if err := Save(Defaults(), filepath.Join(t.TempDir(), "c.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.MachineConfigPath()); !os.IsNotExist(err) {
		t.Errorf("machine.yaml exists after an unchanged save: %v", err)
	}
}

func TestLegacyValuesMigrateOnFirstSave(t *testing.T) {
	machineTestHome(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("mistralrs:\n  isq: Q8_0\n"), 0644)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Port = "50077"
	if err := Save(cfg, path); err != nil {
		t.Fatal(err)
	}
	mc, _ := LoadMachine()
	if mc.MistralRS.ISQ != "Q8_0" {
		t.Errorf("legacy ISQ lost: %q", mc.MistralRS.ISQ)
	}
}

func TestSetMachineSectionsResetsBaseline(t *testing.T) {
	a, _, pa, _ := loadPair(t)
	mc := machineDefaults()
	mc.MistralRS.ISQ = "Q2K"
	if err := SaveMachine(mc); err != nil {
		t.Fatal(err)
	}
	a.SetMachineSections(mc)
	if a.MistralRS.ISQ != "Q2K" {
		t.Fatal("sections not applied")
	}
	// Machine moves on; a saves an unrelated change and must not write Q2K back.
	mc.MistralRS.ISQ = "Q4K"
	if err := SaveMachine(mc); err != nil {
		t.Fatal(err)
	}
	a.Port = "50088"
	if err := Save(a, pa); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadMachine()
	if got.MistralRS.ISQ != "Q4K" {
		t.Errorf("ISQ = %q, want Q4K", got.MistralRS.ISQ)
	}
}

func TestLoadUsesMachineFileWhenProfileFileMissing(t *testing.T) {
	machineTestHome(t)
	mc := machineDefaults()
	mc.MistralRS.ISQ = "Q4K"
	if err := SaveMachine(mc); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MistralRS.ISQ != "Q4K" {
		t.Errorf("ISQ = %q, want Q4K", cfg.MistralRS.ISQ)
	}
}

func TestRepeatSaveBySecondProfileKeepsFirstProfilesField(t *testing.T) {
	a, b, pa, pb := loadPair(t)
	a.MistralRS.ISQ = "Q4K"
	if err := Save(a, pa); err != nil {
		t.Fatal(err)
	}
	b.LlamaServer.ExtraArgs = []string{"--from-b"}
	if err := Save(b, pb); err != nil {
		t.Fatal(err)
	}
	b.Port = "50123"
	if err := Save(b, pb); err != nil {
		t.Fatal(err)
	}
	mc, _ := LoadMachine()
	if mc.MistralRS.ISQ != "Q4K" || len(mc.LlamaServer.ExtraArgs) != 1 {
		t.Errorf("machine = ISQ %q extra %v, want Q4K and [--from-b]", mc.MistralRS.ISQ, mc.LlamaServer.ExtraArgs)
	}
}

func TestSnapshotTakenBeforeRefreshDoesNotRevertOtherProfile(t *testing.T) {
	a, b, pa, pb := loadPair(t)
	snap := a.Clone()
	b.MistralRS.ISQ = "Q4K"
	if err := Save(b, pb); err != nil {
		t.Fatal(err)
	}
	mc, _ := LoadMachine()
	a.SetMachineSections(mc) // what the watcher does to the live config
	snap.Port = "50124"      // an in-flight update built on the old snapshot
	if err := Save(snap, pa); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadMachine()
	if got.MistralRS.ISQ != "Q4K" {
		t.Errorf("ISQ = %q, want Q4K", got.MistralRS.ISQ)
	}
}

// A machine.yaml that does not parse must not cost the profile its settings:
// callers treat a Load error as "use Defaults()", and the next Persist would
// then overwrite config.yaml. Load keeps the profile's values, and saving
// leaves the unreadable machine.yaml alone.
func TestMalformedMachineFileKeepsProfileSettings(t *testing.T) {
	machineTestHome(t)
	bad := "llama_server: [unclosed\n"
	if err := os.MkdirAll(filepath.Dir(paths.MachineConfigPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.MachineConfigPath(), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	profile := "locus_mode: cloud_only\nllama_server:\n    port: 8099\n"
	if err := os.WriteFile(path, []byte(profile), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load with a malformed machine.yaml: %v", err)
	}
	if cfg.LocusMode != "cloud_only" {
		t.Errorf("LocusMode = %q, want the profile's cloud_only", cfg.LocusMode)
	}
	if cfg.LlamaServer.Port != 8099 {
		t.Errorf("LlamaServer.Port = %d, want the profile's 8099", cfg.LlamaServer.Port)
	}

	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save without runtime edits: %v", err)
	}
	if got, _ := os.ReadFile(paths.MachineConfigPath()); string(got) != bad {
		t.Errorf("machine.yaml rewritten: %q", got)
	}
	again, err := Load(path)
	if err != nil || again.LocusMode != "cloud_only" {
		t.Errorf("profile settings after save: locus=%q err=%v", again.LocusMode, err)
	}

	cfg.LlamaServer.Port = 8100
	if err := Save(cfg, path); err == nil {
		t.Error("Save of a runtime edit over an unreadable machine.yaml succeeded")
	}
	if got, _ := os.ReadFile(paths.MachineConfigPath()); string(got) != bad {
		t.Errorf("machine.yaml rewritten by a runtime edit: %q", got)
	}
}
