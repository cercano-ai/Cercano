package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "cercano/source/server/pkg/proto"
)

// Only disposable stores are edited to model failed historical transfer journals.
// Recovery itself goes over authenticated RPC to the ordinary production host.
func historicalCoordinatorRecovery(t *testing.T, ctx context.Context, state string, source, destination launchSelection) launchSelection {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(state, "conversations.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if strings.HasSuffix(t.Name(), "reserved") {
		_, err = db.Exec(`UPDATE conversation_ownership SET state='reserved',destination='failed-B',handover='failed-transfer' WHERE conversation_id=? AND owner=? AND epoch=?`, source.Conversation, source.Identity.Host, source.Epoch)
		if err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(state, "conversations.db.resume-operations")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"failed-B", "failed-C"} {
		target := &pb.LifecycleIdentity{Host: host, Store: source.Identity.Store, Contract: source.Identity.Contract, Schema: source.Identity.Schema}
		op := &pb.LifecycleControlRequest{Source: source.Identity, Target: source.Identity, Destination: target, Owner: &pb.LifecycleOwner{Host: source.Identity.Host, Conversation: source.Conversation, Epoch: source.Epoch}, HandoverId: "failed-" + host}
		b, _ := json.Marshal(op)
		if err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%x.json", sha256.Sum256([]byte(host+"\x00"+source.Conversation)))), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	controls, err := launchControls(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	defer controls.Close()
	request := &pb.LifecycleControlRequest{Target: destination.Identity, Requester: destination.Identity, Owner: &pb.LifecycleOwner{Conversation: source.Conversation}}
	first, err := controls.LifecycleResume(ctx, request)
	if err != nil || !first.GetConfirmationRequired() {
		t.Fatalf("first confirmation: %v %v", first, err)
	}
	fresh, err := controls.LifecycleResume(ctx, request)
	if err != nil || !fresh.GetConfirmationRequired() || fresh.OriginalRecovery.Challenge == first.OriginalRecovery.Challenge {
		t.Fatalf("fresh confirmation: %v %v", fresh, err)
	}
	var beforeHost, beforeEpoch, beforeState string
	if err = db.QueryRow(`SELECT owner,epoch,state FROM conversation_ownership WHERE conversation_id=?`, source.Conversation).Scan(&beforeHost, &beforeEpoch, &beforeState); err != nil {
		t.Fatal(err)
	}
	expectedState := "owned"
	if strings.HasSuffix(t.Name(), "reserved") {
		expectedState = "reserved"
	}
	if beforeHost != source.Identity.Host || beforeEpoch != source.Epoch || beforeState != expectedState {
		t.Fatalf("confirmation offer mutated ownership: %s %s %s", beforeHost, beforeEpoch, beforeState)
	}
	request.ConfirmOriginalRecovery = true
	request.RecoveryChallenge = first.OriginalRecovery.Challenge
	if _, err = controls.LifecycleResume(ctx, request); err == nil {
		t.Fatal("superseded confirmation accepted")
	}
	request.RecoveryChallenge = fresh.OriginalRecovery.Challenge
	result, err := controls.LifecycleResume(ctx, request)
	if err != nil || !result.GetReady() || !result.GetCurrentOwner() || result.GetOwner().GetHost() != destination.Identity.Host || result.GetOwner().GetEpoch() == source.Epoch {
		t.Fatalf("recover original: %v %v", result, err)
	}
	var host, epoch, status string
	if err = db.QueryRow(`SELECT owner,epoch,state FROM conversation_ownership WHERE conversation_id=?`, source.Conversation).Scan(&host, &epoch, &status); err != nil {
		t.Fatal(err)
	}
	if host != destination.Identity.Host || epoch != result.Owner.Epoch || status != "owned" {
		t.Fatalf("durable recovered owner %s %s %s", host, epoch, status)
	}
	destination.Conversation = source.Conversation
	destination.Epoch = epoch
	t.Log("multiple failed journals ignored; superseded consent refused; fresh consent recovered exact durable owner")
	return destination
}
