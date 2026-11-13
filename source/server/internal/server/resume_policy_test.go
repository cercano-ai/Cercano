package server

import (
	"cercano/source/server/internal/failurelog"
	"cercano/source/server/internal/recoveryguard"
	"context"
	"database/sql"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// All failures have a qualified fixture adapter available: refusal must not be
// an accidental consequence of containment discovery failing later.
func TestResumeFallbackPolicy(t *testing.T) {
	for _, mode := range []string{"auth", "acquire-auth", "identity", "cancel", "deadline", "malformed", "publication", "resolver-error", "missing", "unreachable", "dial-auth", "source-auth", "source-loss", "receipt-lookup", "guard-error", "guard-live", "guard-identity", "guard-inspection", "legacy-blocked"} {
		t.Run(mode, func(t *testing.T) {
			a, b, _ := lifecycleFixture(t)
			serveCutoverFixture(t, a)
			_, destination := serveCutoverFixture(t, b)
			source := &resumeSource{Server: a, unauth: mode == "source-auth", lost: mode == "source-loss", legacy: mode == "legacy-blocked", blocked: mode == "legacy-blocked"}
			endpoint := serveResumeSource(t, source)
			ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}})
			if _, err := a.Retain(ctx, "chat"); err != nil {
				t.Fatal(err)
			}
			l := b.coexistence
			l.options.ControlAuthorize = func(context.Context) error { return nil }
			p := &containedFixture{}
			l.options.RecoverySource = p
			logPath := filepath.Join(t.TempDir(), "failures.jsonl")
			w, e := failurelog.NewWriter(logPath)
			if e != nil {
				t.Fatal(e)
			}
			defer w.Close()
			b.failureLog = w
			discoveries := 0
			l.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
				discoveries++
				return &pb.LifecycleRecoverySelection{Identity: l.controlIdentity(), Endpoint: destination, TokenFile: "secret-token"}, nil
			}
			setResumeSource(b, a, endpoint)
			r := resumeRequest(b, destination)
			r.Requester = l.controlIdentity()
			switch mode {
			case "acquire-auth":
				calls := 0
				l.options.ControlAuthorize = func(context.Context) error {
					calls++
					if calls == 2 {
						return status.Error(codes.Unauthenticated, "secret-token")
					}
					return nil
				}
				r.DestinationEndpoint = ""
			case "source-auth":
				l.options.ControlDial = func(context.Context, string, *pb.LifecycleIdentity) (*grpc.ClientConn, error) {
					return nil, status.Error(codes.Unauthenticated, "secret-token")
				}
			case "auth":
				l.options.ControlAuthorize = func(context.Context) error { return status.Error(codes.Unauthenticated, "secret-token") }
			case "identity":
				l.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
					s := a.coexistence.controlIdentity()
					s.Build = ""
					return s, endpoint, nil
				}
			case "cancel":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			case "deadline":
				l.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
					return nil, "", context.DeadlineExceeded
				}
			case "malformed":
				path := resumeJournal(l, "chat")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("{secret-receipt"), 0600); err != nil {
					t.Fatal(err)
				}
			case "publication":
				l.resumeJournalSync = func(*os.File) error { return errors.New("uncertain publication secret-receipt") }
			case "resolver-error":
				l.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
					return nil, "", errors.New("metadata unreadable")
				}
			case "missing":
				calls := 0
				l.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
					calls++
					if calls == 1 {
						return nil, "", os.ErrNotExist
					}
					return a.coexistence.controlIdentity(), endpoint, nil
				}
			case "guard-live":
				l.options.RecoveryGuard = func(context.Context, string) (func(), error) { return nil, recoveryguard.ErrLiveSource }
				r.DestinationEndpoint = ""
			case "guard-identity", "guard-inspection":
				l.options.RecoveryGuard = func(context.Context, string) (func(), error) { return nil, errors.New(mode + " secret-token") }
				r.DestinationEndpoint = ""
			case "guard-error":
				l.options.RecoveryGuard = func(context.Context, string) (func(), error) { return nil, errors.New("guard metadata unreadable") }
				r.DestinationEndpoint = ""
			case "receipt-lookup":
				db, err := sql.Open("sqlite", l.options.StorePath)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				if _, err := db.Exec("DROP TABLE conversation_handovers"); err != nil {
					t.Fatal(err)
				}
				l.options.ControlDial = func(context.Context, string, *pb.LifecycleIdentity) (*grpc.ClientConn, error) {
					return nil, status.Error(codes.Unavailable, "unavailable")
				}
			case "unreachable", "dial-auth":
				l.options.ControlDial = func(context.Context, string, *pb.LifecycleIdentity) (*grpc.ClientConn, error) {
					code := codes.Unavailable
					if mode == "dial-auth" {
						code = codes.Unauthenticated
					}
					return nil, status.Error(code, "secret-token")
				}
			}
			before, err := conversation.ReadOwnership(context.Background(), l.store, "chat")
			if err != nil {
				t.Fatal(err)
			}
			got, err := b.LifecycleResume(ctx, r)
			eligible := mode == "deadline" || mode == "missing" || mode == "unreachable" || mode == "guard-live"
			if eligible {
				if err != nil || !got.GetConfirmationRequired() || discoveries != 1 {
					t.Fatalf("eligible discovery: got=%v err=%v discoveries=%d", got, err, discoveries)
				}
			} else if err == nil || got != nil || discoveries != 0 {
				t.Errorf("refusal replaced: ready=%t confirmation=%t code=%s discoveries=%d", got.GetReady(), got.GetConfirmationRequired(), status.Code(err), discoveries)
			}
			data, logErr := os.ReadFile(logPath)
			if logErr != nil {
				t.Fatal(logErr)
			}
			if !strings.Contains(string(data), `"operation":"resume"`) || !strings.Contains(string(data), `"stage":`) || !strings.Contains(string(data), `"error_kind":`) {
				t.Fatal("original diagnostic missing")
			}
			for _, secret := range []string{"secret-token", "secret-receipt", endpoint, l.options.StorePath} {
				if strings.Contains(string(data), secret) {
					t.Fatal("diagnostic leaked private material")
				}
			}
			after, e := conversation.ReadOwnership(context.Background(), l.store, "chat")
			if e != nil {
				t.Fatal(e)
			}
			// Response loss can legitimately leave the source's exact reservation.
			if mode != "source-loss" && after != before {
				t.Fatalf("ownership changed: %+v -> %+v", before, after)
			}
			if p.stop != 0 || source.shutdowns != 0 {
				t.Fatal("ordinary failure authorized stop")
			}
		})
	}
}

