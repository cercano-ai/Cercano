package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cercano/source/clients/cli/internal/slash"
	"cercano/source/clients/cli/internal/theme"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
)

type fakeVersionSource struct {
	endpoint string
	id       *proto.GetDevelopmentHostIdentityResponse
	err      error
	calls    int
}

func (f *fakeVersionSource) DevelopmentHostConnection() agentclient.DevelopmentHostConnection {
	return agentclient.DevelopmentHostConnection{Endpoint: f.endpoint}
}
func (f *fakeVersionSource) ReadDevelopmentHostIdentity(ctx context.Context, _ agentclient.DevelopmentHostConnection) (*proto.GetDevelopmentHostIdentityResponse, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("unbounded request")
	}
	f.calls++
	return f.id, f.err
}
func versionTestModel() (Model, *fakeVersionSource) {
	f := &fakeVersionSource{endpoint: "host-a", id: &proto.GetDevelopmentHostIdentityResponse{HostInstance: "instance-a", Build: "host-build", Contract: "contract", SchemaVersion: 1}}
	m := (Model{styles: theme.NewStyles(theme.Cracker()), palette: theme.Cracker()}).WithVersion("tui-build")
	m.width = 120
	m.height = 40
	m.versionSource = f
	return m, f
}
func TestVersionIdentityCurrentConnection(t *testing.T) {
	m, f := versionTestModel()
	model, _ := m.openVersion()
	m = model.(Model)
	v := m.content.(*versionPage)
	cmd := m.fetchVersion(v)
	if f.calls != 0 {
		t.Fatal("fetch blocked UI")
	}
	msg := cmd().(versionIdentityMsg)
	model, _ = m.updateVersionIdentity(msg)
	m = model.(Model)
	for _, s := range []string{"RUNNING TUI build: vtui-build", "Build: host-build", "Instance: instance-a", "Endpoint: host-a"} {
		if !strings.Contains(v.View(), s) {
			t.Fatalf("missing %q: %s", s, v.View())
		}
	}
	f.endpoint = "host-b"
	f.id.Build = "new-host-build"
	model, cmd = m.updateVersionIdentity(msg)
	m = model.(Model)
	if strings.Contains(v.View(), "Build: host-build") {
		t.Fatal("stale identity retained")
	}
	if cmd == nil {
		t.Fatal("connection move did not fetch")
	}
	model, _ = m.updateVersionIdentity(cmd().(versionIdentityMsg))
	m = model.(Model)
	if !strings.Contains(v.View(), "new-host-build") || !strings.Contains(v.View(), "tui-build") || m.splash.Meta.Version != "vtui-build" {
		t.Fatal(v.View())
	}
}
func TestVersionIdentityUnavailable(t *testing.T) {
	for _, fail := range []bool{false, true} {
		m, f := versionTestModel()
		f.id = &proto.GetDevelopmentHostIdentityResponse{}
		if fail {
			f.err = errors.New("unsupported")
		}
		model, _ := m.openVersion()
		m = model.(Model)
		v := m.content.(*versionPage)
		m.updateVersionIdentity(m.fetchVersion(v)().(versionIdentityMsg))
		if !strings.Contains(v.View(), "Build: unavailable") || strings.Contains(v.View(), "host-build") {
			t.Fatal(v.View())
		}
	}
}
func TestVersionLabelInputAndRegistry(t *testing.T) {
	m, _ := versionTestModel()
	start := headerTextWidth("▓▓ CERCANO ")
	end := start + headerTextWidth("vtui-build")
	if !strings.Contains(stripAnsiCSI(m.renderHeader()), "▓▓ CERCANO vtui-build") {
		t.Fatal("header changed")
	}
	for x := start - 1; x <= end; x++ {
		model, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: 0})
		_, opened := model.(Model).content.(*versionPage)
		want := x >= start && x < end
		if opened != want {
			t.Fatalf("x=%d opened=%t want=%t", x, opened, want)
		}
		if want && cmd == nil {
			t.Fatal("missing async fetch")
		}
	}
	r := slash.New()
	slash.RegisterVersion(r)
	if len(r.All()) != 1 || r.All()[0].Handler(nil).Kind != slash.ResultOpenVersion {
		t.Fatal("registry")
	}
}

func TestVersionPollRefreshAndClosedResponse(t *testing.T) {
	m, f := versionTestModel()
	model, _ := m.openVersion()
	m = model.(Model)
	v := m.content.(*versionPage)
	old := m.fetchVersion(v)().(versionIdentityMsg)
	m.updateVersionIdentity(old)
	f.endpoint = "host-b"
	_, cmd := m.pollVersion(versionPollMsg{v})
	if cmd == nil || v.connection.Endpoint != "host-b" || !strings.Contains(v.View(), "loading") {
		t.Fatal("poll did not invalidate moved connection")
	}
	m.updateVersionIdentity(old)
	if strings.Contains(v.View(), "host-build") {
		t.Fatal("old request accepted")
	}
	m.content = nil
	_, cmd = m.updateVersionIdentity(old)
	if cmd != nil {
		t.Fatal("closed page restarted work")
	}
}
