package activationtxn

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"cercano/source/server/internal/updatecoord/candidate"
	"cercano/source/server/internal/updatecoord/operation"
	"cercano/source/server/internal/updatecoord/state"
)

func TestCandidateHealthGate(t *testing.T) {
	op := operation.Record{ID: 1, InstallationID: "install", TargetVersion: "2.0.0", State: operation.StateHealthCheck}
	journal := state.ActivationJournal{SchemaVersion: state.ActivationJournalSchemaVersion, InstallID: op.InstallationID, OpID: op.ID, TargetVersion: op.TargetVersion, StagedVersionDir: "versions/2", VerifiedArtifactSHA256: strings.Repeat("a", 64), Checkpoint: state.JournalSelected}
	endpoint := netip.MustParseAddrPort("127.0.0.1:12345")
	good := candidate.Observation{Status: candidate.Consistent, Version: op.TargetVersion, Ready: true}
	for _, status := range []candidate.ObservationStatus{candidate.Consistent, candidate.Unknown, candidate.Inconsistent, candidate.NotReady} {
		t.Run(string(status), func(t *testing.T) {
			obs := good
			obs.Status = status
			got, ok, err := observeCandidateHealth(context.Background(), endpoint, op, journal, func(_ context.Context, e netip.AddrPort, v string) (candidate.Observation, error) {
				if e != endpoint || v != op.TargetVersion {
					t.Fatal("wrong request")
				}
				return obs, nil
			})
			if err != nil || ok != (status == candidate.Consistent) || got.Status != status {
				t.Fatalf("%+v %v %v", got, ok, err)
			}
		})
	}
	for _, mutate := range []func(*operation.Record, *state.ActivationJournal){
		func(o *operation.Record, j *state.ActivationJournal) { o.ID++ },
		func(o *operation.Record, j *state.ActivationJournal) { o.InstallationID = "other" },
		func(o *operation.Record, j *state.ActivationJournal) { o.TargetVersion = "3" },
		func(o *operation.Record, j *state.ActivationJournal) { o.State = operation.StateRestarting },
		func(o *operation.Record, j *state.ActivationJournal) { o.RecoveryRequested = true },
		func(o *operation.Record, j *state.ActivationJournal) { o.RecoveryNeeded = true },
		func(o *operation.Record, j *state.ActivationJournal) { j.Checkpoint = state.JournalHealthVerified },
	} {
		o, j := op, journal
		mutate(&o, &j)
		_, ok, err := observeCandidateHealth(context.Background(), endpoint, o, j, func(context.Context, netip.AddrPort, string) (candidate.Observation, error) {
			t.Fatal("invalid state reached observer")
			return good, nil
		})
		if ok || !errors.Is(err, ErrHealthState) {
			t.Fatalf("%v %v", ok, err)
		}
	}
	for _, mode := range []string{"error", "cancel", "wrong-version", "not-ready", "reasons"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, ok, _ := observeCandidateHealth(ctx, endpoint, op, journal, func(context.Context, netip.AddrPort, string) (candidate.Observation, error) {
				obs := good
				switch mode {
				case "error":
					return obs, context.DeadlineExceeded
				case "cancel":
					cancel()
				case "wrong-version":
					obs.Version = "3"
				case "not-ready":
					obs.Ready = false
				case "reasons":
					obs.NotReadyReasons = []string{"shutdown_begun"}
				}
				return obs, nil
			})
			if ok {
				t.Fatal("uncertainty accepted")
			}
		})
	}
	_, ok, err := ObserveCandidateHealth(context.Background(), nil, endpoint, op, journal)
	if ok || err == nil {
		t.Fatal("nil candidate accepted")
	}
}
