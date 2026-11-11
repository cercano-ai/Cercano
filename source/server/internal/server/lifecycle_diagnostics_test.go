package server

import (
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/failurelog"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestResumeFailedReloadReproduction(t *testing.T) {
	s, _, _ := lifecycleFixture(t)
	path := filepath.Join(t.TempDir(), "failures.jsonl")
	w, e := failurelog.NewWriter(path)
	if e != nil {
		t.Fatal(e)
	}
	defer w.Close()
	s.failureLog = w
	calls := 0
	s.coexistence.options.Reload = func(context.Context, conversation.Store, *conversation.Ownership) error {
		calls++
		return errors.New("private fixture failure")
	}
	if _, err := s.Retain(context.Background(), "failed"); err == nil {
		t.Fatal("expected reload failure")
	}
	client, endpoint := serveCutoverFixture(t, s)
	for i := 0; i < 2; i++ {
		got, err := client.LifecycleResume(context.Background(), &pb.LifecycleControlRequest{Target: s.coexistence.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: "failed"}, DestinationEndpoint: endpoint})
		if status.Code(err) != codes.Unknown || err == nil || status.Convert(err).Message() != "retained sessions or work prevent transition" || got.GetReady() {
			t.Fatalf("resume: %v %v", got, err)
		}
	}
	if calls != 1 {
		t.Fatalf("reload calls=%d", calls)
	}
	o := ownerOf(s.coexistence.owners["failed"])
	before := s.coexistence.snapshotBlockers(o)
	if before.Known != true || before.Ready || before.Reason != "acquisition_unresolved" || before.AcquisitionStage != "reload" || before.Admission.UnresolvedHostWork != 1 || before.Admission.HostWork[broker.PersistenceWork] != 1 {
		t.Fatalf("snapshot: %+v", before)
	}
	assertSnapshotReadOnly(t, s, o)
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(data), "private fixture failure") || !strings.Contains(string(data), `"error_kind":"admission_busy"`) || !strings.Contains(string(data), `"stage":"acquire"`) || !strings.Contains(string(data), `"AcquisitionStage":"reload"`) || !strings.Contains(string(data), `"check":"resume.go:`) {
		t.Fatalf("unsafe or missing diagnostics: %s", data)
	}
	if calls != 1 {
		t.Fatalf("snapshot invoked reload: %d", calls)
	}
}

// Compare every fixture file, including SQLite sidecars and operation journals.
func fixtureFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		out[path] = string(b) + info.ModTime().String()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func assertSnapshotReadOnly(t *testing.T, s *Server, o broker.Owner) {
	t.Helper()
	l := s.coexistence
	// A pending receipt is a tripwire: observing it would deliver and rewrite it.
	l.launches = map[string]*pb.LifecycleControlRequest{}
	receipt := &pb.LifecycleControlRequest{Owner: wireOwner(o), LaunchOperation: "fixture-launch", LaunchAction: "pending"}
	if e := l.saveLaunch(receipt, "pending"); e != nil {
		t.Fatal(e)
	}
	if e := saveResume(resumeJournal(l, o.Conversation), &pb.LifecycleControlRequest{Receipt: "private-fixture-receipt"}); e != nil {
		t.Fatal(e)
	}
	before := l.snapshotBlockers(o)
	cap := l.owners[o.Conversation]
	files := fixtureFiles(t, filepath.Dir(l.options.StorePath))
	for i := 0; i < 10; i++ {
		if got := l.snapshotBlockers(o); !reflect.DeepEqual(before, got) {
			t.Fatalf("snapshot changed: %+v", got)
		}
	}
	if l.owners[o.Conversation] != cap || l.launches["fixture-launch"].LaunchAction != "pending" || !reflect.DeepEqual(files, fixtureFiles(t, filepath.Dir(l.options.StorePath))) {
		t.Fatal("snapshot mutated owner, receipt, or fixture files")
	}
	// Even already-delivered receipts must not be rewritten by observation.
	if err := l.saveLaunch(receipt, "delivered"); err != nil {
		t.Fatal(err)
	}
	files = fixtureFiles(t, filepath.Dir(l.options.StorePath))
	for i := 0; i < 10; i++ {
		if got := l.snapshotBlockers(o); !reflect.DeepEqual(before, got) {
			t.Fatalf("delivered snapshot changed: %+v", got)
		}
	}
	if !reflect.DeepEqual(files, fixtureFiles(t, filepath.Dir(l.options.StorePath))) {
		t.Fatal("snapshot rewrote a delivered receipt")
	}
	// Returned maps cannot mutate accounting.
	before.Admission.HostWork[broker.PersistenceWork] = 1234
	if l.snapshotBlockers(o).Admission.HostWork[broker.PersistenceWork] == 1234 {
		t.Fatal("snapshot aliases host accounting")
	}
}

