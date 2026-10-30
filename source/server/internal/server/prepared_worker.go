package server

import (
	toolssvc "cercano/source/server/internal/hostsvc/tools"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/worker"
)

// ConfigurePreparedWorker installs the pinned runner using only the prepared
// host's services. Call during startup, before exposing admission. Unlike the
// legacy selector this never constructs a discovery-based runner, even briefly.
// Host and worker descendants share exact per-dispatch scopes.
func (s *Server) ConfigurePreparedWorker(policy worker.SpawnPolicy) error {
	return s.configureLifecycleWorker(policy)
}

// configureLifecycleWorker uses the attached lifecycle and existing host services,
// independently of whether those services came from ordinary or prepared startup.
func (s *Server) configureLifecycleWorker(policy worker.SpawnPolicy) error {
	if s == nil || s.coexistence == nil || s.cfgSvc == nil || s.persistSvc == nil || s.permBroker == nil {
		return fmt.Errorf("pinned worker requires lifecycle and host services")
	}
	if s.workerRunner != nil {
		return fmt.Errorf("worker runner already configured")
	}
	if err := policy.ValidatePrepared(); err != nil {
		return err
	}
	r := worker.NewWorkerRunnerWithPolicy(policy, s.persistSvc, s.cfgSvc, s.permBroker,
		nil, // Prepared descendant admission is retained by the per-turn authority.
		func(ctx context.Context, id, name string) error {
			if _, err := s.resolvePreparedTaskOwner(ctx, id); err != nil {
				return err
			}
			return s.setSessionProfile(id, name)
		},
		func() inference.Provider { return s.OpenLLMProvider() }, s.openModels.Model, s.turnModelEvidence, s.restartRuntimeTool)
	r.(worker.TurnAuthoritySetter).SetTurnAuthority(func(ctx context.Context, id string) (worker.TurnAuthority, error) {
		cap, err := s.resolvePreparedTaskOwner(ctx, id)
		if err != nil {
			return nil, err
		}
		owner := ownerOf(cap)
		workctx, ticket, err := s.BeginRetainedWork(ctx, owner, broker.PersistenceWork)
		if err != nil {
			return nil, err
		}
		return &preparedWorkerAuthority{server: s, ctx: workctx, id: id, ticket: ticket}, nil
	})
	// Same live host bridges as SelectExecutionMode, applied directly to the
	// pinned runner. No legacy runner replacement or global context mutation.
	r.(worker.ProfileReaderSetter).SetProfileReader(func(id string) agent.Profile {
		if s.profileBroker == nil {
			return agent.Profile{}
		}
		return s.profileBroker.Active(id)
	})
	r.(worker.McpBridgeSetter).SetMCPBridge(s.advertiseMCPTools, s.callMCPTool)
	r.(worker.HistorySetter).SetHistory(conversation.HistoryReaderFunc(func(ctx context.Context, id string, q conversation.HistoryQuery) ([]conversation.HistoryEntry, error) {
		if _, err := s.resolvePreparedTaskOwner(ctx, id); err != nil {
			return nil, err
		}
		return s.persistSvc.Store().ReadHistory(ctx, id, q)
	}))
	ledger := worker.HostAutonomyLedger(s.persistSvc.Store())
	r.(worker.AutonomyLedgerSetter).SetAutonomyLedger(func(ctx context.Context, op, raw, id string) (string, bool, error) {
		if op == "create" || op == "update" {
			var run conversation.AutonomyRun
			if err := json.Unmarshal([]byte(raw), &run); err != nil {
				return "", false, err
			}
			if _, err := s.resolvePreparedTaskOwner(ctx, run.ConversationID); err != nil {
				return "", false, err
			}
		} else if _, err := s.resolvePreparedTaskOwner(ctx, id); err != nil {
			return "", false, err
		}
		out, found, err := ledger(ctx, op, raw, id)
		if err != nil {
			persistsvc.ReportWriteFailure(ctx, err)
		}
		return out, found, err
	})
	if store, ok := s.persistSvc.Store().(conversation.DispatchEventStore); ok {
		sink := worker.HostDispatchEventSink(store)
		r.(worker.DispatchEventSinkSetter).SetDispatchEventSink(func(ctx context.Context, ev conversation.DispatchEvent) error {
			if _, err := s.resolvePreparedTaskOwner(ctx, ev.ConversationID); err != nil {
				return err
			}
			err := sink(ctx, ev)
			if err != nil {
				persistsvc.ReportWriteFailure(ctx, err)
			}
			return err
		})
	}
	r.(worker.TasksSetter).SetTasks(serverTasks{s})
	r.(worker.SessionModelSetter).SetSessionModel(s.sessionModel)
	if tools, ok := s.toolSvc.(*toolssvc.Service); ok {
		tools.SetDispatchScope(s.beginPreparedDispatch)
	} else {
		return fmt.Errorf("prepared dispatch requires scoped tool service")
	}
	s.workerRunner = r
	s.configureWorkerAccounting()
	return nil
}

