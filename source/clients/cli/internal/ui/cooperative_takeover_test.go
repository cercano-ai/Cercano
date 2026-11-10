package ui

import (
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/protobuf/proto"
	"testing"
)

// Port the old takeover UI's binding regressions to the one contained-source
// offer. Superseded retry/takeover offers must not become host-stop consent.
func TestRecoveryUIRejectsInvalidBindings(t *testing.T) {
	source := &pb.LifecycleIdentity{Host: "S", Build: "build", Store: "store", Contract: "contract", Schema: 1}
	dest := proto.Clone(source).(*pb.LifecycleIdentity)
	dest.Host = "D"
	offer := &pb.LifecycleOriginalRecovery{Challenge: "single-use", Selection: &pb.LifecycleRecoverySelection{Identity: dest, Endpoint: "127.0.0.1:1234", TokenFile: "never-read"}, Operation: &pb.LifecycleControlRequest{Target: source, Source: source, Destination: dest, Owner: &pb.LifecycleOwner{Conversation: "history", Host: "S", Epoch: "epoch"}, HandoverId: "op", LegacyProcessIdentity: "snapshot"}}
	if err := validateContainedRecoveryOffer(offer, "history"); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"legacy", "takeover", "retry", "store", "epoch", "challenge", "endpoint", "destination", "token"} {
		t.Run(change, func(t *testing.T) {
			bad := proto.Clone(offer).(*pb.LifecycleOriginalRecovery)
			switch change {
			case "legacy":
				bad.Legacy = true
			case "takeover":
				bad.Takeover = true
			case "retry":
				bad.Operation.LegacyProcessIdentity = ""
				bad.Operation.Receipt = "old-receipt"
			case "store":
				bad.Operation.Source.Store = "other-store"
			case "epoch":
				bad.Operation.Owner.Epoch = ""
			case "challenge":
				bad.Challenge = ""
			case "endpoint":
				bad.Selection.Endpoint = "192.0.2.1:1234"
			case "destination":
				bad.Operation.Destination.Host = "other"
			case "token":
				bad.Selection.TokenFile = ""
			}
			if err := validateContainedRecoveryOffer(bad, "history"); err == nil {
				t.Fatal("accepted invalid offer")
			}
		})
	}
	for _, change := range []string{"identity", "endpoint", "token"} {
		t.Run("selected/"+change, func(t *testing.T) {
			selected := DevHostSelection{Identity: proto.Clone(dest).(*pb.LifecycleIdentity), Endpoint: offer.Selection.Endpoint, TokenFile: offer.Selection.TokenFile}
			switch change {
			case "identity":
				selected.Identity.Host = "other"
			case "endpoint":
				selected.Endpoint = "127.0.0.1:5678"
			case "token":
				selected.TokenFile = "other"
			}
			m := New(nil, false).WithDevHost(selected, "fixture")
			next, cmd := m.offerForcedResume(&forcedResumeOffer{destination: selected, original: offer})
			if cmd != nil || next.(Model).pendingConfirm != nil || next.(Model).errMsg == "" {
				t.Fatal("mismatched destination prompted or executed")
			}
		})
	}
}
