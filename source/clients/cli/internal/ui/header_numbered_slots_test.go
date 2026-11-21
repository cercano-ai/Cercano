package ui

import (
	"strings"
	"testing"

	"cercano/source/clients/cli/internal/theme"
	proto "cercano/source/server/pkg/proto"
)

func TestHeaderNumberedSlotsWithoutLocal(t *testing.T) {
	m := Model{styles: theme.NewStyles(theme.Cracker()), activeCloudProfile: "primary-account", lastModel: "last-served-cloud"}
	out := stripAnsiCSI(m.renderHeaderRight(120))
	if !strings.Contains(out, "1:primary-account") || !strings.Contains(out, "2:—") || strings.Contains(out, "3:") || strings.Contains(out, "last-served") {
		t.Fatalf("want configured primary, unconfigured secondary, and no local slot; got %q", out)
	}
}

func TestHeaderNumberedSlotsConfigTransport(t *testing.T) {
	stub, client := newScenario(t)
	stub.mu.Lock()
	stub.active = "legacy-active-not-primary"
	stub.routing = &proto.RoutingAssignments{Primary: "team-external", Secondary: "personal"}
	stub.mu.Unlock()
	msg := fetchConfigCmd(client)().(configLoadedMsg)
	if !msg.SlotsLoaded || msg.ActiveCloudProfile != "team-external" || msg.SecondaryCloudProfile != "personal" {
		t.Fatalf("routing assignments lost in config transport: %+v", msg)
	}
	m := Model{styles: theme.NewStyles(theme.Cracker()), openModel: "old-local", lastModel: "last-served"}
	updated, _ := m.Update(msg)
	m = updated.(Model)
	out := stripAnsiCSI(m.renderHeaderRight(120))
	if out != "1:team-external │ 2:personal" {
		t.Fatalf("unexpected slots: %q", out)
	}
	updated, _ = m.Update(configLoadedMsg{SlotsLoaded: true, ActiveCloudProfile: "personal", SecondaryCloudProfile: "team-external", OpenModel: "qwen3:8b"})
	m = updated.(Model)
	out = stripAnsiCSI(m.renderHeaderRight(120))
	if out != "1:personal │ 2:team-external │ 3:qwen3:8b" {
		t.Fatalf("unexpected swapped/local slots: %q", out)
	}
	updated, _ = m.Update(configLoadedMsg{Failed: true})
	if got := stripAnsiCSI(updated.(Model).renderHeaderRight(120)); got != out {
		t.Fatalf("failed refresh cleared slots: %q", got)
	}
	updated, _ = m.Update(configLoadedMsg{SlotsLoaded: true})
	if got := stripAnsiCSI(updated.(Model).renderHeaderRight(120)); got != "1:— │ 2:—" {
		t.Fatalf("removed slots remain: %q", got)
	}
}