type preparedWorkerAuthority struct {
	server      *Server
	ctx         context.Context
	id          string
	ticket      *broker.WorkTicket
	mu          sync.Mutex
	closed      bool
	descendants map[string]context.Context
	completions []func(error) error
	once        sync.Once
}

// Context resolves only capabilities actually acquired by this turn. The caller
// must still carry the exact original owner; an ID or stream ancestry is not a grant.
func (a *preparedWorkerAuthority) Context(ctx context.Context, id string) (context.Context, error) {
	if _, err := a.server.resolvePreparedTaskOwner(ctx, a.id); err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, broker.ErrAdmissionClosed
	}
	target := a.ctx
	if id != a.id {
		target = a.descendants[id]
	}
	if target == nil {
		return nil, fmt.Errorf("prepared worker descendant persistence not authorized")
	}
	if _, err := a.server.resolvePreparedTaskOwner(target, id); err != nil {
		return nil, err
	}
	return target, nil
}

// Ensure retains the acquired context and completion even if row creation fails.
// Calls run on the stream drain path, serially with descendant writes.
func (a *preparedWorkerAuthority) Ensure(ctx context.Context, id, parent, dir, model string, tools []string) error {
	parentCtx, err := a.Context(ctx, parent)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return broker.ErrAdmissionClosed
	}
	if id == a.id || a.descendants[id] != nil {
		return broker.ErrAdmissionBusy
	}
	childCtx, complete, err := a.server.beginPreparedDispatch(parentCtx, parent, id)
	if err != nil {
		return err
	}
	if a.descendants == nil {
		a.descendants = make(map[string]context.Context)
	}
	a.descendants[id] = childCtx
	a.completions = append(a.completions, complete)
	err = a.server.persistSvc.Store().EnsureSubagentConversation(childCtx, id, parent, dir, model, tools)
	if err != nil {
		persistsvc.ReportWriteFailure(childCtx, err)
	}
	return err
}

// Close is called only after the stream and all host callbacks have drained.
// Reverse admission order is a topological bottom-up join. A failed or uncertain
// turn never proves completion, so its scopes and work blockers remain retained.
func (a *preparedWorkerAuthority) Close(err error) {
	a.once.Do(func() {
		a.mu.Lock()
		a.closed = true
		completions := append([]func(error) error(nil), a.completions...)
		a.mu.Unlock()
		runErr := errors.Join(err, persistsvc.WriteFailure(a.ctx))
		var closeErr error
		for i := len(completions) - 1; i >= 0; i-- {
			closeErr = errors.Join(closeErr, completions[i](runErr))
		}
		if runErr != nil || closeErr != nil {
			_ = a.ticket.PersistenceFailed()
			return
		}
		_ = a.ticket.Complete()
	})
}

// preparedDispatchKey carries the exact dispatch scope, not stream ancestry.
type preparedDispatchKey struct{}

// A dispatch owns its child until every scheduled producer has returned, including
// its final persistence callback. Closing the scope fences new schedules before
// joining; cancellation or a returned model response is not completion.
type preparedDispatchScope struct {
	owner  broker.Owner
	mu     sync.Mutex
	closed bool
	waits  []func() error
}

func (d *preparedDispatchScope) join() error {
	d.mu.Lock()
	d.closed = true
	waits := d.waits
	d.mu.Unlock()
	var err error
	for _, wait := range waits {
		err = errors.Join(err, wait())
	}
	return err
}