func TestLifecycleSnapshotControlWorkAttachmentsAndMismatch(t *testing.T) {
	s, _, _ := lifecycleFixture(t)
	l := s.coexistence
	o, e := s.Retain(context.Background(), "control")
	if e != nil {
		t.Fatal(e)
	}
	if got := l.snapshotBlockers(o); !got.Ready || got.Reason != "ready" || !got.Admission.Open {
		t.Fatalf("control: %+v", got)
	}
	assertSnapshotReadOnly(t, s, o)
	for _, bad := range []broker.Owner{{Conversation: "unknown", Host: o.Host, Epoch: o.Epoch}, {Conversation: o.Conversation, Host: "foreign", Epoch: o.Epoch}, {Conversation: o.Conversation, Host: o.Host, Epoch: "stale"}, {Conversation: o.Conversation}} {
		got := l.snapshotBlockers(bad)
		if got.Known || got.Ready || got.Admission.Retained || got.Reason != "unknown_or_mismatched_owner" {
			t.Fatalf("mismatch: %+v", got)
		}
	}
	attachment, e := l.admission.AttachOwner(o)
	if e != nil {
		t.Fatal(e)
	}
	defer attachment.Detach()
	if got := l.snapshotBlockers(o); got.Reason != "attachments" || got.Admission.Attachments != 1 {
		t.Fatalf("attachment: %+v", got)
	}
	tickets := []*broker.WorkTicket{}
	for _, kind := range []broker.WorkKind{broker.ForegroundWork, broker.TaskAcceptanceWork, broker.TaskExecutionWork, broker.CallbackWork, broker.PersistenceWork} {
		ticket, e := l.admission.AdmitWork(o, kind)
		if e != nil {
			t.Fatal(e)
		}
		tickets = append(tickets, ticket)
	}
	if e := tickets[4].PersistenceFailed(); e != nil {
		t.Fatal(e)
	}
	got := l.snapshotBlockers(o)
	if got.Reason != "session_work" || got.Admission.UnresolvedWork != 1 || got.Admission.Attachments != 1 {
		t.Fatalf("work: %+v", got)
	}
	for kind := broker.ForegroundWork; kind <= broker.PersistenceWork; kind++ {
		if got.Admission.Work[kind] != 1 {
			t.Fatalf("kind %v: %+v", kind, got)
		}
	}
	assertSnapshotReadOnly(t, s, o)
	// Snapshot concurrently with accounting changes; race testing validates lock order.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			l.snapshotBlockers(o)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			ticket, e := l.admission.AdmitWork(o, broker.CallbackWork)
			if e == nil {
				_ = ticket.Complete()
			}
		}
	}()
	wg.Wait()
	for _, ticket := range tickets[:4] {
		_ = ticket.Complete()
	}
}

func TestLifecycleResumeDeniedLoggingPrivacy(t *testing.T) {
	s, _, _ := lifecycleFixture(t)
	path := filepath.Join(t.TempDir(), "failures.jsonl")
	w, e := failurelog.NewWriter(path)
	if e != nil {
		t.Fatal(e)
	}
	defer w.Close()
	s.failureLog = w
	_, e = s.LifecycleResume(context.Background(), &pb.LifecycleControlRequest{Owner: &pb.LifecycleOwner{Conversation: "private-conversation"}, Receipt: "private-token"})
	if status.Code(e) != codes.PermissionDenied {
		t.Fatal(e)
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(data), "private-") || strings.Contains(string(data), "blockers") || !strings.Contains(string(data), `"stage":"control"`) {
		t.Fatalf("denied request leaked: %s", data)
	}
}

func TestForceSourceRefusalDiagnostics(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	ac, _ := serveCutoverFixture(t, a)
	_, endpoint := serveCutoverFixture(t, b)
	path := filepath.Join(t.TempDir(), "source.jsonl")
	w, err := failurelog.NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	a.failureLog = w
	a.coexistence.options.Reload = func(context.Context, conversation.Store, *conversation.Ownership) error {
		return errors.New("SECRET-prompt")
	}
	if _, err = a.Retain(context.Background(), "blocked"); err == nil {
		t.Fatal("expected unresolved acquisition")
	}
	o := ownerOf(a.coexistence.owners["blocked"])
	r := &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: endpoint, Owner: wireOwner(o), Management: true, HandoverId: "source-operation", Receipt: "SECRET-receipt"}
	if _, err = ac.LifecycleForceRelinquish(context.Background(), r); err == nil {
		t.Fatal("expected refusal")
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{`"operation":"force_relinquish"`, `"side":"source"`, `"stage":"owner_ready"`, `"handover_id":"source-operation"`, `"Reason":"acquisition_unresolved"`, `"UnresolvedHostWork":1`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %s in %s", want, data)
		}
	}
	for _, secret := range []string{"SECRET", endpoint, a.coexistence.options.StorePath} {
		if strings.Contains(string(data), secret) {
			t.Errorf("sensitive diagnostic: %s", data)
		}
	}
}

// Ordinary relinquish must report its own admission refusal, not a destination snapshot.
func TestRelinquishSourceAttachmentDiagnostics(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	ac, _ := serveCutoverFixture(t, a)
	_, endpoint := serveCutoverFixture(t, b)
	o, err := a.Retain(context.Background(), "attached")
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := a.coexistence.admission.AttachTerminal(o, "SECRET-terminal")
	if err != nil {
		t.Fatal(err)
	}
	_ = attachment
	path := filepath.Join(t.TempDir(), "source.jsonl")
	w, err := failurelog.NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	a.failureLog = w
	r := &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: endpoint, Owner: wireOwner(o), Management: true, HandoverId: "attached-operation"}
	if _, err = ac.LifecycleRelinquish(context.Background(), r); err == nil {
		t.Fatal("expected attachment refusal")
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{`"operation":"relinquish"`, `"side":"source"`, `"stage":"reserve_quiescence"`, `"Attachments":1`, `"Reason":"attachments"`, `"handover_id":"attached-operation"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %s in %s", want, data)
		}
	}
	if strings.Contains(string(data), "SECRET") {
		t.Fatalf("sensitive diagnostic: %s", data)
	}
}
