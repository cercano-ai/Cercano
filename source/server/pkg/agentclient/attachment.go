package agentclient

import (
	"context"
	"errors"
	"io"
	"time"

	"cercano/source/server/pkg/proto"
)

type turnVersion struct {
	epoch      string
	generation uint64
}

func (c *Client) observeTurn(parent string, response *proto.StreamProcessResponse) {
	if response.GetTurnEpoch() == "" {
		return
	}
	c.observationMu.Lock()
	defer c.observationMu.Unlock()
	if c.observedTurns == nil {
		c.observedTurns = make(map[string]turnVersion)
	}
	old := c.observedTurns[parent]
	if old.epoch != response.TurnEpoch || old.generation <= response.TurnGeneration {
		c.observedTurns[parent] = turnVersion{response.TurnEpoch, response.TurnGeneration}
	}
}

// IsCurrentConversationEvent fences stale approval requests and live indicators.
// Historical output may still be displayed in its own turn, never in a newer one.
func (c *Client) IsCurrentConversationEvent(parent string, response *proto.StreamProcessResponse) bool {
	c.observationMu.Lock()
	defer c.observationMu.Unlock()
	current := c.observedTurns[parent]
	return current.epoch == response.GetTurnEpoch() && current.generation == response.GetTurnGeneration()
}

type ConversationUpdate struct {
	Response *proto.StreamProcessResponse
	Message  *StreamMsg
	Err      error
}

// AttachConversation observes without owning or canceling execution. It resumes
// the host's current replay buffer with a cursor; persisted older turns remain
// available through ResumeConversation rather than an unbounded client queue.
func (c *Client) AttachConversation(ctx context.Context, parent string) (<-chan ConversationUpdate, error) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-c.stopWatch:
			cancel()
		case <-ctx.Done():
		}
	}()
	request := &proto.AttachConversationRequest{ConversationId: parent}
	c.connMu.Lock()
	agent := c.agent
	c.connMu.Unlock()
	stream, err := agent.AttachConversation(ctx, request)
	if err != nil {
		cancel()
		return nil, err
	}
	if c.pinned != nil {
		// Opening an HTTP/2 stream is not server-side enrollment. Wait for the
		// explicit acknowledgement so immediate intentional release cannot race it.
		header, headerErr := stream.Header()
		if headerErr != nil || len(header.Get("cercano-terminal-attached")) != 1 || header.Get("cercano-terminal-attached")[0] != "true" {
			cancel()
			if headerErr != nil {
				return nil, headerErr
			}
			return nil, errors.New("host did not acknowledge terminal attachment")
		}
	}
	out := make(chan ConversationUpdate, 8)
	go func() {
		defer close(out)
		defer cancel()
		send := func(update ConversationUpdate) bool {
			select {
			case out <- update:
				return true
			case <-ctx.Done():
				return false
			}
		}
		for {
			if ctx.Err() != nil {
				return
			}
			if stream == nil {
				c.connMu.Lock()
				agent := c.agent
				c.connMu.Unlock()
				stream, err = agent.AttachConversation(ctx, request)
			}
			var response *proto.StreamProcessResponse
			if err == nil {
				response, err = stream.Recv()
			}
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				if isUnavailable(err) || errors.Is(err, io.EOF) {
					stream = nil
					err = nil
					timer := time.NewTimer(100 * time.Millisecond)
					select {
					case <-timer.C:
					case <-ctx.Done():
						timer.Stop()
						return
					}
					continue
				}
				send(ConversationUpdate{Err: err})
				return
			}
			c.observeTurn(parent, response)
			if response.EventSequence > 0 && response.TurnEpoch == request.AfterTurnEpoch && response.TurnGeneration == request.AfterTurnGeneration && response.EventSequence <= request.AfterEventSequence {
				continue
			}
			if !send(ConversationUpdate{Response: response, Message: decodeStreamResponse(response)}) {
				return
			}
			if response.EventSequence > 0 {
				request.AfterTurnEpoch = response.TurnEpoch
				request.AfterTurnGeneration = response.TurnGeneration
				request.AfterEventSequence = response.EventSequence
			}
		}
	}()
	return out, nil
}
