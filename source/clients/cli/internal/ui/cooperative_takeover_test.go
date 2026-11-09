package ui

import (
	"strings"
	"testing"

	pb "cercano/source/server/pkg/proto"
	"google.golang.org/protobuf/proto"
)

func TestTakeoverUIRequiresDistinctExplicitConfirmation(t *testing.T) {
	source := &pb.LifecycleIdentity{Host: "S", Build: "build", Store: "store", Contract: "contract", Schema: 1}
	dest := proto.Clone(source).(*pb.LifecycleIdentity)
	dest.Host = "D"
	o := &pb.LifecycleOriginalRecovery{Takeover: true, Challenge: "single-use", Selection: &pb.LifecycleRecoverySelection{Identity: dest, Endpoint: "127.0.0.1:1234", TokenFile: "fixture-not-read"}, Operation: &pb.LifecycleControlRequest{Target: source, Source: source, Destination: dest, Owner: &pb.LifecycleOwner{Conversation: "history", Host: "S", Epoch: "epoch"}, HandoverId: "new-op"}}
	if err := validateOriginalOffer(o, "history"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"enter", "esc", "n", "y"} {
		t.Run(key, func(t *testing.T) {
			selected := DevHostSelection{Identity: dest, Conversation: "current"}
			m := New(nil, false).WithDevHost(selected, "fixture")
			offered, cmd := m.offerForcedResume(&forcedResumeOffer{destination: selected, original: o})
			m = offered.(Model)
			if cmd != nil || m.pendingConfirm == nil || !strings.Contains(m.pendingConfirm.title, "Take over") {
				t.Fatal("missing distinct takeover confirmation")
			}
			details := strings.Join(m.pendingConfirm.details, " ")
			for _, warning := range []string{"NEW cooperative takeover", "not an original-operation retry", "not be replayed", "NOT rolled back", "history will be reloaded"} {
				if !strings.Contains(details, warning) {
					t.Fatalf("missing warning %q", warning)
				}
			}
			_, cmd = m.resolveConfirmKey(key)
			if (cmd != nil) != (key == "y") {
				t.Fatal("takeover bypassed explicit consent")
			}
		})
	}
	for _, change := range []string{"legacy", "store", "epoch"} {
		bad := proto.Clone(o).(*pb.LifecycleOriginalRecovery)
		switch change {
		case "legacy":
			bad.Legacy = true
		case "store":
			bad.Operation.Source.Store = "other-store"
		case "epoch":
			bad.Operation.Owner.Epoch = ""
		}
		if err := validateOriginalOffer(bad, "history"); err == nil {
			t.Fatalf("accepted invalid %s binding", change)
		}
	}
}