func TestResumeReadyJournalFailure(t *testing.T) {
	for _, mode := range []string{"malformed", "lookup"} {
		t.Run(mode, func(t *testing.T) {
			_, b, _ := lifecycleFixture(t)
			ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}})
			l := b.coexistence
			l.options.ControlAuthorize = func(context.Context) error { return nil }
			if _, err := b.Retain(ctx, "chat"); err != nil {
				t.Fatal(err)
			}
			path := resumeJournal(l, "chat")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if mode == "malformed" {
				if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				op := &pb.LifecycleControlRequest{Destination: l.controlIdentity(), HandoverId: "historic", Owner: &pb.LifecycleOwner{Conversation: "chat"}}
				if err := saveResume(path, op); err != nil {
					t.Fatal(err)
				}
				db, err := sql.Open("sqlite", l.options.StorePath)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				if _, err := db.Exec("DROP TABLE conversation_handovers"); err != nil {
					t.Fatal(err)
				}
			}
			got, err := b.LifecycleResume(ctx, resumeRequest(b, ""))
			if err == nil || got != nil {
				t.Fatalf("journal failure reported ready: %v %v", got, err)
			}
		})
	}
}

// Deliberately desynchronize only disposable admission accounting to exercise
// cleanup uncertainty after guarded recovery; it must never become a stop offer.
func TestResumeAcceptanceCleanupFailure(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}})
	old, err := a.Retain(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	l := b.coexistence
	l.options.ControlAuthorize = func(context.Context) error { return nil }
	acceptance, err := b.AcceptIncoming(old, "cleanup-op", a.coexistence.controlIdentity().Build, l.options.Contract)
	if err != nil {
		t.Fatal(err)
	}
	op := &pb.LifecycleControlRequest{Source: a.coexistence.controlIdentity(), Destination: l.controlIdentity(), Owner: wireOwner(old), HandoverId: "cleanup-op", Receipt: acceptance.receipt}
	if err = saveResume(resumeJournal(l, "chat"), op); err != nil {
		t.Fatal(err)
	}
	if err = l.admission.ResolveIncomingDurably(acceptance.h); err != nil {
		t.Fatal(err)
	}
	if err = a.coexistence.owners["chat"].Close(); err != nil {
		t.Fatal(err)
	}
	l.options.RecoveryGuard = func(context.Context, string) (func(), error) { return func() {}, nil }
	stop := &containedFixture{}
	l.options.RecoverySource = stop
	discoveries := 0
	l.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
		discoveries++
		return nil, errors.New("unexpected discovery")
	}
	got, err := b.LifecycleResume(ctx, resumeRequest(b, ""))
	if err == nil || got != nil || discoveries != 0 || stop.stop != 0 {
		t.Fatal("cleanup uncertainty replaced")
	}
	if l.incoming[acceptance.h] != acceptance {
		t.Fatal("uncertain participation discarded")
	}
}
