package server

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"testing"
)

// No production processes are inspected or signalled by this adapter.
type resumeProcessFixture struct {
	identity     string
	stops        int
	dead         bool
	onStop       func()
	lostResponse bool
	remainAlive  bool
}

func (p *resumeProcessFixture) Prepare(context.Context, *pb.LifecycleIdentity) (string, error) {
	return p.identity, nil
}
func (p *resumeProcessFixture) Stop(ctx context.Context, source *pb.LifecycleIdentity, expected string, validate func() error) error {
	if expected != p.identity {
		return status.Error(codes.FailedPrecondition, "PID/start identity changed")
	}
	if err := validate(); err != nil {
		return err
	}
	p.stops++
	p.dead = !p.remainAlive
	if p.onStop != nil {
		p.onStop()
	}
	if p.lostResponse {
		return status.Error(codes.Unavailable, "stop response lost")
	}
	return nil
}
func TestDestinationResumeConfirmedLegacyStop(t *testing.T) {
	for _, mode := range []string{"accept", "unconfirmed", "stale-process", "stale-epoch", "stale-operation", "stale-source", "stale-requester", "requester", "unsupported", "unsupported-initial", "restart", "lost-stop-response", "still-alive"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			a, b, _ := lifecycleFixture(t)
			serveCutoverFixture(t, a)
			bc, endpoint := serveCutoverFixture(t, b)
			if _, err := a.Retain(ctx, "chat"); err != nil {
				t.Fatal(err)
			}
			setResumeSource(b, a, serveResumeSource(t, &resumeSource{Server: a, legacy: true, blocked: true}))
			process := &resumeProcessFixture{identity: "uid:501/pid:42/start:100/exe:verified/host:A"}
			process.onStop = func() { a.coexistence.owners["chat"].Close() }
			b.coexistence.options.LegacyStop = process
			b.coexistence.options.RecoveryGuard = func(context.Context, string) (func(), error) {
				if !process.dead {
					return nil, status.Error(codes.FailedPrecondition, "alive")
				}
				return func() {}, nil
			}
			if mode == "unsupported-initial" {
				b.coexistence.options.LegacyStop = nil
			}
			initial := resumeRequest(b, endpoint)
			initial.Requester = b.coexistence.controlIdentity()
			offer, err := bc.LifecycleResume(ctx, initial)
			if err != nil {
				t.Fatal(err)
			}
			if !offer.ConfirmationRequired || (offer.LegacyProcessIdentity == "" && mode != "unsupported-initial") || process.stops != 0 {
				t.Fatalf("invalid offer %v", offer)
			}
			r := &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Source: offer.Source, Owner: offer.Owner, HandoverId: offer.OperationId, DestinationEndpoint: endpoint, ConfirmLegacyStop: true, LegacyProcessIdentity: offer.LegacyProcessIdentity, Requester: b.coexistence.controlIdentity()}
			switch mode {
			case "still-alive":
				process.remainAlive = true
			case "lost-stop-response":
				process.lostResponse = true
			case "unconfirmed":
				r.ConfirmLegacyStop = false
			case "stale-process":
				process.identity = "replacement"
			case "stale-epoch":
				r.Owner = &pb.LifecycleOwner{Conversation: "chat", Host: "A", Epoch: "stale"}
			case "stale-operation":
				r.HandoverId = "stale"
			case "stale-source":
				r.Source = &pb.LifecycleIdentity{Host: "A", Build: "changed"}
			case "stale-requester":
				r.Requester = &pb.LifecycleIdentity{Host: "unrelated"}
			case "requester":
				r.Requester = offer.Source
			case "unsupported":
				b.coexistence.options.LegacyStop = nil
			case "restart":
				b, bc, endpoint = restartResumeFixture(t, b)
				r.Target = b.coexistence.controlIdentity()
				r.DestinationEndpoint = endpoint
			}
			got, err := bc.LifecycleResume(ctx, r)
			if mode == "unsupported-initial" && !strings.Contains(status.Convert(err).Message(), "safe legacy process stop unavailable") {
				t.Fatalf("missing actionable unsupported refusal: %v", err)
			}
			if mode == "lost-stop-response" {
				if status.Code(err) != codes.Unavailable || process.stops != 1 {
					t.Fatalf("expected lost stop response: %v stops=%d", err, process.stops)
				}
				// Model destination/UI restart after the stop happened but its
				// response was lost. The durable operation must recover, not stop twice.
				b, bc, endpoint = restartResumeFixture(t, b)
				r.Target = b.coexistence.controlIdentity()
				r.DestinationEndpoint = endpoint
				got, err = bc.LifecycleResume(ctx, r)
			}
			if mode == "still-alive" {
				if status.Code(err) != codes.FailedPrecondition || process.stops != 1 || got.GetReady() {
					t.Fatalf("recovered without verified death: %v %v stops=%d", got, err, process.stops)
				}
				return
			}
			accepted := mode == "accept" || mode == "restart" || mode == "lost-stop-response"
			if accepted {
				if err != nil || !got.GetReady() || got.GetOwner().GetHost() != "B" || process.stops != 1 {
					t.Fatalf("continuation %v %v stops=%d", got, err, process.stops)
				}
				retry, err := bc.LifecycleResume(ctx, r)
				if err != nil || !retry.GetReady() || process.stops != 1 {
					t.Fatalf("lost response retry %v %v stops=%d", retry, err, process.stops)
				}
			} else if process.stops != 0 {
				t.Fatalf("unsafe stop in %s", mode)
			} else if mode != "unconfirmed" && err == nil {
				t.Fatalf("expected refusal: %v", got)
			}
		})
	}
}

func TestLegacyOfferRefreshesStaleProcess(t *testing.T) {
	ctx := context.Background()
	a, b, _ := lifecycleFixture(t)
	serveCutoverFixture(t, a)
	bc, endpoint := serveCutoverFixture(t, b)
	if _, err := a.Retain(ctx, "chat"); err != nil {
		t.Fatal(err)
	}
	setResumeSource(b, a, serveResumeSource(t, &resumeSource{Server: a, legacy: true, blocked: true}))
	p := &resumeProcessFixture{identity: "first"}
	b.coexistence.options.LegacyStop = p
	r := resumeRequest(b, endpoint)
	r.Requester = b.coexistence.controlIdentity()
	first, err := bc.LifecycleResume(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	p.identity = "replacement"
	second, err := bc.LifecycleResume(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if second.LegacyProcessIdentity != "replacement" || first.LegacyProcessIdentity == second.LegacyProcessIdentity {
		t.Fatalf("stale offer: %v", second)
	}
	r.ConfirmLegacyStop = true
	r.Source = first.Source
	r.Owner = first.Owner
	r.HandoverId = first.OperationId
	r.LegacyProcessIdentity = first.LegacyProcessIdentity
	if _, err = bc.LifecycleResume(ctx, r); err == nil || p.stops != 0 {
		t.Fatalf("old approval accepted: %v", err)
	}
}
