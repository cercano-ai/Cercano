package server

import (
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/config"
)

func TestInstalledWorkerExecutionMode(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "Cellar", "cercano-agent", "1.0", "bin", "cercano")
	if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	policy, err := worker.NewInstalledKegPolicy(binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"worker", "", "in_process"} {
		t.Run(mode, func(t *testing.T) {
			srv, _ := newServerWithStore(t)
			srv.SetConfigPersistence("", config.Config{ExecutionMode: mode})
			if err := srv.ConfigureInstalledWorker(policy); err != nil {
				t.Fatal(err)
			}
			if mode == "in_process" {
				if srv.workerRunner != nil || !sameRunner(srv.pickTurnRunner(), srv.inProcessRunner) {
					t.Fatal("installed policy changed in-process selection")
				}
			} else if srv.workerRunner == nil || !sameRunner(srv.pickTurnRunner(), srv.workerRunner) {
				t.Fatal("ordinary dispatch must choose installed-policy worker runner")
			}
		})
	}
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	srv, _ := newServerWithStore(t)
	if err := srv.ConfigureInstalledWorker(policy); err == nil || srv.workerRunner != nil {
		t.Fatal("unavailable selected keg must fail before runner installation")
	}
}
