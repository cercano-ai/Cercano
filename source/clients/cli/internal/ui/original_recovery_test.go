package ui

import (
	pb "cercano/source/server/pkg/proto"
	"strings"
	"testing"
)

// Exercise the production confirmation queue and key handling, never auth files
// or live RPC. The server fixture separately exercises the consent RPC journey.
func TestOriginalRecoveryUIConfirmation(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, key := range []string{"enter", "n", "esc", "y"} {
			t.Run(key+map[bool]string{false: "/retry", true: "/legacy"}[legacy], func(t *testing.T) {
				source := &pb.LifecycleIdentity{Host: "S", Build: "build", Store: "store", Contract: "contract", Schema: 1}
				dest := &pb.LifecycleIdentity{Host: "D", Build: "build", Store: "store", Contract: "contract", Schema: 1}
				o := &pb.LifecycleOriginalRecovery{Challenge: "single-use", Legacy: legacy, Selection: &pb.LifecycleRecoverySelection{Identity: dest, Endpoint: "127.0.0.1:1234", TokenFile: "fixture-not-read"}, Operation: &pb.LifecycleControlRequest{Target: source, Source: source, Destination: dest, Owner: &pb.LifecycleOwner{Conversation: "history", Host: "S", Epoch: "epoch"}, HandoverId: "op", Receipt: "receipt"}}
				if err := validateOriginalOffer(o, "history"); err != nil {
					t.Fatal(err)
				}
				selected := DevHostSelection{Identity: &pb.LifecycleIdentity{Host: "N"}, Conversation: "current"}
				m := New(nil, false).WithDevHost(selected, "fixture")
				offered, cmd := m.offerForcedResume(&forcedResumeOffer{destination: selected, original: o})
				m = offered.(Model)
				if cmd != nil || m.pendingConfirm == nil {
					t.Fatal("offer bypassed explicit confirmation")
				}
				details := strings.Join(m.pendingConfirm.details, " ")
				if !strings.Contains(details, "fence the source") || !strings.Contains(details, "cancel conversation work") {
					t.Fatal("missing warning")
				}
				if legacy && !strings.Contains(details, "may create the missing source reservation") {
					t.Fatal("legacy semantics hidden")
				}
				stale := m
				stale.taskSubscriptionGeneration++
				if _, cmd := m.pendingConfirm.onYes(stale); cmd != nil {
					t.Fatal("stale approval reusable")
				}
				next, cmd := m.resolveConfirmKey(key)
				if (cmd != nil) != (key == "y") {
					t.Fatalf("key %s bypassed consent", key)
				}
				if next.devHost.selection.Identity.Host != "N" {
					t.Fatal("offer silently selected original host")
				}
			})
		}
	}
}

func TestLiveSourceRecoveryUI(t *testing.T) {
	source := &pb.LifecycleIdentity{Host: "S", Build: "build", Store: "store", Contract: "contract", Schema: 1}
	dest := &pb.LifecycleIdentity{Host: "D", Build: "build", Store: "store", Contract: "contract", Schema: 1}
	o := &pb.LifecycleOriginalRecovery{Challenge: "fresh", Selection: &pb.LifecycleRecoverySelection{Identity: dest, Endpoint: "127.0.0.1:1234", TokenFile: "never-read"}, Operation: &pb.LifecycleControlRequest{Target: source, Source: source, Destination: dest, Owner: &pb.LifecycleOwner{Conversation: "history", Host: "S", Epoch: "epoch"}, HandoverId: "stable", LegacyProcessIdentity: "snapshot"}}
	if e := validateOriginalOffer(o, "history"); e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"enter", "n", "esc", "y"} {
		t.Run(key, func(t *testing.T) {
			selected := DevHostSelection{Identity: dest, Conversation: "current"}
			m := New(nil, false).WithDevHost(selected, "fixture")
			offered, _ := m.offerForcedResume(&forcedResumeOffer{destination: selected, original: o})
			m = offered.(Model)
			details := strings.Join(m.pendingConfirm.details, " ")
			for _, text := range []string{"WHOLE old host", "sibling conversations and tasks", "NOT replayed", "single-use"} {
				if !strings.Contains(details, text) {
					t.Fatalf("missing disclosure %s: %s", text, details)
				}
			}
			stale := m
			stale.taskSubscriptionGeneration++
			if _, cmd := m.pendingConfirm.onYes(stale); cmd != nil {
				t.Fatal("stale UI consent")
			}
			_, cmd := m.resolveConfirmKey(key)
			if (cmd != nil) != (key == "y") {
				t.Fatal("default no bypassed")
			}
		})
	}
}
