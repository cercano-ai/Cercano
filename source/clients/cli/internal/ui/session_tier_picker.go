package ui

import (
	"fmt"
	"strings"

	"cercano/source/server/pkg/agentclient"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type sessionTierPicker struct {
	convID  string
	cursor  int
	loading bool
	err     string
	status  agentclient.SessionModelStatus
}

func (m *Model) requestSessionModel(action, profile, model string, quiet bool) tea.Cmd {
	if m.sessionModelPending && m.sessionModelConv == m.convID {
		return nil
	}
	if m.convID == "" {
		m.convID = newConvID()
		if m.convRef != nil {
			m.convRef.id = m.convID
		}
	}
	m.sessionModelSerial++
	serial := m.sessionModelSerial
	if m.sessionModelConv != m.convID {
		m.sessionModelState = agentclient.SessionModelStatus{}
	}
	m.sessionModelConv = m.convID
	m.sessionModelPending = action == "set" || action == "tier" || action == "clear"
	cmd := sessionModelCmd(m.agent, m.convID, m.effectiveWorkDir(), action, profile, model, quiet)
	return func() tea.Msg { msg := cmd().(sessionModelMsg); msg.serial = serial; return msg }
}

func (m Model) activeSessionTierPicker() bool {
	return m.sessionTierPicker != nil && m.sessionTierPicker.convID == m.convID
}

func (m Model) cloudChipRange() (int, int, bool) {
	if m.cloudModel == "" && m.activeCloudProfile == "" && !(m.sessionModelConv == m.convID && m.sessionModelState.Effective != nil) {
		return 0, 0, false
	}
	plain := ansi.Strip(m.renderHeader())
	at := strings.LastIndex(plain, "1:")
	if at < 0 {
		return 0, 0, false
	}
	end := strings.Index(plain[at:], " │ ")
	if end < 0 {
		return 0, 0, false
	}
	start := ansi.StringWidth(plain[:at])
	return start, start + ansi.StringWidth(plain[at:at+end]), true
}

func (m *Model) openSessionTierPicker() tea.Cmd {
	m.sessionTierPicker = &sessionTierPicker{convID: m.convID, loading: true}
	cmd := m.requestSessionModel("status", "", "", true)
	m.sessionTierPicker.convID = m.convID
	return cmd
}

func (m Model) sessionTierBox() (string, int, int) {
	p := m.sessionTierPicker
	width := min(58, max(12, m.width))
	var lines []string
	if p.loading {
		lines = []string{"Loading chat tiers…"}
	} else if p.status.Effective == nil {
		lines = []string{"No cloud profile configured."}
	} else {
		currentMarked := false
		for i, tier := range p.status.Tiers {
			model := tier.Model
			if model == "" {
				model = "unavailable"
			}
			marker := "  "
			if i == p.cursor {
				marker = "› "
			}
			label := ansi.Truncate(marker+fmt.Sprintf("[%s : %s]", tier.Tier, model), max(1, width-4), "…")
			style := lipgloss.NewStyle().Foreground(m.palette.Primary)
			if !currentMarked && p.status.Effective.Model == tier.Model && tier.Model != "" {
				currentMarked = true
				style = style.Bold(true)
			}
			if i == p.cursor {
				style = style.Background(m.palette.SelectionBg)
			}
			lines = append(lines, style.Render(label))
		}
	}
	if m.sessionModelPending && m.sessionModelConv == m.convID {
		lines = append(lines, "Applying for next turn…")
	}
	if p.err != "" {
		lines = append(lines, ansi.Truncate(ansi.Strip(p.err), max(1, width-4), "…"))
	}
	lines = append(lines, "↑/↓ Tab: select · Enter: use · Esc: close")
	box := lipgloss.NewStyle().Background(m.palette.Surface).Foreground(m.palette.Primary).Border(lipgloss.RoundedBorder()).BorderForeground(m.palette.Accent).Padding(0, 1).Width(max(1, width-2)).Render(strings.Join(lines, "\n"))
	start, _, _ := m.cloudChipRange()
	return box, max(0, min(start, m.width-lipgloss.Width(box))), 1
}

func (m *Model) selectSessionTier() tea.Cmd {
	p := m.sessionTierPicker
	if p.loading || p.status.Effective == nil || p.cursor < 0 || p.cursor >= len(p.status.Tiers) {
		return nil
	}
	choice := p.status.Tiers[p.cursor]
	if choice.Model == "" {
		p.err = "This tier has no configured model."
		return nil
	}
	p.err = ""
	return m.requestSessionModel("tier", p.status.Effective.Profile, choice.Tier, false)
}

func (m *Model) sessionTierKey(key string) tea.Cmd {
	if key == "esc" {
		m.sessionTierPicker = nil
		return nil
	}
	p := m.sessionTierPicker
	if m.sessionModelPending || p.loading {
		return nil
	}
	n := len(p.status.Tiers)
	if n == 0 {
		return nil
	}
	switch key {
	case "up", "shift+tab":
		p.cursor = (p.cursor + n - 1) % n
	case "down", "tab":
		p.cursor = (p.cursor + 1) % n
	case "enter", " ", "space":
		return m.selectSessionTier()
	}
	return nil
}

func (m *Model) sessionTierClick(x, y int) tea.Cmd {
	box, left, top := m.sessionTierBox()
	if x < left || x >= left+lipgloss.Width(box) || y < top || y >= top+lipgloss.Height(box) {
		m.sessionTierPicker = nil
		return nil
	}
	row := y - top - 1
	p := m.sessionTierPicker
	if !p.loading && row >= 0 && row < len(p.status.Tiers) {
		p.cursor = row
		return m.selectSessionTier()
	}
	return nil
}

// A queued next turn must not race the route update acknowledgement.
func (m *Model) drainQueuedChat() (queuedTurn, bool) {
	if m.sessionModelPending && m.sessionModelConv == m.convID {
		return queuedTurn{}, false
	}
	return m.mainChat().DrainNext()
}
