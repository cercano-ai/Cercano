package main

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Real production wiring and authenticated terminal release; all inference and
// external services use the ordinary-generation synthetic fixture.
func TestProductionResumeOwnershipAfterClose(t *testing.T) {
	for _, name := range []string{"intentional-release", "transport-only", "competing-terminal"} {
		release := name != "transport-only"
		t.Run(name, func(t *testing.T) {
			startOrdinaryGenerationObserved(t, func(s *server.Server, o developmentHostOptions, endpoint string, stop func()) {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				token := filepath.Join(filepath.Dir(o.State), "token")
				auth, err := devhostauth.Load(o.Host, token)
				if err != nil {
					t.Fatal(err)
				}
				opts := append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, auth.DialOptions()...)
				conn, err := grpc.NewClient(endpoint, opts...)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				controls := proto.NewAgentClient(conn)
				acquired, err := controls.LifecycleAcquire(ctx, &proto.LifecycleControlRequest{Target: o.Identity, Owner: &proto.LifecycleOwner{Conversation: "resume-probe"}})
				if err != nil {
					t.Fatal(err)
				}
				epoch := acquired.Owner.Epoch
				client, err := agentclient.DialAuthenticatedPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: endpoint, HostInstance: o.Host, Build: o.Identity.Build, Contract: o.Identity.Contract, SchemaVersion: o.Identity.Schema, ConversationID: "resume-probe", OwnershipEpoch: epoch}, token)
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				before, err := conversation.ReadOwnership(ctx, s.GenerationStore(), "resume-probe")
				if err != nil || before.State != "owned" || before.Epoch != epoch {
					t.Fatalf("before: %+v %v", before, err)
				}
				if release {
					// Dial/validation alone is not a terminal attachment.
					if err := client.ReleaseSession(ctx, o.Identity); err == nil {
						t.Fatal("unattached terminal released owner")
					}
					updates, err := client.AttachConversation(ctx, "resume-probe")
					if err != nil {
						t.Fatal(err)
					}
					if name == "competing-terminal" {
						other, err := agentclient.DialAuthenticatedPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: endpoint, HostInstance: o.Host, Build: o.Identity.Build, Contract: o.Identity.Contract, SchemaVersion: o.Identity.Schema, ConversationID: "resume-probe", OwnershipEpoch: epoch}, token)
						if err != nil {
							t.Fatal(err)
						}
						defer other.Close()
						// Ordinary attachment must offer an explicit, structured takeover,
						// not silently enroll a second terminal.
						rejected, err := other.AttachConversation(ctx, "resume-probe")
						target := agentclient.TakeoverTarget(err)
						if status.Code(err) != codes.AlreadyExists || target == "" || rejected != nil {
							t.Fatalf("competing attachment: updates=%v target=%q err=%v", rejected, target, err)
						}
						if err := other.ReleaseSession(ctx, o.Identity); status.Code(err) != codes.FailedPrecondition || !strings.Contains(err.Error(), "requester terminal is not attached") {
							t.Fatalf("unattached competing terminal release refusal: %v", err)
						}
						retained, err := conversation.ReadOwnership(ctx, s.GenerationStore(), "resume-probe")
						if err != nil || !reflect.DeepEqual(retained, before) {
							t.Fatalf("refusal mutated owner: %+v %v", retained, err)
						}
						// This is the explicit confirmation: use exactly the observed
						// target. Success waits for the host's attachment acknowledgment.
						replacement, err := other.TakeoverConversation(ctx, "resume-probe", target)
						if err != nil || replacement == nil {
							t.Fatalf("takeover acknowledgment: %v", err)
						}
						closeStreams := func() {
							client.Close()
							other.Close()
							for _, stream := range []<-chan agentclient.ConversationUpdate{updates, replacement} {
								for stream != nil {
									select {
									case _, open := <-stream:
										if !open {
											stream = nil
										}
									case <-ctx.Done():
										t.Error("attachment stream did not close: ", ctx.Err())
										return
									}
								}
							}
						}
						defer closeStreams()
						retained, err = conversation.ReadOwnership(ctx, s.GenerationStore(), "resume-probe")
						if err != nil || !reflect.DeepEqual(retained, before) {
							t.Fatalf("takeover mutated durable owner or epoch: %+v %v", retained, err)
						}
						if err := client.ReleaseSession(ctx, o.Identity); status.Code(err) != codes.FailedPrecondition || !strings.Contains(err.Error(), "requester terminal is not attached") {
							t.Fatalf("displaced terminal release refusal: %v", err)
						}
						retained, err = conversation.ReadOwnership(ctx, s.GenerationStore(), "resume-probe")
						if err != nil || !reflect.DeepEqual(retained, before) {
							t.Fatalf("displaced release mutated owner: %+v %v", retained, err)
						}
						if err := other.ReleaseSession(ctx, o.Identity); err != nil {
							t.Fatalf("replacement release after takeover acknowledgment: %v", err)
						}
						closeStreams()
					} else if err := client.ReleaseSession(ctx, o.Identity); err != nil {
						t.Fatalf("immediate intentional close after AttachConversation: %v", err)
					}
				}
				client.Close()
				after, err := conversation.ReadOwnership(ctx, s.GenerationStore(), "resume-probe")
				expected := "owned"
				if release {
					expected = "released"
				}
				if err != nil || after.State != expected {
					t.Fatalf("after close: %+v %v", after, err)
				}
				t.Logf("before=%+v after=%+v", before, after)
				stop() // only this disposable production host; no process-wide signal
				next, err := server.NewCoexistenceServer(ctx, server.CoexistenceOptions{StorePath: o.Identity.Store, Host: "next-fixture-host", Build: o.Identity.Build, Contract: conversation.DevelopmentCoexistenceDeclaration(), Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }})
				if err != nil {
					t.Fatal(err)
				}
				defer next.GenerationStore().Close()
				owner, err := next.Retain(ctx, "resume-probe")
				if release {
					if err != nil || owner.Epoch == epoch {
						t.Fatalf("released reacquisition: %+v %v", owner, err)
					}
					if err := next.ReleaseRetained(ctx, owner, nil); err != nil {
						t.Fatal(err)
					}
				} else if err == nil {
					t.Fatal("retained owner acquired without release")
				} else {
					t.Logf("unreleased-owner refusal (in-process lock may remain): %v", err)
				}
			})
		})
	}
}
