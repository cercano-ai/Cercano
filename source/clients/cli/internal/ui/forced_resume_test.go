package ui

import (
	"cercano/source/server/pkg/proto"
	"testing"
)

func TestForcedResumeConfirmationAndStaleCallbacks(t *testing.T) {
	for _, key := range []string{"enter", "n", "esc", "y"} {
		t.Run(key, func(t *testing.T) {
			a := DevHostSelection{Identity: &proto.LifecycleIdentity{Host: "A"}, Conversation: "history", Epoch: "old"}
			b := DevHostSelection{Identity: &proto.LifecycleIdentity{Host: "B"}, Conversation: "current", Epoch: "new"}
			m := New(nil, false).WithDevHost(b, "test")
			offered, cmd := m.offerForcedResume(&forcedResumeOffer{source: a, destination: b, owner: &proto.LifecycleOwner{Conversation: "history", Host: "A", Epoch: "old"}})
			m = offered.(Model)
			if cmd != nil || m.pendingConfirm == nil {
				t.Fatal("offer mutated without confirmation")
			}
			m, cmd = m.resolveConfirmKey(key)
			if (cmd != nil) != (key == "y") {
				t.Fatalf("key %s command=%v", key, cmd != nil)
			}
			if m.devHost.selection.Identity.Host != "B" || m.convID != "current" {
				t.Fatal("offer rebound to old server")
			}
		})
	}
	m := New(nil, false).WithDevHost(DevHostSelection{Identity: &proto.LifecycleIdentity{Host: "B"}}, "test")
	offered, _ := m.offerForcedResume(&forcedResumeOffer{source: DevHostSelection{}, destination: m.devHost.selection, owner: &proto.LifecycleOwner{Conversation: "history", Host: "A", Epoch: "old"}})
	m = offered.(Model)
	callback := m.pendingConfirm.onYes
	m.taskSubscriptionGeneration++
	_, cmd := callback(m)
	if cmd != nil {
		t.Fatal("stale confirmation cancelled work")
	}
}

func verifyProductionForcedResume(t *testing.T, offer *forcedResumeOffer) {
	t.Helper()
	m := New(nil, false).WithDevHost(offer.destination, "acceptance")
	// Declining never sends the destructive command or switches to A.
	for _, key := range []string{"enter", "n", "esc"} {
		next, _ := m.offerForcedResume(offer)
		m = next.(Model)
		declined, action := m.resolveConfirmKey(key)
		m = declined
		if action != nil || m.devHost.selection.Identity.Host != offer.destination.Identity.Host {
			t.Fatal("decline changed host")
		}
	}
	next, _ := m.offerForcedResume(offer)
	m = next.(Model)
	_, action := m.resolveConfirmKey("y")
	if action == nil {
		t.Fatal("missing force command")
	}
	result := action().(devHostResult)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.client == nil || result.selection == nil {
		t.Fatal("missing validated destination client")
	}
	defer result.client.Close()
	if result.selection.Identity.Host != offer.destination.Identity.Host || result.selection.Epoch == offer.owner.Epoch {
		t.Fatal("forced resume stayed on source")
	}
	applied, _ := m.applyDevHost(result)
	fresh := applied.(Model)
	if fresh.devHost.selection.Identity.Host != offer.destination.Identity.Host || fresh.convID != offer.owner.Conversation {
		t.Fatal("UI not rebound to B")
	}
	if fresh.devHost.defaultSelection.Identity.Host != offer.destination.Identity.Host {
		t.Fatal("default host lost")
	}
}
