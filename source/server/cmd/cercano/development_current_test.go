package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/server"
	pb "cercano/source/server/pkg/proto"
)

func TestCurrentTargetRetiresOnlyDrainedPreviousHost(t *testing.T) {
	ctx := t.Context()
	state := t.TempDir()
	store := filepath.Join(state, "conversations.db")
	if err := conversation.PrepareCoexistence(ctx, store); err != nil {
		t.Fatal(err)
	}
	contract := conversation.CurrentCoexistenceDeclaration()
	srv, err := server.NewCoexistenceServer(ctx, server.CoexistenceOptions{
		StorePath: store, Host: "A", Build: "build-A", Contract: contract,
		Reload:             func(context.Context, conversation.Store, *conversation.Ownership) error { return nil },
		CloseParticipation: func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.CloseCoexistenceStore()
	id := &pb.LifecycleIdentity{Host: "A", Build: "build-A", Store: store, Contract: contract.Contract, Schema: int32(contract.SchemaVersion)}
	assertAlive := func() {
		t.Helper()
		select {
		case <-srv.CoexistenceRetired():
			t.Fatal("A retired prematurely")
		default:
		}
	}
	retireIfNotCurrent(ctx, state, id, "", srv.RetireCoexistence)
	assertAlive() // No discovery record cannot retire a host.
	if err := publishCurrentTarget(state, launchSelection{Identity: id}); err != nil {
		t.Fatal(err)
	}
	retireIfNotCurrent(ctx, state, id, "", srv.RetireCoexistence)
	assertAlive() // Current idle host must remain usable.
	owner, err := srv.Retain(ctx, "existing-A-session")
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := srv.AttachRetained(owner)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := srv.AdmitRetainedWork(owner, broker.ForegroundWork)
	if err != nil {
		t.Fatal(err)
	}
	stop := watchCurrentTarget(ctx, state, id, srv.RetireCoexistence)
	defer stop()
	b := &pb.LifecycleIdentity{Host: "B", Build: "build-B", Store: store, Contract: id.Contract, Schema: id.Schema}
	if err := publishCurrentTarget(state, launchSelection{Identity: b}); err != nil {
		t.Fatal(err)
	}
	retireIfNotCurrent(ctx, state, id, "", srv.RetireCoexistence)
	assertAlive()
	attachment.Detach()
	retireIfNotCurrent(ctx, state, id, "", srv.RetireCoexistence)
	assertAlive() // Disconnect is not session closure, nor work completion.
	if err := srv.ReleaseRetained(ctx, owner, nil); err == nil {
		t.Fatal("released outstanding work")
	}
	if err := ticket.Complete(); err != nil {
		t.Fatal(err)
	}
	retireIfNotCurrent(ctx, state, id, "", srv.RetireCoexistence)
	assertAlive() // Completed work does not close the session.
	later, err := srv.AdmitRetainedWork(owner, broker.ForegroundWork)
	if err != nil {
		t.Fatalf("later A work refused: %v", err)
	}
	if err := later.Complete(); err != nil {
		t.Fatal(err)
	}
	if err := srv.ReleaseRetained(ctx, owner, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-srv.CoexistenceRetired():
	case <-time.After(5 * time.Second):
		t.Fatal("drained A did not retire automatically")
	}
}

// A ready-but-not-yet-selected B must survive an arbitrarily slow launcher.
func TestCurrentTargetDoesNotRetireStartupCandidate(t *testing.T) {
	state := t.TempDir()
	a := &pb.LifecycleIdentity{Host: "A", Store: "store", Contract: "contract", Schema: 2}
	b := &pb.LifecycleIdentity{Host: "B", Store: "store", Contract: "contract", Schema: 2}
	if err := publishCurrentTarget(state, launchSelection{Identity: a}); err != nil {
		t.Fatal(err)
	}
	retired := make(chan struct{}, 8)
	stop := watchCurrentTarget(t.Context(), state, b, func(context.Context) error { retired <- struct{}{}; return nil })
	defer stop()
	select {
	case <-retired:
		t.Fatal("unselected B retired while A still current")
	case <-time.After(600 * time.Millisecond):
	}
	if err := publishCurrentTarget(state, launchSelection{Identity: b}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-retired:
		t.Fatal("current B retired")
	case <-time.After(600 * time.Millisecond):
	}
	c := &pb.LifecycleIdentity{Host: "C", Store: "store", Contract: "contract", Schema: 2}
	if err := publishCurrentTarget(state, launchSelection{Identity: c}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-retired:
	case <-time.After(3 * time.Second):
		t.Fatal("B did not retire after next forward launch")
	}
}
