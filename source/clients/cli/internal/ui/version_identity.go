package ui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type developmentIdentitySource interface {
	DevelopmentHostConnection() agentclient.DevelopmentHostConnection
	ReadDevelopmentHostIdentity(context.Context, agentclient.DevelopmentHostConnection) (*proto.GetDevelopmentHostIdentityResponse, error)
}
type versionPage struct {
	build, detail string
	width, height int
	source        developmentIdentitySource
	connection    agentclient.DevelopmentHostConnection
	request       uint64
}

func (v *versionPage) ID() contentPageID { return "version" }
func (v *versionPage) SetSize(w, h int)  { v.width, v.height = w, h }
func (v *versionPage) Update(k tea.KeyPressMsg) (tea.Cmd, bool) {
	return nil, k.String() == "esc" || k.String() == "q"
}
func identityText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if strings.TrimSpace(s) == "" {
		return "unavailable"
	}
	return s
}
func (v *versionPage) View() string {
	text := fmt.Sprintf("Version identity\n\nRUNNING TUI build: %s\n\nConnected development agent host\nEndpoint: %s\n%s\n\nEsc / q: back", identityText(v.build), identityText(v.connection.Endpoint), v.detail)
	if v.width > 0 {
		text = ansi.Wrap(text, v.width, "")
	}
	if v.height > 0 {
		lines := strings.Split(text, "\n")
		if len(lines) > v.height {
			text = strings.Join(lines[:v.height], "\n")
		}
	}
	return text
}

type versionIdentityMsg struct {
	page     *versionPage
	request  uint64
	identity *proto.GetDevelopmentHostIdentityResponse
	err      error
}
type versionPollMsg struct{ page *versionPage }

func versionPoll(v *versionPage) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return versionPollMsg{v} })
}
func (m Model) identitySource() developmentIdentitySource {
	if m.versionSource != nil {
		return m.versionSource
	}
	return m.agent
}
func (m Model) mouseInVersion(x, y int) bool {
	start := headerTextWidth("▓▓ CERCANO ")
	end := start + headerTextWidth(m.splash.Meta.Version)
	return y == 0 && x >= start && x < end && x < m.width
}
func (m Model) openVersion() (tea.Model, tea.Cmd) {
	v := &versionPage{build: m.splash.Meta.Version}
	m.configSurface = nil
	m.content = v
	v.SetSize(m.width, m.contentPageHeight())
	return m, tea.Batch(m.fetchVersion(v), versionPoll(v))
}
func (m Model) fetchVersion(v *versionPage) tea.Cmd {
	v.source = m.identitySource()
	v.connection = v.source.DevelopmentHostConnection()
	v.request++
	v.detail = "Build: loading…\nInstance: loading…"
	source, connection, request := v.source, v.connection, v.request
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		id, err := source.ReadDevelopmentHostIdentity(ctx, connection)
		return versionIdentityMsg{v, request, id, err}
	}
}
func (m Model) versionConnectionChanged(v *versionPage) bool {
	source := m.identitySource()
	return source != v.source || source.DevelopmentHostConnection() != v.connection
}
func (m Model) pollVersion(msg versionPollMsg) (tea.Model, tea.Cmd) {
	if m.content != msg.page {
		return m, nil
	}
	var fetch tea.Cmd
	if m.versionConnectionChanged(msg.page) {
		fetch = m.fetchVersion(msg.page)
	}
	return m, tea.Batch(fetch, versionPoll(msg.page))
}
func (m Model) updateVersionIdentity(msg versionIdentityMsg) (tea.Model, tea.Cmd) {
	v := msg.page
	if m.content != v || msg.request != v.request {
		return m, nil
	}
	if m.versionConnectionChanged(v) {
		return m, m.fetchVersion(v)
	}
	id := msg.identity
	if msg.err != nil || id == nil {
		v.detail = "Build: unavailable\nInstance: unavailable\nDevelopment host qualification: unavailable (server unsupported or unreachable)."
	} else if id.GetHostInstance() == "" || id.GetBuild() == "" || id.GetContract() == "" || id.GetSchemaVersion() == 0 {
		v.detail = "Build: unavailable\nInstance: unavailable\nDevelopment host qualification: unqualified (ordinary server or incomplete identity)."
	} else {
		v.detail = fmt.Sprintf("Build: %s\nInstance: %s\nContract: %s\nSchema: %d\nRetired: %t", identityText(id.GetBuild()), identityText(id.GetHostInstance()), identityText(id.GetContract()), id.GetSchemaVersion(), id.GetRetired())
	}
	return m, nil
}
