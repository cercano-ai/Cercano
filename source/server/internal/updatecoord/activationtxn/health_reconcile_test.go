package activationtxn

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/updatecoord/operation"
	"cercano/source/server/internal/updatecoord/state"
)

func TestGuardedHealthReconciliation(t *testing.T) {
	for _, scenario := range []string{"reopen", "selection", "manifest", "bytes", "missing-policy", "stale-id", "wrong-directory", "closed-guard", "selected", "duplicate-selection", "duplicate-manifest"} {
		t.Run(scenario, func(t *testing.T) {
			f, health, id := healthFixture(t)
			req := health.Request
			ctx := context.Background()
			j, jr, err := req.Store.LoadActivationJournal(ctx, req.OpID)
			if err != nil {
				t.Fatal(err)
			}
			before, rev, err := req.Store.LoadOperationRecord(ctx, req.OpID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "selected" {
				jr, err = req.Store.AcknowledgeActivationHealth(ctx, rev, jr, j)
				if err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "reopen" {
				if err := req.Store.Close(); err != nil {
					t.Fatal(err)
				}
				req.Store, err = state.Open(f.root, j.InstallID)
				if err != nil {
					t.Fatal(err)
				}
				defer req.Store.Close()
			}
			if scenario == "duplicate-selection" || scenario == "duplicate-manifest" {
				if _, err := ReconcileHealth(ctx, req); err != nil {
					t.Fatal(err)
				}
				before, rev, err = req.Store.LoadOperationRecord(ctx, req.OpID)
				if err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "selection", "duplicate-selection":
				if err := os.Remove(filepath.Join(req.Directory, "selection.json")); err != nil {
					t.Fatal(err)
				}
			case "manifest", "duplicate-manifest":
				req.Images.Manifest.Members[0].SHA256 = digestChar('f')
			case "bytes":
				if err := os.WriteFile(id.Executable, []byte("tampered"), 0700); err != nil {
					t.Fatal(err)
				}
			case "missing-policy":
				req.Images = nil
			case "stale-id":
				req.OpID++
			case "wrong-directory":
				req.Directory = t.TempDir()
			case "closed-guard":
				if err := req.Lock.Close(); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ReconcileHealth(ctx, req)
			if scenario == "reopen" {
				if err != nil || got.AlreadyReconciled || got.OperationRevision != rev+1 {
					t.Fatalf("%+v %v", got, err)
				}
				for i := 0; i < 2; i++ {
					retry, err := ReconcileHealth(ctx, req)
					if err != nil || !retry.AlreadyReconciled || retry.OperationRevision != got.OperationRevision {
						t.Fatalf("retry %+v %v", retry, err)
					}
				}
			} else if err == nil || got != (state.HealthReconciliation{}) {
				t.Fatalf("refusal %+v %v", got, err)
			}
			after, afterRev, err := req.Store.LoadOperationRecord(ctx, before.ID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "reopen" {
				if after.State != operation.StateCleanup || !after.HealthVerified {
					t.Fatalf("%+v", after)
				}
			} else if after != before || afterRev != rev {
				t.Fatalf("operation changed %+v", after)
			}
			saved, savedRev, err := req.Store.LoadActivationJournal(ctx, before.ID)
			if err != nil || savedRev != jr || (scenario != "selected" && saved.Checkpoint != state.JournalHealthVerified) {
				t.Fatalf("journal %+v %d %v", saved, savedRev, err)
			}
		})
	}
}
