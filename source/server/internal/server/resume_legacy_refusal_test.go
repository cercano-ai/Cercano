package server

import (
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"os"
	"strings"
	"testing"
)

func TestResumeLegacyConfirmationRequiresUpgrade(t *testing.T) {
	for _, mode := range []string{"owned", "unowned", "local", "reserved", "contained", "mixed-confirmation"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			a, b, _ := lifecycleFixture(t)
			bc, endpoint := serveCutoverFixture(t, b)
			owner, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			cap := a.coexistence.owners["chat"]
			switch mode {
			case "unowned":
				err = cap.Release(ctx)
			case "local":
				err = cap.Release(ctx)
				if err == nil {
					_, err = b.Retain(ctx, "chat")
				}
			case "reserved":
				err = cap.ReserveHandover(ctx, "B", "old-operation")
			}
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			p := &resumeProcessFixture{identity: "old-snapshot", onStop: func() { calls++; cap.Close() }}
			b.coexistence.options.LegacyStop = p
			contained := &containedFixture{}
			if mode == "contained" || mode == "mixed-confirmation" {
				b.coexistence.options.LegacyStop = contained
			}
			b.coexistence.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
				calls++
				return a.coexistence.controlIdentity(), "127.0.0.1:1", nil
			}
			b.coexistence.options.RecoveryGuard = func(context.Context, string) (func(), error) { calls++; return func() {}, nil }
			op := &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Owner: wireOwner(owner), Requester: b.coexistence.controlIdentity(), HandoverId: "old-operation", LegacyProcessIdentity: p.identity, ConfirmLegacyStop: true, DestinationEndpoint: endpoint}
			path := resumeJournal(b.coexistence, "chat")
			if err := saveResume(path, op); err != nil {
				t.Fatal(err)
			}
			journal, _ := os.ReadFile(path)
			before, err := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
			if err != nil {
				t.Fatal(err)
			}
			if mode == "mixed-confirmation" {
				op.ConfirmOriginalRecovery = true
				op.RecoveryChallenge = "modern"
			}
			got, err := bc.LifecycleResume(ctx, op)
			after, readErr := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
			current, _ := os.ReadFile(path)
			if status.Code(err) != codes.FailedPrecondition || !strings.Contains(strings.ToLower(status.Convert(err).Message()), "upgrade") || got != nil {
				t.Errorf("expected actionable upgrade refusal: %v %v", got, err)
			}
			if readErr != nil || before != after || calls != 0 || p.stops != 0 || contained.stop != 0 || string(current) != string(journal) {
				t.Fatalf("legacy request had side effects: before=%+v after=%+v calls=%d stops=%d/%d err=%v", before, after, calls, p.stops, contained.stop, readErr)
			}
		})
	}
}
