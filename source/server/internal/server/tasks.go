package server

import (
	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/taskrun"
	"context"
	"errors"
)

// serverTasks resolves the startup-owned manager without creating one on calls.
// A missing manager is an integration error, not a synchronous fallback.
type serverTasks struct{ s *Server }

var _ capabilities.Tasks = serverTasks{}

func (x serverTasks) manager() (*taskrun.Manager, error) {
	x.s.taskRunMu.RLock()
	defer x.s.taskRunMu.RUnlock()
	if x.s.taskManager == nil {
		return nil, errors.New("independent task manager unavailable")
	}
	return x.s.taskManager, nil
}
func (x serverTasks) Launch(ctx context.Context, spec dispatch.Spec) (conversation.TaskRecord, error) {
	m, err := x.manager()
	if err != nil {
		return conversation.TaskRecord{}, err
	}
	task, err := m.Launch(ctx, spec)
	if conversation.IsMutationFailure(err) {
		persistsvc.ReportWriteFailure(ctx, err)
	}
	return task, err
}
func (x serverTasks) List(ctx context.Context, parent string, limit int) ([]conversation.TaskRecord, error) {
	m, err := x.manager()
	if err != nil {
		return nil, err
	}
	return m.List(ctx, parent, limit)
}
func (x serverTasks) Inspect(ctx context.Context, parent, id string) (conversation.TaskRecord, error) {
	m, err := x.manager()
	if err != nil {
		return conversation.TaskRecord{}, err
	}
	return m.Inspect(ctx, parent, id)
}
func (x serverTasks) Wait(ctx context.Context, parent, id string) (conversation.TaskRecord, error) {
	m, err := x.manager()
	if err != nil {
		return conversation.TaskRecord{}, err
	}
	return m.Wait(ctx, parent, id)
}
func (x serverTasks) Stop(ctx context.Context, parent, id string) (conversation.TaskRecord, error) {
	m, err := x.manager()
	if err != nil {
		return conversation.TaskRecord{}, err
	}
	return m.Stop(ctx, parent, id)
}
