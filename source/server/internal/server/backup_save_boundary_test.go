package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

func TestBackupSaveBoundaryRejectsBeforeLiveEffects(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("active_cloud_profile: primary\ncloud_profiles:\n  - name: primary\n  - name: spare\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{cfgSvc: cfgsvc.New(path, c, secrets.NewMemory())}
	// No provider service: reaching rebuild on rejection is itself a failure.
	if err := os.WriteFile(path, []byte("active_cloud_profile: external\n"), 0600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("rejected draft reached provider rebuild: %v", r)
		}
		if got := s.cfgSvc.Get(); len(got.PrimaryBackups()) != 0 {
			t.Errorf("rejected draft changed live backups: %v", got.PrimaryBackups())
		}
	}()
	response, err := s.SetBackupCloudProfile(context.Background(), &proto.SetBackupCloudProfileRequest{Name: "spare"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Ok || response.Error == "" {
		t.Fatalf("expected publication failure: %v", response)
	}
}

func TestRuntimeSaveBoundaryRejectsBeforeLiveEffects(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("port: 54321\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{cfgSvc: cfgsvc.New(path, c, secrets.NewMemory())}
	candidate := s.cfgSvc.Get()
	candidate.LlamaServer.Binary = "/fixture/detected"
	if err := os.WriteFile(path, []byte("port: 54322\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.publishDetectedRuntime(candidate); err == nil {
		t.Fatal("accepted stale detection")
	}
	if got := s.cfgSvc.Get(); got.LlamaServer.Binary != c.LlamaServer.Binary {
		t.Fatal("rejected detection applied live")
	}
}
