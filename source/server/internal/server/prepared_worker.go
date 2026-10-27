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
// Host-side independent dispatch uses a per-dispatch scope. Worker-originated
// nested dispatch remains explicitly unsupported.
func (s *Server) ConfigurePreparedWorker(policy worker.SpawnPolicy) error {
	if s.coexistence == nil || s.cfgSvc == nil || s.persistSvc == nil || s.permBroker == nil {
		return fmt.Errorf("prepared worker requires prepared server services")
	}
	if s.workerRunner != nil {
		return fmt.Errorf("worker runner already configured")
	}
	if err := policy.ValidatePrepared(); err != nil {
		return err
	}
	r := worker.NewWorkerRunnerWithPolicy(policy, s.persistSvc, s.cfgSvc, s.permBroker,
		func(ctx context.Context, id, parent, dir, model string, tools []string) error {
			if _, err := s.resolvePreparedTaskOwner(ctx, parent); err != nil {
				return err
			}
			return fmt.Errorf("prepared worker descendant acquisition is not enabled")
		},
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
	server *Server
	ctx    context.Context
	id     string
	ticket *broker.WorkTicket
}

func (a *preparedWorkerAuthority) Context(ctx context.Context, id string) (context.Context, error) {
	if id != a.id {
		return nil, fmt.Errorf("prepared worker descendant persistence not authorized")
	}
	if _, err := a.server.resolvePreparedTaskOwner(ctx, id); err != nil {
		return nil, err
	}
	return a.ctx, nil
}
func (a *preparedWorkerAuthority) Close(err error) {
	if err != nil || persistsvc.WriteFailure(a.ctx) != nil {
		_ = a.ticket.PersistenceFailed()
		return
	}
	_ = a.ticket.Complete()
}

// preparedDispatchKey bounds descendants to a single admitted level.
type preparedDispatchKey struct{}

func (s *Server) beginPreparedDispatch(ctx context.Context, parent, child string) (context.Context, func(error) error, error) {
	if ctx.Value(preparedDispatchKey{}) != nil || parent == child || child == "" {
		return nil, nil, fmt.Errorf("nested or invalid prepared dispatch")
	}
	cap, err := s.resolvePreparedTaskOwner(ctx, parent)
	if err != nil {
		return nil, nil, err
	}
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
	ctx = conversation.WithOwnership(ctx, cap, owned)
	ctx = context.WithValue(ctx, preparedDispatchKey{}, true)
	var once sync.Once
	var closeErr error
	complete := func(runErr error) error {
		once.Do(func() {
			closeErr = persistsvc.WriteFailure(ctx)
			if closeErr == nil {
				closeErr = runErr
			}
			if closeErr != nil {
				_ = ticket.PersistenceFailed()
				return
			}
			// Durable release must precede removal of the parent work blocker.
			closeErr = owned.Release(context.WithoutCancel(ctx))
			if closeErr != nil {
				_ = ticket.PersistenceFailed()
				return
			}
			l.mu.Lock()
			delete(l.owners, child)
			l.mu.Unlock()
			closeErr = ticket.Complete()
		})
		return closeErr
	}
	return ctx, complete, nil
}
