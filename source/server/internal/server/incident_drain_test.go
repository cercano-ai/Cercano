package server

// This is a constructed downstream-state characterization, not a reproduction
// of the unknown origin of the incident's three persistence tickets.
import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/agentclient"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/metadata"
)

func TestIncidentClosedDrainCharacterization(t *testing.T) {
	if os.Getenv("CERCANO_INCIDENT_FIXTURE") != "1" {
		root := t.TempDir()
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestIncidentClosedDrainCharacterization$", "-test.v")
		cmd.Env = append(os.Environ(), "CERCANO_INCIDENT_FIXTURE=1", "HOME="+root, "XDG_CONFIG_HOME="+root, "CERCANO_DISABLE_CREDENTIALS=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture: %v\n%s", err, out)
		}
		t.Logf("%s", out)
		return
	}
	ctx := reproLoopbackContext()
	a, b, _ := lifecycleFixture(t)
	_, source := serveCutoverFixture(t, a)
	_, dest := serveCutoverFixture(t, b)
	setResumeSource(b, a, source)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	owner, err := a.Retain(ctx, "chat")
	must(err)
	owned := conversation.WithOwnership(ctx, a.coexistence.owners["chat"])
	must(a.coexistence.store.EnsureConversation(owned, "chat", "", "fixture"))
	tasks := a.coexistence.store.(conversation.TaskStore)
	for i := 0; i < 10; i++ {
		r := conversation.TaskRecord{ID: fmt.Sprintf("incident-%d", i), ParentID: "chat", ChildID: fmt.Sprintf("child-%d", i), Status: "accepted", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		must(tasks.InsertTask(owned, r))
		r.Status = "completed"
		if i == 9 {
			r.Status = "failed"
		}
		ok, e := tasks.FinishTask(owned, r)
		must(e)
		if !ok {
			t.Fatal("terminal task not saved")
		}
	}
	db, err := sql.Open("sqlite", a.coexistence.options.StorePath)
	must(err)
	defer db.Close()
	assertTasks := func() {
		t.Helper()
		var total, completed, failed int
		must(db.QueryRowContext(ctx, `SELECT count(*), sum(status='completed'), sum(status='failed') FROM delegated_tasks WHERE parent_id='chat'`).Scan(&total, &completed, &failed))
		if total != 10 || completed != 9 || failed != 1 {
			t.Fatalf("terminal task counts: %d %d %d", total, completed, failed)
		}
	}
	assertTasks()
	for i := 0; i < 3; i++ {
		ticket, e := a.AdmitRetainedWork(owner, broker.PersistenceWork)
		must(e)
		must(ticket.PersistenceFailed())
	}
	attachment, err := a.coexistence.admission.AttachTerminal(owner, "incident-pane")
	must(err)
	defer attachment.Detach()
	releaseCtx := metadata.NewIncomingContext(ctx, metadata.Pairs(agentclient.PinnedHostMetadata, owner.Host, agentclient.PinnedConversationMetadata, owner.Conversation, agentclient.PinnedEpochMetadata, owner.Epoch, agentclient.PinnedTerminalMetadata, "incident-pane"))
	_, err = a.LifecycleRelease(releaseCtx, &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Owner: wireOwner(owner)})
	must(err)
	p := &containedFixture{}
	b.coexistence.options.RecoverySource = p
	b.coexistence.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
		return &pb.LifecycleRecoverySelection{Identity: b.coexistence.controlIdentity(), Endpoint: dest, TokenFile: "fixture"}, nil
	}
	before, err := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
	must(err)
	for i := 0; i < 3; i++ {
		r := resumeRequest(b, dest)
		r.Requester = b.coexistence.controlIdentity()
		offer, e := b.LifecycleResume(ctx, r)
		if e != nil || !offer.GetConfirmationRequired() {
			t.Fatalf("resume %d: %v %v", i, offer, e)
		}
		after, e := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
		must(e)
		if after != before || p.stop != 0 {
			t.Fatalf("unconsented recovery changed owner or stopped source: %+v stops=%d", after, p.stop)
		}
		assertTasks()
		snap := a.coexistence.admission.Snapshot(owner)
		if !snap.Closed || !snap.Draining || snap.Attachments != 0 || snap.Work[broker.PersistenceWork] != 3 || snap.UnresolvedWork != 3 || snap.HostWork[broker.PersistenceWork] != 1 || snap.UnresolvedHostWork != 0 {
			t.Fatalf("terminal tasks cleared uncertainty: %+v", snap)
		}
		a.coexistence.mu.Lock()
		closure := a.coexistence.closures[owner]
		a.coexistence.mu.Unlock()
		if closure == nil {
			t.Fatal("terminal closure lost")
		}
		_, uncertain, e := closure.closure.Pending()
		must(e)
		if uncertain != 3 {
			t.Fatalf("closure uncertainty=%d", uncertain)
		}
		t.Logf("attempt=%d exact owner=%+v snapshot=%+v offer=%v", i, owner, snap, offer.GetConfirmationRequired())
	}
	// Force-cutover must stop at the already reserved close, not a later
	// unresolved-persistence stage (which would mischaracterize this incident).
	_, err = a.LifecycleForceRelinquish(ctx, &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Owner: wireOwner(owner), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: dest, Management: true, HandoverId: "incident-probe"})
	if !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("existing closure refusal: %v", err)
	}
	t.Logf("existing closure refusal: %v", err)
}
