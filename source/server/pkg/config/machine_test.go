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