func (s *Server) beginPreparedDispatch(ctx context.Context, parent, child string) (context.Context, func(error) error, error) {
	if parent == child || child == "" {
		return nil, nil, fmt.Errorf("invalid prepared dispatch")
	}
	cap, err := s.resolvePreparedTaskOwner(ctx, parent)
	if err != nil {
		return nil, nil, err
	}
	// Reserve a join slot before acquisition. Closing the parent and registering
	// a descendant are atomic under the scope lock; no lifecycle lock is held
	// during acquisition or a join. Only the immediate parent's authority works.
	parentScope, _ := ctx.Value(preparedDispatchKey{}).(*preparedDispatchScope)
	done := make(chan struct{})
	var descendantErr error
	registered := false
	if parentScope != nil {
		parentScope.mu.Lock()
		if parentScope.owner != ownerOf(cap) || parentScope.closed {
			parentScope.mu.Unlock()
			return nil, nil, broker.ErrAdmissionClosed
		}
		parentScope.waits = append(parentScope.waits, func() error { <-done; return descendantErr })
		parentScope.mu.Unlock()
	}
	// Failed admission has no completion callback. Release its join slot; any
	// uncertain acquisition still retains the existing parent work blocker.
	defer func() {
		if !registered {
			close(done)
		}
	}()
	ctx, ticket, err := s.BeginRetainedWork(ctx, ownerOf(cap), broker.PersistenceWork)
	if err != nil {
		return nil, nil, err
	}
	l := s.coexistence
	l.mu.Lock()
	if l.owners[child] != nil {
		l.mu.Unlock()
		_ = ticket.Complete()
		return nil, nil, broker.ErrAdmissionBusy
	}
	if _, readErr := conversation.ReadOwnership(ctx, l.store, child); !errors.Is(readErr, sql.ErrNoRows) {
		l.mu.Unlock()
		_ = ticket.Complete()
		if readErr == nil {
			readErr = fmt.Errorf("dispatch child identity already exists")
		}
		return nil, nil, readErr
	}
	// No Retain/reload/user-row creation and no adoption of existing capabilities.
	owned, err := conversation.AcquireOwnership(ctx, l.store, child, l.options.Host)
	if owned != nil {
		l.owners[child] = owned
	}
	l.mu.Unlock()
	if err != nil {
		if owned != nil {
			_ = ticket.PersistenceFailed()
		} else {
			_ = ticket.Complete()
		}
		return nil, nil, err
	}
	// The child is an exact admitted identity, not just a store capability.
	// Keep the parent's ticket across registration, producers and durable release.
	childOwner := ownerOf(owned)
	l.mu.Lock()
	err = l.admission.RegisterOwner(childOwner)
	var childTicket *broker.WorkTicket
	if err == nil {
		childTicket, err = l.admission.AdmitWork(childOwner, broker.PersistenceWork)
	}
	if err == nil {
		l.ready[childOwner] = true
	}
	l.mu.Unlock()
	if err != nil {
		_ = ticket.PersistenceFailed()
		return nil, nil, err
	}
	ctx = conversation.WithOwnership(ctx, cap, owned)
	ctx = WithPreparedTaskOwnership(ctx, childOwner, owned)
	scope := &preparedDispatchScope{owner: childOwner}
	ctx = context.WithValue(ctx, preparedDispatchKey{}, scope)
	var once sync.Once
	var closeErr error
	complete := func(runErr error) error {
		once.Do(func() {
			defer func() {
				descendantErr = closeErr
				close(done)
			}()
			closeErr = errors.Join(scope.join(), persistsvc.WriteFailure(ctx), runErr)
			if closeErr != nil {
				_ = ticket.PersistenceFailed()
				return
			}
			// All child tickets must be drained before its durable release.
			l.mu.Lock()
			err := childTicket.Complete()
			var closure *broker.Closure
			if err == nil {
				closure, err = l.admission.CloseOwner(childOwner, nil)
			}
			l.mu.Unlock()
			if err != nil {
				closeErr = err
				_ = ticket.PersistenceFailed()
				return
			}
			// Durable release must precede removal of the parent work blocker.
			closeErr = owned.Release(context.WithoutCancel(ctx))
			if closeErr != nil {
				_ = ticket.PersistenceFailed()
				return
			}
			closeErr = closure.Relinquished()
			if closeErr != nil {
				_ = ticket.PersistenceFailed()
				return
			}
			l.mu.Lock()
			delete(l.ready, childOwner)
			delete(l.owners, child)
			l.mu.Unlock()
			closeErr = ticket.Complete()
		})
		return closeErr
	}
	registered = true
	return ctx, complete, nil
}
