package agentclient

import (
	"context"
	"errors"
	"io"
	"time"

	"cercano/source/server/pkg/proto"
)

type TaskUpdate struct {
	Event *proto.TaskSubscriptionEvent
	Err   error
}

// SubscribeTasks observes one parent's durable task state. Cursor advancement
// follows delivery into the bounded FIFO, so reconnect cannot skip a record
// still waiting for the caller. Canceling ctx only stops observation.
func (c *Client) SubscribeTasks(ctx context.Context, parent string) (<-chan TaskUpdate, error) {
	ctx, cancel := context.WithCancel(ctx)
	c.connMu.Lock()
	agent, failedConn := c.agent, c.conn
	c.connMu.Unlock()
	stream, err := agent.SubscribeTasks(ctx, &proto.SubscribeTasksRequest{ParentId: parent})
	recovered := false
	if err != nil && c.pinned != nil && pinnedTransportLost(err, failedConn) {
		if recoveryErr := c.reconnectPinnedTransport(ctx, failedConn); recoveryErr != nil {
			err = recoveryErr
		} else {
			recovered = true
			c.connMu.Lock()
			agent, failedConn = c.agent, c.conn
			c.connMu.Unlock()
			stream, err = agent.SubscribeTasks(ctx, &proto.SubscribeTasksRequest{ParentId: parent})
		}
	}
	if err != nil {
		cancel()
		return nil, err
	}
	out := make(chan TaskUpdate, 8)
	go func() {
		select {
		case <-c.stopWatch:
			cancel()
		case <-ctx.Done():
		}
	}()
	go func() {
		defer close(out)
		defer cancel()
		cursors := make(map[string]int64)
		send := func(update TaskUpdate) bool {
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
				request := &proto.SubscribeTasksRequest{ParentId: parent}
				for id, seq := range cursors {
					request.Cursors = append(request.Cursors, &proto.TaskProgressCursor{TaskId: id, Sequence: seq})
				}
				c.connMu.Lock()
				agent := c.agent
				failedConn = c.conn
				c.connMu.Unlock()
				stream, err = agent.SubscribeTasks(ctx, request)
			}
			var event *proto.TaskSubscriptionEvent
			if err == nil {
				event, err = stream.Recv()
			}
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				if c.pinned != nil && (pinnedTransportLost(err, failedConn) || errors.Is(err, io.EOF)) {
					if recovered {
						send(TaskUpdate{Err: err})
						return
					}
					if recoveryErr := c.reconnectPinnedTransport(ctx, failedConn); recoveryErr != nil {
						send(TaskUpdate{Err: recoveryErr})
						return
					}
					recovered = true
					stream, err = nil, nil
					continue
				}
				if isUnavailable(err) || errors.Is(err, io.EOF) {
					stream = nil
					// The client's standing connection watcher owns reconnection. A
					// chat-scoped observer must not mark the shared client failed merely
					// because its own cancellation interrupts a reconnect attempt.
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
				send(TaskUpdate{Err: err})
				return
			}
			recovered = false
			if !send(TaskUpdate{Event: event}) {
				return
			}
			if progress := event.GetProgress(); progress != nil {
				cursors[progress.TaskId] = progress.Sequence
			}
		}
	}()
	return out, nil
}
