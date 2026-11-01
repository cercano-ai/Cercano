package ui

import (
	"cercano/source/server/pkg/agentclient"
	tea "charm.land/bubbletea/v2"
	"context"
	"time"
)

// Metadata belongs to a particular hydration and client, not merely an ID:
// reopening the same conversation must not admit the previous client's reply.
type resumeMetadataMsg struct {
	client        *agentclient.Client
	convID        string
	gen           int
	titleRevision uint64
	info          agentclient.ConversationInfo
	err           error
}

func fetchResumeMetadata(client *agentclient.Client, id string, gen int, revision uint64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		info, err := client.GetConversation(ctx, id)
		return resumeMetadataMsg{client: client, convID: id, gen: gen, titleRevision: revision, info: info, err: err}
	}
}
