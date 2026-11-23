package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/chatroute"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/dispatch"
	"cercano/source/server/internal/failurelog"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/taskrun"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/config"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// Exercise the real explicit-close/drain callback, not just ticket completion.
func TestWorkerPersistenceTerminalDrain(t *testing.T) {
	for _, mode := range []string{"model", "cancel", "deadline", "write", "canceled-write", "descendant-write", "callback", "session-set", "session-clear", "session-canceled-write", "session-validation", "session-read", "task-insert", "task-validation", "task-authorization"} {
		t.Run(mode, func(t *testing.T) {
			credentialtest.Enable(t)
			cfg := config.Defaults()
			cfg.OpenRuntime = "synthetic"
			cfg.CloudProfiles = []config.CloudProfile{{Name: "saved", Provider: "deepinfra", Flavor: "chat_completions", Model: "default", BaseURL: "https://example.invalid/v1"}}
			s, _, owner, _ := preparedWakeFixtureWithConfig(t, cfg)
			l := s.coexistence
			path := filepath.Join(t.TempDir(), "diagnostics.jsonl")
			log, err := failurelog.NewWriter(path)
			if err != nil {
				t.Fatal(err)
			}
			s.failureLog = log
			attachment, err := l.admission.AttachTerminal(owner, "pane")
			if err != nil {
				t.Fatal(err)
			}
			defer attachment.Detach()
			ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(agentclient.PinnedHostMetadata, owner.Host, agentclient.PinnedConversationMetadata, owner.Conversation, agentclient.PinnedEpochMetadata, owner.Epoch, agentclient.PinnedTerminalMetadata, "pane"))
			ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}})
			workctx, ticket, err := s.BeginRetainedWork(ctx, owner, broker.PersistenceWork)
			if err != nil {
				t.Fatal(err)
			}
			a := &preparedWorkerAuthority{server: s, ctx: workctx, id: owner.Conversation, ticket: ticket}
			secret := errors.New("SECRET prompt /private/path credential receipt")
			outcome := secret
			if mode == "cancel" || mode == "canceled-write" || mode == "session-canceled-write" {
				outcome = fmt.Errorf("worker: %w", context.Canceled)
			}
			if mode == "deadline" {
				outcome = fmt.Errorf("worker: %w", context.DeadlineExceeded)
			}
			if mode == "write" || mode == "canceled-write" {
				persistsvc.ReportWriteFailure(workctx, secret)
			}
			if mode == "descendant-write" {
				if err := a.Ensure(workctx, "drain-child", owner.Conversation, "", "", nil); err != nil {
					t.Fatal(err)
				}
				child, err := a.Context(workctx, "drain-child")
				if err != nil {
					t.Fatal(err)
				}
				persistsvc.ReportWriteFailure(child, secret)
			}
			if strings.HasPrefix(mode, "session-") {
				keys := secrets.NewMemory()
				if err := keys.Set("saved", "fixture-key"); err != nil {
					t.Fatal(err)
				}
				s.SetSecrets(keys)
				s.persistSvc = sessionFailureService{Service: s.persistSvc, store: sessionFailureStore{Store: s.persistSvc.Store(), route: s.persistSvc.Store().(chatroute.Store), err: secret, read: mode == "session-read"}}
				req := chatroute.Request{Action: "clear"}
				if mode == "session-set" {
					req = chatroute.Request{Action: "set", Profile: "saved", Model: "exact"}
				}
				if mode == "session-validation" {
					req = chatroute.Request{Action: "set", Profile: "missing", Model: "exact"}
				}
				if mode == "session-read" {
					req.Action = "status"
				}
				if _, err := s.sessionModel(workctx, owner.Conversation, req); err == nil {
					t.Fatal("expected session model failure")
				}
			}
			if strings.HasPrefix(mode, "task-") {
				insertErr := error(&conversation.MutationError{Err: secret})
				if mode == "task-authorization" {
					insertErr = secret
				}
				manager, err := taskrun.NewPrepared(taskInsertFailureStore{TaskStore: l.store.(conversation.TaskStore), err: insertErr}, func(context.Context, dispatch.Spec) (dispatch.Result, error) {
					t.Error("execution started despite insert failure")
					return dispatch.Result{}, nil
				}, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer manager.Close(context.Background())
				s.taskManager = manager
				spec := dispatch.Spec{ConversationID: owner.Conversation, Task: "work"}
				if mode == "task-validation" {
					spec.Task = ""
				}
				if _, err := (serverTasks{s}).Launch(workctx, spec); err == nil {
					t.Fatal("expected task launch failure")
				}
			}
			if mode == "callback" {
				outcome = errors.Join(context.Canceled, fmt.Errorf("callback: %w", &worker.CallbackCleanupError{Err: secret}))
			}
			l.options.ControlAuthorize = func(context.Context) error { return nil }
			response, err := s.LifecycleRelease(ctx, &pb.LifecycleControlRequest{Target: l.controlIdentity(), Source: l.controlIdentity(), Owner: wireOwner(owner)})
			if err != nil || response == nil {
				t.Fatalf("close: %v %v", response, err)
			}
			before := l.snapshotBlockers(owner)
			if !before.Admission.Closed || !before.Admission.Draining {
				t.Fatalf("not draining: %+v", before)
			}
			a.Close(outcome)
			a.Close(nil) // idempotence must never clear a retained failure
			uncertain := mode == "task-insert" || mode == "write" || mode == "canceled-write" || mode == "descendant-write" || mode == "callback" || mode == "session-set" || mode == "session-clear" || mode == "session-canceled-write"
			if uncertain {
				snap := l.snapshotBlockers(owner)
				if !snap.Admission.Closed || !snap.Admission.Draining || snap.Admission.UnresolvedWork == 0 {
					t.Fatalf("lost blocker: %+v", snap)
				}
				if _, err := l.admission.AttachTerminal(owner, "new"); !errors.Is(err, broker.ErrAdmissionClosed) {
					t.Fatalf("admission reopened: %v", err)
				}
				state, err := conversation.ReadOwnership(t.Context(), l.store, owner.Conversation)
				if err != nil || state.State != "owned" {
					t.Fatalf("lost durable owner: %+v %v", state, err)
				}
				if _, err := s.ListRetained(); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(data), `"event":"lifecycle.persistence_retained"`) || !strings.Contains(string(data), `"origin":"worker_close"`) {
					t.Fatalf("missing provenance: %s", data)
				}
				for _, forbidden := range []string{"SECRET", "prompt", "/private/path", "credential", "receipt"} {
					if strings.Contains(string(data), forbidden) {
						t.Fatalf("unsafe diagnostics: %s", data)
					}
				}
			} else {
				deadline := time.Now().Add(5 * time.Second)
				for {
					retained, err := s.ListRetained()
					if err != nil {
						t.Fatal(err)
					}
					if len(retained) == 0 {
						break
					}
					if time.Now().After(deadline) {
						t.Fatalf("execution failure blocked drain: %+v", l.snapshotBlockers(owner))
					}
					time.Sleep(time.Millisecond)
				}
				state, err := conversation.ReadOwnership(t.Context(), l.store, owner.Conversation)
				if err != nil || state.State != "released" {
					t.Fatalf("not durably released: %+v %v", state, err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(data), "lifecycle.persistence_retained") {
					t.Fatalf("execution classified as persistence: %s", data)
				}
			}
		})
	}
}

// Fail at the mutation boundary independently of the worker execution outcome.
type sessionFailureService struct {
	persistsvc.Service
	store conversation.Store
}

func (s sessionFailureService) Store() conversation.Store { return s.store }

type sessionFailureStore struct {
	conversation.Store
	route chatroute.Store
	err   error
	read  bool
}

func (s sessionFailureStore) SetChatRoute(context.Context, string, *chatroute.Route) error {
	return &conversation.MutationError{Err: s.err}
}
func (s sessionFailureStore) ChatRoute(ctx context.Context, id string) (*chatroute.Route, error) {
	if s.read {
		return nil, s.err
	}
	return s.route.ChatRoute(ctx, id)
}

// Launch must preserve insertion uncertainty even before a task becomes active.
type taskInsertFailureStore struct {
	conversation.TaskStore
	err error
}

func (s taskInsertFailureStore) InsertTask(context.Context, conversation.TaskRecord) error {
	return s.err
}
