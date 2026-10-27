package ui

import (
	"context"
	"fmt"
	"time"

	"cercano/source/server/pkg/agentclient"
	tea "charm.land/bubbletea/v2"
)

// Provenance is captured when scheduling the RPC, not when its result is
// reduced. The subscription generation is also the conversation/attachment
// lifetime fence and is preserved across development-host model rebuilds.
type permissionDecisionMsg struct {
	client         *agentclient.Client
	conversation   string
	generation     uint64
	allow, persist bool
	err            error
}

func permissionDecisionCmd(ag *agentclient.Client, conv, tool string, generation uint64, allow, persist bool) tea.Cmd {
	return func() tea.Msg {
		result := permissionDecisionMsg{client: ag, conversation: conv, generation: generation, allow: allow, persist: persist}
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
