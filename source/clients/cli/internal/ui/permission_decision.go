package ui

import (
	"context"
	"fmt"
	"time"

	"cercano/source/server/pkg/agentclient"
	tea "charm.land/bubbletea/v2"
)

type permissionDecisionMsg struct {
	allow, persist bool
	err            error
}

func permissionDecisionCmd(ag *agentclient.Client, conv, tool string, allow, persist bool) tea.Cmd {
	return func() tea.Msg {
		result := permissionDecisionMsg{allow: allow, persist: persist}
		if ag == nil {
			result.err = fmt.Errorf("agent is not connected")
			return result
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if allow {
			result.err = ag.AllowToolCallPersist(ctx, conv, tool, persist)
		} else {
			result.err = ag.DenyToolCall(ctx, conv, tool)
		}
		return result
	}
}
