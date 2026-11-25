package agentclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type pinnedFake struct {
	partial            bool
	chatCode           codes.Code
	taskStarted        chan struct{}
	taskRelease        chan struct{}
	taskRequests       atomic.Int32
	completed          bool
	continuation       bool
	normalEOF          bool
	repairFails        bool
	afterFinalCode     codes.Code
	credentialsEnabled bool
	validationCode     codes.Code
	validationStarted  chan struct{}
	validationRelease  chan struct{}
	proto.UnimplementedAgentServer
	state                        atomic.Int32
	identities, checks, requests atomic.Int32
	host                         string
	mu                           sync.Mutex
	terminalIDs                  []string
}

func (f *pinnedFake) GetDevelopmentHostIdentity(context.Context, *proto.GetDevelopmentHostIdentityRequest) (*proto.GetDevelopmentHostIdentityResponse, error) {
	f.identities.Add(1)
	return &proto.GetDevelopmentHostIdentityResponse{HostInstance: f.host, Build: "build", Contract: "contract", SchemaVersion: 2}, nil
}
func (f *pinnedFake) CheckRetainedSession(ctx context.Context, r *proto.CheckRetainedSessionRequest) (*proto.CheckRetainedSessionResponse, error) {
	n := f.checks.Add(1)
	if n > 1 && f.repairFails {
		return nil, status.Error(codes.FailedPrecondition, "repair refused")
	}
	if n > 1 && f.validationStarted != nil {
		close(f.validationStarted)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-f.validationRelease:
		}
	}
	if n > 1 && f.validationCode != codes.OK {
		return nil, status.Error(f.validationCode, "validation refused")
	}
	md, _ := metadata.FromIncomingContext(ctx)
	ids := md.Get(PinnedTerminalMetadata)
	if len(ids) != 1 || ids[0] == "" {
		return nil, errors.New("missing terminal identity")
	}
	f.mu.Lock()
	f.terminalIDs = append(f.terminalIDs, ids[0])
	f.mu.Unlock()
	if r.HostInstance != "A" || r.ConversationId != "conversation" || r.OwnershipEpoch != "epoch" || len(md.Get(PinnedEpochMetadata)) != 1 || md.Get(PinnedEpochMetadata)[0] != "epoch" {
		return nil, errors.New("wrong binding")
	}
	return &proto.CheckRetainedSessionResponse{State: proto.RetainedSessionState(f.state.Load())}, nil
}
func (f *pinnedFake) ResetSetup(context.Context, *proto.ResetSetupRequest) (*proto.ResetSetupResponse, error) {
	f.requests.Add(1)
	return &proto.ResetSetupResponse{}, nil
}
func pinnedHost(t *testing.T, addr string, f *pinnedFake) (*grpc.Server, string) {
	t.Helper()
	l, e := net.Listen("tcp", addr)
	if e != nil {
		t.Fatal(e)
	}
	s := grpc.NewServer()
	proto.RegisterAgentServer(s, f)
	go s.Serve(l)
	t.Cleanup(s.Stop)
	return s, l.Addr().String()
}
func pinAt(addr string) PinnedSessionIntent {
	return PinnedSessionIntent{Endpoint: addr, HostInstance: "A", Build: "build", Contract: "contract", SchemaVersion: 2, ConversationID: "conversation", OwnershipEpoch: "epoch"}
}
func forbidLaunch(t *testing.T) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	old := launchServerProcess
	launchServerProcess = func(string) (string, error) { calls.Add(1); return "", errors.New("launch forbidden") }
	t.Cleanup(func() {
		launchServerProcess = old
		if calls.Load() != 0 {
			t.Errorf("launch called %d times", calls.Load())
		}
	})
	return &calls
}
func TestPinnedNoLaunchRefusals(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	forbidLaunch(t)
	for _, state := range []proto.RetainedSessionState{proto.RetainedSessionState_RETAINED_SESSION_STATE_STALE, proto.RetainedSessionState_RETAINED_SESSION_STATE_RELEASED, proto.RetainedSessionState_RETAINED_SESSION_STATE_RETIRED, proto.RetainedSessionState_RETAINED_SESSION_STATE_CLOSED, proto.RetainedSessionState_RETAINED_SESSION_STATE_UNSPECIFIED} {
		t.Run(state.String(), func(t *testing.T) {
			f := &pinnedFake{host: "A"}
			f.state.Store(int32(state))
			_, addr := pinnedHost(t, "127.0.0.1:0", f)
			_, err := DialPinnedSession(context.Background(), pinAt(addr))
			var refusal *SessionCheckError
			if !errors.As(err, &refusal) || refusal.State != state {
				t.Fatalf("wrong refusal: %v", err)
			}
			f.state.Store(1)
			c, err := DialPinnedSession(context.Background(), pinAt(addr))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			f.state.Store(int32(state))
			if err = c.reconnect(context.Background()); !errors.As(err, &refusal) {
				t.Fatalf("retry: %v", err)
			}
		})
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := DialPinnedSession(ctx, pinAt(addr)); err == nil {
		t.Fatal("unreachable accepted")
	}
	f := &pinnedFake{host: "A"}
	f.state.Store(1)
	s, addr := pinnedHost(t, "127.0.0.1:0", f)
	c, err := DialPinnedSession(context.Background(), pinAt(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s.Stop()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	if err := c.reconnect(ctx2); err == nil {
		t.Fatal("unreachable retry accepted")
	}
}
func TestPinnedTransportAndFreshValidation(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	os.Unsetenv("CERCANO_DISABLE_CREDENTIALS")
	forbidLaunch(t)
	f := &pinnedFake{host: "A"}
	f.state.Store(1)
	s, addr := pinnedHost(t, "127.0.0.1:0", f)
	c, err := DialPinnedSession(context.Background(), pinAt(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.ReconnectPinnedSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.identities.Load() != 2 || f.checks.Load() != 2 {
		t.Fatal("explicit ready reconnect did not revalidate")
	}
	old := c.readConn()
	s.Stop()
	replacement := &pinnedFake{host: "B"}
	replacement.state.Store(1)
	_, _ = pinnedHost(t, addr, replacement)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	err = old.Invoke(ctx, proto.Agent_ResetSetup_FullMethodName, &proto.ResetSetupRequest{}, &proto.ResetSetupResponse{}, grpc.WaitForReady(true))
	if err == nil || replacement.requests.Load() != 0 || replacement.identities.Load() != 0 {
		t.Fatal("unvalidated replacement reached")
	}
	if err := c.ReconnectPinnedSession(context.Background()); !errors.Is(err, ErrPinnedSession) {
		t.Fatalf("replacement: %v", err)
	}
	if replacement.checks.Load() != 0 {
		t.Fatal("session checked on wrong identity")
	}
}

func TestPinnedTerminalIdentityLifetime(t *testing.T) {
	forbidLaunch(t)
	f := &pinnedFake{host: "A"}
	f.state.Store(1)
	_, addr := pinnedHost(t, "127.0.0.1:0", f)
	intent := pinAt(addr)
	c, err := DialPinnedSession(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.ReconnectPinnedSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	d, err := DialPinnedSession(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := f.terminalIDs
	if len(ids) != 3 || ids[0] != ids[1] || ids[0] == ids[2] {
		t.Fatalf("terminal identity lifetime: %v", ids)
	}
}

func (f *pinnedFake) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: !f.credentialsEnabled}, nil
}

func (f *pinnedFake) StreamProcessRequest(_ *proto.ProcessRequestRequest, stream grpc.ServerStreamingServer[proto.StreamProcessResponse]) error {
	f.requests.Add(1)
	if f.partial {
		if err := stream.Send(&proto.StreamProcessResponse{Payload: &proto.StreamProcessResponse_TokenDelta{TokenDelta: &proto.TokenDelta{Content: "partial"}}}); err != nil {
			return err
		}
	}
	if f.chatCode != codes.OK {
		return status.Error(f.chatCode, "refused")
	}
	if f.completed {
		if err := stream.Send(&proto.StreamProcessResponse{Payload: &proto.StreamProcessResponse_FinalResponse{FinalResponse: &proto.ProcessRequestResponse{Output: "done"}}}); err != nil {
			return err
		}
	}
	if f.continuation {
		if err := stream.Send(&proto.StreamProcessResponse{Payload: &proto.StreamProcessResponse_TokenDelta{TokenDelta: &proto.TokenDelta{Content: "continuation"}}}); err != nil {
			return err
		}
	}
	if f.normalEOF {
		if err := stream.Send(&proto.StreamProcessResponse{Payload: &proto.StreamProcessResponse_FinalResponse{FinalResponse: &proto.ProcessRequestResponse{Output: "second final"}}}); err != nil {
			return err
		}
		return nil
	}
	if f.afterFinalCode != codes.OK {
		return status.Error(f.afterFinalCode, "after final")
	}
	return status.Error(codes.Unavailable, "transport lost after submission")
}
func TestPinnedChatAutomaticallyRevalidatesWithoutResend(t *testing.T) {
	f := &pinnedFake{host: "A"}
	f.state.Store(1)
	_, addr := pinnedHost(t, "127.0.0.1:0", f)
	c, err := DialPinnedSession(context.Background(), pinAt(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	out, err := c.StreamChatWithRecovery(context.Background(), "conversation", "once", "")
	if err != nil {
		t.Fatal(err)
	}
	for range out {
	}
	if f.checks.Load() != 2 {
		t.Fatalf("fresh validation count = %d, want 2", f.checks.Load())
	}
	if f.requests.Load() != 1 {
		t.Fatalf("request replayed: %d", f.requests.Load())
	}
}

func (f *pinnedFake) AttachConversation(_ *proto.AttachConversationRequest, stream grpc.ServerStreamingServer[proto.StreamProcessResponse]) error {
	if err := stream.SendHeader(metadata.Pairs("cercano-terminal-attached", "true")); err != nil {
		return err
	}
	if err := stream.Send(&proto.StreamProcessResponse{TurnEpoch: "turn", TurnGeneration: 1, EventSequence: 1}); err != nil {
		return err
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}

func TestPinnedAttachmentTransportDrop(t *testing.T) {
	for _, host := range []string{"A", "B", "stale", "credentials"} {
		t.Run(host, func(t *testing.T) {
			if host == "credentials" {
				t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
			}
			f := &pinnedFake{host: "A"}
			f.state.Store(1)
			s, addr := pinnedHost(t, "127.0.0.1:0", f)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c, err := DialPinnedSession(ctx, pinAt(addr))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			updates, err := c.AttachConversation(ctx, "conversation")
			if err != nil {
				t.Fatal(err)
			}
			<-updates
			s.Stop()
			replacement := &pinnedFake{host: host}
			replacement.state.Store(1)
			if host == "stale" {
				replacement.host = "A"
				replacement.state.Store(int32(proto.RetainedSessionState_RETAINED_SESSION_STATE_STALE))
			}
			if host == "credentials" {
				replacement.host = "A"
				replacement.credentialsEnabled = true
			}
			pinnedHost(t, addr, replacement)
			if host != "A" {
				select {
				case u := <-updates:
					if !errors.Is(u.Err, ErrPinnedSession) && !(host == "credentials" && errors.Is(u.Err, ErrCredentialIsolation)) {
						t.Fatalf("refusal: %v", u.Err)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				return
			}
			for replacement.checks.Load() == 0 || c.State() != ConnStateConnected {
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(10 * time.Millisecond):
				}
			}
			if _, err := c.readAgent().ResetSetup(ctx, &proto.ResetSetupRequest{}); err != nil {
				t.Fatalf("subsequent safe RPC: %v", err)
			}
			if c.readConn().GetState() != connectivity.Ready {
				t.Fatal("not ready")
			}
		})
	}
}

func TestPinnedConcurrentRecoveryAndCancellation(t *testing.T) {
	f := &pinnedFake{host: "A"}
	f.state.Store(1)
	_, addr := pinnedHost(t, "127.0.0.1:0", f)
	c, err := DialPinnedSession(context.Background(), pinAt(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	old := c.readConn()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.reconnectPinnedTransport(context.Background(), old); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if f.checks.Load() != 2 {
		t.Fatalf("duplicate recovery: %d", f.checks.Load())
	}
	c.pinnedGate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.ReconnectPinnedSession(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	<-c.pinnedGate
	c.Close()
	if err := c.ReconnectPinnedSession(context.Background()); !errors.Is(err, errClientClosed) {
		t.Fatalf("close: %v", err)
	}
}

func TestPinnedCloseDuringValidationCannotInstall(t *testing.T) {
	f := &pinnedFake{host: "A", validationStarted: make(chan struct{}), validationRelease: make(chan struct{})}
	f.state.Store(1)
	_, addr := pinnedHost(t, "127.0.0.1:0", f)
	c, err := DialPinnedSession(context.Background(), pinAt(addr))
	if err != nil {
		t.Fatal(err)
	}
	old := c.readConn()
	done := make(chan error, 1)
	go func() { done <- c.reconnectPinnedTransport(context.Background(), old) }()
	select {
	case <-f.validationStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("validation not started")
	}
	c.Close()
	close(f.validationRelease)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("installed after close")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not cancel recovery")
	}
	if c.readConn() != old || c.readConn().GetState() != connectivity.Shutdown {
		t.Fatal("closed client replaced")
	}
}

func TestPinnedIntermediateFinalStreamOutcome(t *testing.T) {
	for _, continuation := range []bool{false, true} {
		for _, repairFails := range []bool{false, true} {
			for _, code := range []codes.Code{codes.Unavailable, codes.OK, codes.Unauthenticated, codes.PermissionDenied} {
				t.Run(fmt.Sprintf("continuation=%v/repairFails=%v/%s", continuation, repairFails, code), func(t *testing.T) {
					f := &pinnedFake{host: "A", completed: true, continuation: continuation, repairFails: repairFails, normalEOF: code == codes.OK}
					if code != codes.Unavailable {
						f.afterFinalCode = code
					}
					f.state.Store(1)
					_, addr := pinnedHost(t, "127.0.0.1:0", f)
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					c, err := DialPinnedSession(ctx, pinAt(addr))
					if err != nil {
						t.Fatal(err)
					}
					defer c.Close()
					out, err := c.StreamChatWithRecovery(ctx, "conversation", "once", "")
					if err != nil {
						t.Fatal(err)
					}
					finals, tokens := 0, 0
					var streamErr error
					for msg := range out {
						if msg.Type == TypeDone {
							finals++
						}
						if msg.Type == TypeToken {
							tokens++
						}
						if msg.Err != nil {
							streamErr = msg.Err
						}
					}
					wantFinals := 1
					if code == codes.OK {
						wantFinals = 2
					}
					wantTokens := 0
					if continuation {
						wantTokens = 1
					}
					if finals != wantFinals || tokens != wantTokens {
						t.Fatalf("finals=%d tokens=%d", finals, tokens)
					}
					if code == codes.OK {
						if streamErr != nil {
							t.Fatal(streamErr)
						}
					} else {
						if status.Code(streamErr) != code {
							t.Errorf("original status lost: got %v want %s", streamErr, code)
						}
						if code == codes.Unavailable && (streamErr == nil || !strings.Contains(streamErr.Error(), "remaining request outcome unknown; not resent")) {
							t.Errorf("missing uncertainty: %v", streamErr)
						}
						if code == codes.Unavailable && repairFails && !errors.Is(streamErr, ErrPinnedSession) {
							t.Errorf("repair identity lost: %v", streamErr)
						}
					}
					checks := int32(1)
					if code == codes.Unavailable {
						checks = 2
					}
					if f.requests.Load() != 1 || f.checks.Load() != checks {
						t.Fatalf("requests=%d checks=%d", f.requests.Load(), f.checks.Load())
					}
				})
			}
		}
	}
}

func (f *pinnedFake) SubscribeTasks(r *proto.SubscribeTasksRequest, stream grpc.ServerStreamingServer[proto.TaskSubscriptionEvent]) error {
	f.taskRequests.Add(1)
	if r.ParentId == "cursor-test" {
		seq := int64(1)
		if len(r.Cursors) > 0 {
			seq = r.Cursors[0].Sequence + 1
		}
		if err := stream.Send(&proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Progress{Progress: &proto.TaskProgressRecord{TaskId: "task", Sequence: seq}}}); err != nil {
			return err
		}
		if seq > 1 {
			<-stream.Context().Done()
			return stream.Context().Err()
		}
	}
	if f.taskStarted != nil {
		close(f.taskStarted)
	}
	if f.taskRelease != nil {
		select {
		case <-f.taskRelease:
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
	return status.Error(codes.Unavailable, "task transport failed")
}
func TestPinnedTaskOnlyRecoveryBounded(t *testing.T) {
	f := &pinnedFake{host: "A", taskStarted: make(chan struct{}), taskRelease: make(chan struct{})}
	f.state.Store(1)
	s, addr := pinnedHost(t, "127.0.0.1:0", f)
	c, err := DialPinnedSession(context.Background(), pinAt(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	updates, err := c.SubscribeTasks(ctx, "conversation")
	if err != nil {
		t.Fatal(err)
	}
	<-f.taskStarted
	s.Stop()
	replacement := &pinnedFake{host: "B"}
	replacement.state.Store(1)
	pinnedHost(t, addr, replacement)
	select {
	case update, ok := <-updates:
		if !ok || !errors.Is(update.Err, ErrPinnedSession) {
			t.Fatalf("missing validation refusal: %+v", update)
		}
	case <-ctx.Done():
		t.Fatal("stale task retries never surfaced validation refusal")
	}
	if replacement.identities.Load() != 1 || replacement.taskRequests.Load() != 0 {
		t.Fatalf("identity=%d task=%d", replacement.identities.Load(), replacement.taskRequests.Load())
	}
}

func TestPinnedTaskRecoveryPreservesCursor(t *testing.T) {
	f := &pinnedFake{host: "A"}
	f.state.Store(1)
	_, addr := pinnedHost(t, "127.0.0.1:0", f)
	c, err := DialPinnedSession(context.Background(), pinAt(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	updates, err := c.SubscribeTasks(ctx, "cursor-test")
	if err != nil {
		t.Fatal(err)
	}
	for seq := int64(1); seq <= 2; seq++ {
		select {
		case update := <-updates:
			if update.Err != nil || update.Event.GetProgress().GetSequence() != seq {
				t.Fatalf("sequence %d: %+v", seq, update)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if f.checks.Load() != 2 || f.taskRequests.Load() != 2 {
		t.Fatalf("checks=%d subscriptions=%d", f.checks.Load(), f.taskRequests.Load())
	}
}

func TestPinnedPartialAndAuthBoundaries(t *testing.T) {
	for _, code := range []codes.Code{codes.Unavailable, codes.Unauthenticated, codes.PermissionDenied} {
		t.Run(code.String(), func(t *testing.T) {
			f := &pinnedFake{host: "A", partial: true, chatCode: code}
			f.state.Store(1)
			_, addr := pinnedHost(t, "127.0.0.1:0", f)
			c, err := DialPinnedSession(context.Background(), pinAt(addr))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			out, err := c.StreamChatWithRecovery(context.Background(), "conversation", "once", "")
			if err != nil {
				t.Fatal(err)
			}
			failures := 0
			for msg := range out {
				if msg.Err != nil {
					failures++
					if status.Code(msg.Err) != code {
						t.Fatalf("lost cause: %v", msg.Err)
					}
				}
			}
			checks := int32(1)
			if code == codes.Unavailable {
				checks = 2
			}
			if failures != 1 || f.checks.Load() != checks || f.requests.Load() != 1 {
				t.Fatalf("errors=%d checks=%d requests=%d", failures, f.checks.Load(), f.requests.Load())
			}
		})
	}
}
