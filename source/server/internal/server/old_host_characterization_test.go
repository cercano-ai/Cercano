package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Characterizes the red acceptance boundary using historical handlers, not an
// Unimplemented override. Run via the effort fixture script. No production host.
func TestActualOldRecoveryCharacterization(t *testing.T) {
	binary := os.Getenv("OLD_RECOVERY_BINARY")
	if binary == "" {
		t.Skip("requires pinned historical fixture; run effort fixture/run.sh")
	}
	root := t.TempDir()
	store := filepath.Join(root, "owned.db")
	ready := filepath.Join(root, "ready.json")
	cmd := exec.Command(binary, store, ready)
	cmd.Env = []string{"HOME=" + root, "TMPDIR=" + root, "CERCANO_DISABLE_CREDENTIALS=1"}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	stopped := false
	t.Cleanup(func() {
		if stopped {
			return
		}
		stdin.Close()
		select {
		case err := <-exited:
			if err != nil {
				t.Errorf("old fixture exit: %v", err)
			}
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
			<-exited
			t.Error("old fixture required owned-child kill")
		}
	})
	var old struct {
		Identity *pb.LifecycleIdentity
		Owner    *pb.LifecycleOwner
		Endpoint string
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, e := os.ReadFile(ready)
		if e == nil && json.Unmarshal(data, &old) == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("old fixture not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(recoveryFixturePeer(context.Background()), 15*time.Second)
	defer cancel()
	s, err := NewCoexistenceServer(ctx, CoexistenceOptions{StorePath: store, Host: "new-fixture", Build: "current-fixture", Contract: conversation.CurrentCoexistenceDeclaration(), ControlAuthorize: func(context.Context) error { return nil }, CloseParticipation: func(context.Context) error { return nil }, Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, o := range s.coexistence.owners {
			_ = o.Close()
		}
		_ = s.coexistence.store.Close()
	})
	_, destinationEndpoint := serveCutoverFixture(t, s)
	l := s.coexistence
	l.options.ResolveSource = func(_ context.Context, host string) (*pb.LifecycleIdentity, string, error) {
		if host != old.Identity.Host {
			return nil, "", fmt.Errorf("unknown fixture host")
		}
		return old.Identity, old.Endpoint, nil
	}
	l.options.ControlDial = func(_ context.Context, endpoint string, id *pb.LifecycleIdentity) (*grpc.ClientConn, error) {
		if endpoint != old.Endpoint || !proto.Equal(id, old.Identity) {
			return nil, fmt.Errorf("unknown fixture route")
		}
		return grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	l.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
		return &pb.LifecycleRecoverySelection{Identity: l.controlIdentity(), Endpoint: "127.0.0.1:19009", TokenFile: "fixture-only"}, nil
	}
	conn, err := l.options.ControlDial(ctx, old.Endpoint, old.Identity)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewAgentClient(conn)
	op := &pb.LifecycleControlRequest{Target: l.controlIdentity(), Source: old.Identity, Destination: l.controlIdentity(), Owner: old.Owner, Management: true, DestinationEndpoint: destinationEndpoint, HandoverId: "failed-one"}
	accepted, err := s.LifecyclePrepare(ctx, op)
	if err != nil {
		t.Fatal(err)
	}
	op.Target = old.Identity
	op.Receipt = accepted.Receipt
	if err = saveResume(resumeJournal(l, "chat"), op); err != nil {
		t.Fatal(err)
	}
	short, stop := context.WithTimeout(ctx, 150*time.Millisecond)
	_, err = client.LifecycleForceRelinquish(short, op)
	stop()
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("held old persistence: %v", err)
	}
	second := proto.Clone(op).(*pb.LifecycleControlRequest)
	second.HandoverId = "failed-two"
	if err = saveResume(resumeJournal(l, "chat")+".second.json", second); err != nil {
		t.Fatal(err)
	}
	before, err := conversation.ReadOwnership(ctx, l.store, "chat")
	if err != nil {
		t.Fatal(err)
	}
	request := resumeRequest(s, "127.0.0.1:19009")
	request.Requester = l.controlIdentity()
	got, err := s.LifecycleResume(ctx, request)
	// Opt-in red acceptance gate stops at the first missing production boundary.
	// It is not yet the full consent/containment/provider/persisted-turn journey.
	if os.Getenv("OLD_RECOVERY_ACCEPTANCE") == "1" && (err != nil || !got.GetConfirmationRequired()) {
		t.Fatalf("acceptance requires compatible recovery offer before a persisted new-host turn: %v %v", got, err)
	}
	if status.Code(err) != codes.Unimplemented || got != nil {
		t.Fatalf("expected actual old unsupported probe; got %v %v", got, err)
	}
	t.Logf("actual old resume boundary: %v", err)
	_, _, err = originalRecoveryCandidate(ctx, l, "chat")
	if status.Code(err) != codes.FailedPrecondition || !strings.Contains(err.Error(), "ambiguous incomplete") {
		t.Fatalf("multiple journal boundary: %v", err)
	}
	t.Logf("independent journal boundary: %v", err)
	after, err := conversation.ReadOwnership(ctx, l.store, "chat")
	if err != nil || before != after {
		t.Fatalf("discovery changed owner: %v -> %v (%v)", before, after, err)
	}
	if len(l.recoveryOffers) != 0 || len(l.owners) != 0 {
		t.Fatal("failed discovery granted consent or ownership")
	}
	// Separate compatibility probe, NOT recovery acceptance: fixture-owned EOF,
	// not production source attribution, releases the historical store lock.
	stdin.Close()
	select {
	case err := <-exited:
		stopped = true
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	recovered, err := conversation.RecoverDeadOwner(ctx, l.store, "chat", before, l.options.Host, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	writeCtx := conversation.WithOwnership(ctx, recovered)
	if err = l.store.EnsureConversation(writeCtx, "chat", "", ""); err != nil {
		t.Fatal(err)
	}
	if err = l.store.Append(writeCtx, conversation.Turn{ConversationID: "chat", Role: "assistant", Content: "fixture compatibility write"}); err != nil {
		t.Fatal(err)
	}
	reader, err := conversation.OpenCoexistence(ctx, store, conversation.CurrentCoexistenceDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	turns, err := reader.GetTurns(ctx, "chat")
	if err != nil || len(turns) != 1 || turns[0].Content != "fixture compatibility write" {
		t.Fatalf("persisted compatibility turn: %v %v", turns, err)
	}
	t.Log("historical store accepted current dead-owner recovery and persisted append after owned fixture exit")
	// This is not a declined-confirmation journey: current code cannot offer it.
	// The future acceptance must recover, reload, and persist an exact new turn.
}
