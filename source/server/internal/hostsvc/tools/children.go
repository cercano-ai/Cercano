package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"cercano/source/server/internal/capabilities"
)

// childResults tracks only launches accepted by this loop, not other tasks in
// the chat or descendants owned by a child. Inspection retains exact ownership
// checks in the task service (also across the worker boundary).
type childResults struct {
	mu     sync.Mutex
	ids    []string
	tasks  capabilities.Tasks
	parent string
}

func (c *childResults) accepted(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id == "" {
		return
	}
	for _, existing := range c.ids {
		if existing == id {
			return
		}
	}
	c.ids = append(c.ids, id)
}
func (c *childResults) drain(ctx context.Context, join bool) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		c.mu.Lock()
		ids := append([]string(nil), c.ids...)
		c.mu.Unlock()
		if len(ids) == 0 {
			return "", nil
		}
		if c.tasks == nil {
			return "", fmt.Errorf("dispatch: child task service unavailable")
		}
		var outcomes []json.RawMessage
		completed := map[string]bool{}
		for _, id := range ids {
			rec, err := c.tasks.Inspect(ctx, c.parent, id)
			if err != nil {
				return "", err
			}
			if rec.Terminal() {
				data, err := json.Marshal(rec)
				if err != nil {
					return "", err
				}
				outcomes = append(outcomes, data)
				completed[id] = true
			}
		}
		if len(outcomes) > 0 {
			c.mu.Lock()
			remaining := c.ids[:0]
			for _, id := range c.ids {
				if !completed[id] {
					remaining = append(remaining, id)
				}
			}
			c.ids = remaining
			c.mu.Unlock()
			data, _ := json.Marshal(outcomes)
			return "Immediate child outcomes (child-authored claims are not independent verification). Process these results before completing your task:\n" + string(data), nil
		}
		if !join {
			return "", nil
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

// An aborted parent must not leave independently launched children orphaned.
// Stop is a request, not a wait: shutdown/join remains with the task owner, so
// a cancelled provider cannot deadlock cleanup behind an unresponsive child.
func (c *childResults) stop(ctx context.Context) {
	if c.tasks == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	c.mu.Lock()
	ids := append([]string(nil), c.ids...)
	c.mu.Unlock()
	for _, id := range ids {
		_, _ = c.tasks.Stop(ctx, c.parent, id)
	}
}
