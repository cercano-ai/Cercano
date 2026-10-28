package ui

import (
	"context"
	"time"

	"cercano/source/server/pkg/agentclient"
	tea "charm.land/bubbletea/v2"
)

// Capture the attachment lifetime before scheduling any configuration or RPC work.
type shutdownDecisionMsg struct {
	client       *agentclient.Client
	conversation string
	generation   uint64
	text         string
}

func (m *Model) shutdownOrigin() shutdownDecisionMsg {
	return shutdownDecisionMsg{client: m.agent, conversation: m.convID, generation: m.taskSubscriptionGeneration}
}

func dispatchShutdown(origin shutdownDecisionMsg, reason string, config *agentclient.ConfigUpdate) tea.Cmd {
	return func() tea.Msg {
		result := origin
		result.text = "shutdown incomplete: request failed; automatic replacement blocked."
		if origin.client == nil {
			result.text = "shutdown request failed: agent is not connected."
			return result
		}
		if config != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err := origin.client.UpdateConfig(ctx, *config)
			cancel()
			if err != nil {
				// RPC messages may contain paths, credentials or provider output.
				result.text = "runtime configuration update failed; shutdown was not requested."
				return result
			}
		}
		if reason == "" {
			reason = "user-requested restart"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		response, err := origin.client.RequestShutdown(ctx, reason)
		if err == nil {
			switch {
			case !response.GetAccepted():
				result.text = "shutdown incomplete: request rejected; automatic replacement blocked."
			case !response.GetComplete():
				result.text = "shutdown accepted but incomplete; automatic replacement blocked."
			default:
				result.text = "shutdown complete; replacement is permitted when the connection drops."
			}
		}
		if config != nil {
			result.text = "Runtime configuration saved. " + result.text
		}
		return result
	}
}
