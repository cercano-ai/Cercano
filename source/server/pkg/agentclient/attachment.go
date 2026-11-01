package agentclient

import (
	"context"
	"errors"
	"io"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/metadata"
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
	Local    bool // This turn also arrives on this client's initiating request stream.
	Response *proto.StreamProcessResponse
	Message  *StreamMsg
	Err      error
}

// AttachConversation observes without owning or canceling execution. It resumes
// the host's current replay buffer with a cursor; persisted older turns remain
// available through ResumeConversation rather than an unbounded client queue.
func (c *Client) AttachConversation(ctx context.Context, parent string) (<-chan ConversationUpdate, error) {
	return c.attachConversation(ctx, parent, nil)
}

func (c *Client) attachConversation(ctx context.Context, parent string, firstMetadata metadata.MD) (<-chan ConversationUpdate, error) {
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
	firstCtx := ctx
	if len(firstMetadata) != 0 {
		md, _ := metadata.FromOutgoingContext(ctx)
		firstCtx = metadata.NewOutgoingContext(ctx, metadata.Join(md, firstMetadata))
	}
	stream, err := agent.AttachConversation(firstCtx, request)
	if err != nil {
		cancel()
		return nil, err
	}
	if c.pinned != nil {
		// Opening an HTTP/2 stream is not server-side enrollment. Wait for the
		// explicit acknowledgement so immediate intentional release cannot race it.
		header, headerErr := stream.Header()
		if headerErr != nil || len(header.Get("cercano-terminal-attached")) != 1 || header.Get("cercano-terminal-attached")[0] != "true" {
			if headerErr == nil {
				// A trailers-only rejection can yield empty headers with no error.
				// Read its status before canceling so structured conflict details
				// survive the real HTTP/2 boundary and can offer explicit takeover.
				_, headerErr = stream.Recv()
			}
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
				if err == nil && c.pinned != nil {
					header, headerErr := stream.Header()
					err = headerErr
					if err == nil && (len(header.Get("cercano-terminal-attached")) != 1 || header.Get("cercano-terminal-attached")[0] != "true") {
						_, err = stream.Recv()
						if err == nil {
							err = errors.New("host did not acknowledge terminal attachment")
						}
					}
				}
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
			local, ok := c.localConversationEvent(ctx, parent, response)
			if !ok {
				return
			}
			if !send(ConversationUpdate{Local: local, Response: response, Message: decodeStreamResponse(response)}) {
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

// Register before opening the RPC: the observer may receive the first event
// before the initiating stream does. Wait for that stream's version rather than
// treating every foreground turn (including an inherited one) as local.
// Keep the small version identities for the client's lifetime: a completed
// request can still be replayed after the observation stream reconnects.
// One request may span multiple continuation generations.
type localRequest struct {
	changed  chan struct{}
	versions map[turnVersion]bool
	latest   turnVersion
	ended    bool
}

func (c *Client) beginLocalRequest(parent string) *localRequest {
	c.observationMu.Lock()
	defer c.observationMu.Unlock()
	if c.localRequests == nil {
		c.localRequests = make(map[string][]*localRequest)
	}
	r := &localRequest{changed: make(chan struct{}), versions: make(map[turnVersion]bool)}
	c.localRequests[parent] = append(c.localRequests[parent], r)
	return r
}
func (c *Client) resolveLocalRequest(r *localRequest, response *proto.StreamProcessResponse) {
	c.observationMu.Lock()
	defer c.observationMu.Unlock()
	if r.ended {
		return
	}
	if response == nil {
		r.ended = true
	} else {
		version := turnVersion{response.TurnEpoch, response.TurnGeneration}
		// Permission responses carry the version on the nested payload.
		if prompt := response.GetPermissionRequired(); version.epoch == "" && prompt != nil {
			version = turnVersion{prompt.TurnEpoch, prompt.TurnGeneration}
		}
		if version.epoch == "" || r.versions[version] {
			return
		}
		r.versions[version] = true
		r.latest = version
	}
	close(r.changed)
	r.changed = make(chan struct{})
}
func (c *Client) localConversationEvent(ctx context.Context, parent string, response *proto.StreamProcessResponse) (bool, bool) {
	if response.BackgroundTurn || response.TurnEpoch == "" {
		return false, true
	}
	c.observationMu.Lock()
	requests := append([]*localRequest(nil), c.localRequests[parent]...)
	c.observationMu.Unlock()
	version := turnVersion{response.TurnEpoch, response.TurnGeneration}
	for _, r := range requests {
		for {
			c.observationMu.Lock()
			local := r.versions[version]
			finished := r.ended || (r.latest.epoch == version.epoch && r.latest.generation > version.generation)
			changed := r.changed
			c.observationMu.Unlock()
			if local {
				return true, true
			}
			if finished {
				break
			}
			select {
			case <-changed:
			case <-ctx.Done():
				return false, false
			}
		}
	}
	return false, true
}
