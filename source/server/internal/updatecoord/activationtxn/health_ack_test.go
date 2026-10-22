package activationtxn

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/updatecoord/candidate"
	"cercano/source/server/internal/updatecoord/nativeprocess"
	"cercano/source/server/internal/updatecoord/operation"
	"cercano/source/server/internal/updatecoord/state"
)

func healthFixture(t *testing.T) (*fixture, HealthRequest, nativeprocess.Identity) {
	t.Helper()
	f := newFixture(t, false)
	r := f.request()
	if _, err := Switch(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	for _, e := range []operation.Event{operation.EventReady, operation.EventDownload, operation.EventDownloaded, operation.EventVerified, operation.EventDrain, operation.EventDrained, operation.EventInstalled, operation.EventRestarted} {
		if _, err := state.NewAdapter(r.Store).Apply(context.Background(), operation.Input{OperationID: r.OpID, Event: e}); err != nil {
			t.Fatal(err)
		}
	}
	j, _, err := r.Store.LoadActivationJournal(context.Background(), r.OpID)
	if err != nil {
		t.Fatal(err)
	}
	return f, HealthRequest{Request: r}, nativeprocess.Identity{PID: 123, StartTicks: 1, Executable: filepath.Join(r.Images.VersionsDirectory, j.StagedVersionDir, r.Images.AgentMember)}
}

func TestGuardedHealthAcknowledgement(t *testing.T) {
	for _, scenario := range []string{"success", "wrong-candidate", "wrong-selection", "stale-id", "uncertain", "operation-revision", "recovery", "journal-revision", "selection-race", "bytes-race", "lifetime-race", "missing-manifest-policy", "wrong-manifest", "wrong-directory", "closed-guard", "nil-candidate"} {
		t.Run(scenario, func(t *testing.T) {
			_, req, id := healthFixture(t)
			ctx := context.Background()
			originalID := req.OpID
			observed := false
			identityCalls := 0
			identity := func() (nativeprocess.Identity, error) {
				identityCalls++
				if scenario == "lifetime-race" && identityCalls > 1 {
					id.StartTicks++
				}
				return id, nil
			}
			switch scenario {
			case "wrong-candidate":
				id.Executable = filepath.Join(t.TempDir(), "other")
			case "wrong-selection":
				if err := os.Remove(filepath.Join(req.Directory, "selection.json")); err != nil {
					t.Fatal(err)
				}
			case "stale-id":
				req.OpID++
			case "missing-manifest-policy":
				req.Images = nil
			case "wrong-manifest":
				req.Images.AgentMember, req.Images.ClientMember = req.Images.ClientMember, req.Images.AgentMember
			case "wrong-directory":
				req.Directory = t.TempDir()
			case "closed-guard":
				if err := req.Lock.Close(); err != nil {
					t.Fatal(err)
				}
			}
			observe := func(_ context.Context, _ netip.AddrPort, version string) (candidate.Observation, error) {
				observed = true
				switch scenario {
				case "operation-revision", "recovery":
					op, rev, err := req.Store.LoadOperationRecord(ctx, req.OpID)
					if err != nil {
						t.Fatal(err)
					}
					if scenario == "recovery" {
						op.RecoveryRequested = true
					}
					if _, err := req.Store.SaveOperationRecord(ctx, rev, op); err != nil {
						t.Fatal(err)
					}
				case "journal-revision":
					j, rev, err := req.Store.LoadActivationJournal(ctx, req.OpID)
					if err != nil {
						t.Fatal(err)
					}
					j.Checkpoint = state.JournalHealthVerified
					if _, err := req.Store.SaveActivationJournal(ctx, rev, j); err != nil {
						t.Fatal(err)
					}
				case "selection-race":
					if err := os.Remove(filepath.Join(req.Directory, "selection.json")); err != nil {
						t.Fatal(err)
					}
				case "bytes-race":
					if err := os.WriteFile(id.Executable, []byte("tampered"), 0700); err != nil {
						t.Fatal(err)
					}
				}
				status := candidate.Consistent
				if scenario == "uncertain" {
					status = candidate.Unknown
				}
				return candidate.Observation{Status: status, Version: version, Ready: true}, nil
			}
			var result HealthResult
			var err error
			if scenario == "nil-candidate" {
				result, err = AcknowledgeCandidateHealth(ctx, req)
			} else {
				result, err = acknowledgeCandidateHealth(ctx, req, identity, observe)
			}
			if scenario == "success" {
				if err != nil || !result.Acknowledged || result.JournalRevision != 4 {
					t.Fatalf("%+v %v", result, err)
				}
				// A stale candidate never inherits success from the durable checkpoint.
				id.Executable = "stale"
				retry, err := acknowledgeCandidateHealth(ctx, req, identity, observe)
				if !errors.Is(err, ErrHealthAlreadyRecorded) || retry.Acknowledged {
					t.Fatalf("retry %+v %v", retry, err)
				}
			} else {
				if result.Acknowledged || (scenario != "uncertain" && err == nil) {
					t.Fatalf("%+v %v", result, err)
				}
			}
			if (scenario == "wrong-candidate" || scenario == "wrong-selection" || scenario == "stale-id") && observed {
				t.Fatal("observed unrelated candidate")
			}
			j, rev, err := req.Store.LoadActivationJournal(ctx, originalID)
			if err != nil {
				t.Fatal(err)
			}
			wantRev := int64(3)
			wantCheckpoint := state.JournalSelected
			if scenario == "success" || scenario == "journal-revision" {
				wantRev = 4
				wantCheckpoint = state.JournalHealthVerified
			}
			if rev != wantRev || j.Checkpoint != wantCheckpoint {
				t.Fatalf("journal %+v rev %d", j, rev)
			}
			op, _, err := req.Store.LoadOperationRecord(ctx, originalID)
			if err != nil || op.State != operation.StateHealthCheck {
				t.Fatalf("lifecycle changed: %+v %v", op, err)
			}
		})
	}
}
