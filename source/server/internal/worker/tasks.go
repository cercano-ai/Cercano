package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/capabilities/builtins"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

type TasksSetter interface{ SetTasks(capabilities.Tasks) }

func (w *workerRunner) SetTasks(tasks capabilities.Tasks) { w.tasks = tasks }

// The host assigns ownership from the authenticated worker turn. No parent ID
// or reserved child ID is accepted on the wire.
func handleTaskControl(ctx context.Context, tasks capabilities.Tasks, parent string, request *proto.TaskControlRequest) *proto.TaskControlResponse {
	response := &proto.TaskControlResponse{Id: request.GetId()}
	if parent == "" {
		response.Error = "trusted per-chat binding missing"
		return response
	}
	if tasks == nil {
		response.Error = "independent task service unavailable"
		return response
	}
	var result any
	var err error
	switch request.GetOperation() {
	case "launch":
		taskClass := config.Task(request.GetRoutingTask())
		if taskClass == "" {
			taskClass = config.TaskDispatch
		}
		if !config.ValidTask(taskClass) {
			response.Error = "invalid task class"
			return response
		}
		args, _ := json.Marshal(map[string]any{"tools": request.GetTools()})
		readOnly := builtins.Dispatch().(capabilities.ArgsTiered).TierFor(args) == capabilities.TierR
		if taskClass == config.TaskReconnaissance && !readOnly {
			response.Error = "reconnaissance requires read-only grants"
			return response
		}
		result, err = tasks.Launch(ctx, dispatch.Spec{ReadOnly: readOnly, Mode: dispatch.Agentic, Role: dispatch.RoleCoproc, ConversationID: parent, Task: request.GetTask(), Tools: request.GetTools(), WorkDir: request.GetWorkDir(), RoutingTask: taskClass, Tier: config.Tier(request.GetTier())})
	case "list":
		limit := int(request.GetLimit())
		if limit < 1 || limit > 100 {
			response.Error = "limit must be 1–100"
			return response
		}
		result, err = tasks.List(ctx, parent, limit)
	case "inspect":
		result, err = tasks.Inspect(ctx, parent, request.GetTaskId())
	case "stop":
		result, err = tasks.Stop(ctx, parent, request.GetTaskId())
	case "wait":
		ms := request.GetTimeoutMillis()
		if ms <= 0 || ms > 300000 {
			response.Error = "wait timeout must be 1–300000 milliseconds"
			return response
		}
		waitCtx, cancel := context.WithTimeout(ctx, time.Duration(ms)*time.Millisecond)
		defer cancel()
		result, err = tasks.Wait(waitCtx, parent, request.GetTaskId())
	default:
		response.Error = "unknown task operation"
		return response
	}
	if err == nil {
		response.ResultJson, err = json.Marshal(result)
	}
	if err != nil {
		// The wire cannot carry Go error identity. Normalize the sentinel to
		// its exact text so the worker-side bridge can translate it back for
		// the foreground tool contract; see streamTasks.call.
		if errors.Is(err, conversation.ErrTaskWaitInterrupted) {
			response.Error = conversation.ErrTaskWaitInterrupted.Error()
			return response
		}
		response.Error = err.Error()
		response.NotFound = errors.Is(err, conversation.ErrTaskNotFound)
		response.ResultJson = nil
	}
	return response
}

type streamTasks struct {
	sndr    *sender
	parent  string
	next    atomic.Uint64
	mu      sync.Mutex
	pending map[string]chan *proto.TaskControlResponse
}

var _ capabilities.Tasks = (*streamTasks)(nil)

func newStreamTasks(sndr *sender, parent string) *streamTasks {
	return &streamTasks{sndr: sndr, parent: parent, pending: make(map[string]chan *proto.TaskControlResponse)}
}
func (p *streamTasks) call(ctx context.Context, parent string, req *proto.TaskControlRequest, out any) error {
	if parent == "" || parent != p.parent {
		return fmt.Errorf("task control is scoped to the active conversation")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	req.Id = strconv.FormatUint(p.next.Add(1), 10)
	ch := make(chan *proto.TaskControlResponse, 1)
	p.mu.Lock()
	p.pending[req.Id] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, req.Id); p.mu.Unlock() }()
	p.sndr.send(&proto.WorkerToHost{Msg: &proto.WorkerToHost_TaskControlRequest{TaskControlRequest: req}})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case response := <-ch:
		if response.GetNotFound() {
			return conversation.ErrTaskNotFound
		}
		if response.GetError() != "" {
			// Exact-text match with the host-side normalization in
			// handleTaskControl restores Go error identity for the sentinel
			// that a same-chat sibling finished during the wait.
			if response.GetError() == conversation.ErrTaskWaitInterrupted.Error() {
				return conversation.ErrTaskWaitInterrupted
			}
			return errors.New(response.GetError())
		}
		return json.Unmarshal(response.GetResultJson(), out)
	}
}
func (p *streamTasks) deliver(response *proto.TaskControlResponse) {
	p.mu.Lock()
	ch := p.pending[response.GetId()]
	p.mu.Unlock()
	if ch != nil {
		select {
		case ch <- response:
		default:
		}
	}
}
func (p *streamTasks) Launch(ctx context.Context, spec dispatch.Spec) (record conversation.TaskRecord, err error) {
	err = p.call(ctx, spec.ConversationID, &proto.TaskControlRequest{Operation: "launch", Task: spec.Task, Tools: spec.Tools, WorkDir: spec.WorkDir, RoutingTask: string(spec.RoutingTask), Tier: string(spec.Tier)}, &record)
	return
}
func (p *streamTasks) List(ctx context.Context, parent string, limit int) (records []conversation.TaskRecord, err error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("limit must be 1–100")
	}
	err = p.call(ctx, parent, &proto.TaskControlRequest{Operation: "list", Limit: int32(limit)}, &records)
	return
}
func (p *streamTasks) Inspect(ctx context.Context, parent, id string) (record conversation.TaskRecord, err error) {
	err = p.call(ctx, parent, &proto.TaskControlRequest{Operation: "inspect", TaskId: id}, &record)
	return
}
func (p *streamTasks) Stop(ctx context.Context, parent, id string) (record conversation.TaskRecord, err error) {
	err = p.call(ctx, parent, &proto.TaskControlRequest{Operation: "stop", TaskId: id}, &record)
	return
}
func (p *streamTasks) Wait(ctx context.Context, parent, id string) (record conversation.TaskRecord, err error) {
	timeout := 300 * time.Second
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < timeout {
		timeout = time.Until(deadline)
	}
	if timeout <= 0 {
		return record, context.DeadlineExceeded
	}
	ms := (timeout + time.Millisecond - 1) / time.Millisecond
	err = p.call(ctx, parent, &proto.TaskControlRequest{Operation: "wait", TaskId: id, TimeoutMillis: int64(ms)}, &record)
	return
}
