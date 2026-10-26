// Package taskrun owns independent delegated executions for one host lifetime.
// It deliberately does not own routing or grants: the injected executor must
// enforce the same policy as synchronous dispatch.
package taskrun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
)

type Executor func(context.Context, dispatch.Spec) (dispatch.Result, error)

// ErrTerminalTransitionRejected means no durable terminal transition was
// confirmed. It remains unresolved even if another writer may have finished it.
var ErrTerminalTransitionRejected = errors.New("task terminal transition was not applied")

type execution struct {
	parent     string
	cancel     context.CancelFunc
	done       chan struct{}
	stop       bool
	persistErr error
	ticket     *broker.WorkTicket
}
type Manager struct {
	admission *broker.Admission
	ownership OwnershipResolver
	mu        sync.Mutex
	store     conversation.TaskStore
	execute   Executor
	ctx       context.Context
	cancel    context.CancelFunc
	active    map[string]*execution
	// waitWake holds one edge-triggered wake channel per registered Wait on a
	// still-running task, keyed by the waiter's parent conversation. Only a
	// completion signaled AFTER registration can wake a waiter, so
	// notifications already pending when the wait began never replay into a
	// busy loop.
	waitWake map[string][]chan struct{}
	closed   bool
	wg       sync.WaitGroup
	// Notify is nonblocking host scheduling, not a turn-scoped progress callback.
	notify func(string)
}

// New reconciles prior unfinished executions exactly once per host lifetime.
// A manager must not be constructed per tool call or per client attachment.
func New(store conversation.TaskStore, execute Executor, notify func(string)) (*Manager, error) {
	return newManagerWithInitialization(store, execute, notify, true)
}

// NewPrepared constructs a manager without schema initialization or broad
// reconciliation. The caller must have prepared and validated the store and
// must enforce ownership before enabling coexistence. This constructor alone
// provides no ownership fencing or recovery.
func NewPrepared(store conversation.TaskStore, execute Executor, notify func(string)) (*Manager, error) {
	return newManagerWithInitialization(store, execute, notify, false)
}

// OwnershipResolver returns the exact scoped capability for a launch. It must
// not acquire or transfer ownership; the host retains authority and its OS lock.
type OwnershipResolver func(context.Context, string) (*conversation.Ownership, error)

// NewPreparedOwned integrates an already prepared store with existing admission
// and ownership capabilities. It never initializes, reconciles, or acquires
// ownership. The launch capability is retained unchanged through writeback.
func NewPreparedOwned(store conversation.TaskStore, execute Executor, notify func(string), admission *broker.Admission, ownership OwnershipResolver) (*Manager, error) {
	if admission == nil || ownership == nil {
		return nil, errors.New("admission and ownership resolver are required")
	}
	m, err := NewPrepared(store, execute, notify)
	if err != nil {
		return nil, err
	}
	m.admission, m.ownership = admission, ownership
	return m, nil
}

func newManagerWithInitialization(store conversation.TaskStore, execute Executor, notify func(string), initialize bool) (*Manager, error) {
	if store == nil || execute == nil {
		return nil, errors.New("task persistence and executor are required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	if initialize {
		if err := store.InitTasks(ctx); err != nil {
			cancel()
			return nil, err
		}
		if err := store.ReconcileTasks(ctx); err != nil {
			cancel()
			return nil, err
		}
	}
	return &Manager{store: store, execute: execute, ctx: ctx, cancel: cancel, active: make(map[string]*execution), waitWake: make(map[string][]chan struct{}), notify: notify}, nil
}

func (m *Manager) Launch(ctx context.Context, spec dispatch.Spec) (conversation.TaskRecord, error) {
	if spec.ConversationID == "" || spec.Task == "" {
		return conversation.TaskRecord{}, errors.New("task and parent conversation are required")
	}
	if err := ctx.Err(); err != nil {
		return conversation.TaskRecord{}, err
	}
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return conversation.TaskRecord{}, err
	}
	now := time.Now()
	t := conversation.TaskRecord{ID: "task_" + hex.EncodeToString(entropy[:]), ParentID: spec.ConversationID, Status: "accepted", ChildID: conversation.NewID(), Task: spec.Task, WorkDir: spec.WorkDir, CreatedAt: now, UpdatedAt: now}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return conversation.TaskRecord{}, errors.New("task host is shutting down")
	}
	var ticket *broker.WorkTicket
	if m.admission != nil {
		owner, err := m.ownership(ctx, t.ParentID)
		if err != nil {
			return conversation.TaskRecord{}, err
		}
		if owner == nil || owner.ConversationID() != t.ParentID {
			return conversation.TaskRecord{}, errors.New("task ownership scope mismatch")
		}
		ticket, err = m.admission.AdmitWork(broker.Owner{Conversation: owner.ConversationID(), Host: owner.HostInstance(), Epoch: owner.Epoch()}, broker.TaskAcceptanceWork)
		if err != nil {
			return conversation.TaskRecord{}, err
		}
		ctx = conversation.WithOwnership(ctx, owner)
	}
	// Acknowledge only durable acceptance. A canceled caller cannot leave a
	// fictitious task handle if persistence failed.
	if err := m.store.InsertTask(ctx, t); err != nil {
		if ticket != nil {
			_ = ticket.Complete()
		}
		return conversation.TaskRecord{}, err
	}
	// Preserve policy/context values, not the launching turn's cancellation or
	// deadline. The manager provides the only execution lifetime here.
	child, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stopHost := context.AfterFunc(m.ctx, cancel)
	e := &execution{ticket: ticket, parent: t.ParentID, cancel: cancel, done: make(chan struct{})}
	m.active[t.ID] = e
	spec.Tools = append([]string(nil), spec.Tools...)
	spec.SubConversationID = t.ChildID
	spec.Emit = nil // main-turn callbacks must never escape into independent work
	m.wg.Add(1)
	go func() { defer m.wg.Done(); defer stopHost(); defer cancel(); m.run(child, t, spec, e) }()
	return t, nil
}
func (m *Manager) run(ctx context.Context, t conversation.TaskRecord, spec dispatch.Spec, e *execution) {
	if e.ticket != nil {
		_ = e.ticket.Advance(broker.TaskExecutionWork)
	}
	var result dispatch.Result
	var runErr error
	func() {
		defer func() {
			if p := recover(); p != nil {
				runErr = fmt.Errorf("task executor panic: %v", p)
			}
		}()
		if err := m.store.StartTask(ctx, t.ParentID, t.ID); err != nil {
			runErr = err
			return
		}
		result, runErr = m.execute(ctx, spec)
	}()
	t.Status = "completed"
	if runErr != nil {
		t.Status = "failed"
		t.Error = runErr.Error()
	}
	if result.SubConversationID != "" {
		t.ChildID = result.SubConversationID
	}
	t.ResultJSON, _ = json.Marshal(result) // Result contains only JSON-safe fields.
	m.mu.Lock()
	// Serialize explicit stop versus completion; the first terminal write wins.
	if e.stop {
		t.Status = "stopped"
	} else if m.closed {
		t.Status = "interrupted"
	}
	if t.Status == "interrupted" && t.Error == "" {
		t.Error = "Agent stopped; execution was not replayed. Inspect partial work before retrying."
	}
	if e.ticket != nil {
		_ = e.ticket.Advance(broker.PersistenceWork)
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	var applied bool
	applied, e.persistErr = m.store.FinishTask(persistCtx, t)
	cancel()
	if !applied && e.persistErr == nil {
		e.persistErr = ErrTerminalTransitionRejected
	}
	if e.persistErr != nil && e.ticket != nil {
		_ = e.ticket.PersistenceFailed()
	}
	// Wake task_wait callers blocked in this same conversation: their
	// foreground turn must yield to the completion boundary so this
	// completion is delivered promptly instead of waiting out the wait
	// deadline. Waiters on other conversations are not touched.
	if e.persistErr == nil {
		m.signalWaitWakeLocked(t.ParentID)
	}
	close(e.done)
	// Keep failed persistence visible to Wait rather than pretending completion
	// was durably saved. Successful entries need no retained goroutine state.
	if e.persistErr == nil {
		delete(m.active, t.ID)
	}
	notify := m.notify
	m.mu.Unlock()
	if e.persistErr == nil {
		if e.ticket != nil {
			_ = e.ticket.Advance(broker.CallbackWork)
		}
		if notify != nil {
			notify(t.ParentID)
		}
		if e.ticket != nil {
			_ = e.ticket.Complete()
		}
	}
}
func (m *Manager) Inspect(ctx context.Context, parent, id string) (conversation.TaskRecord, error) {
	t, err := m.store.GetTask(ctx, parent, id)
	if err != nil {
		return t, err
	}
	if store, ok := m.store.(conversation.TaskEvidenceStore); ok {
		t.Evidence, err = store.TaskEvidence(ctx, parent, id)
	}
	return t, err
}
func (m *Manager) List(ctx context.Context, parent string, limit int) ([]conversation.TaskRecord, error) {
	return m.store.ListTasks(ctx, parent, limit)
}
func (m *Manager) Stop(ctx context.Context, parent, id string) (conversation.TaskRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := m.store.GetTask(ctx, parent, id)
	if err != nil {
		return t, err
	}
	if e := m.active[id]; e != nil && !t.Terminal() {
		e.stop = true
		e.cancel()
	}
	return t, nil
}

// Wait observes; canceling its context never cancels the task itself. When
// another task in the SAME conversation finishes while this wait is
// registered, Wait returns early with conversation.ErrTaskWaitInterrupted and
// the waited task's current nonterminal record, so a foreground turn blocked
// in task_wait can yield to the host's normal completion-boundary machinery
// instead of deferring the sibling completion until the wait deadline or user
// input. The wake is edge-triggered per registration: completions that
// predate the wait stay pending for the turn boundary and never replay into a
// busy loop. The waited task is never touched by this early return.
func (m *Manager) Wait(ctx context.Context, parent, id string) (conversation.TaskRecord, error) {
	m.mu.Lock()
	// Register the wake before reading task state: run() signals waiters under
	// the same lock, so any completion whose bookkeeping happens after this
	// point is guaranteed to wake this waiter, even when it races the reads
	// below.
	wake := m.registerWaitWakeLocked(parent)
	defer m.unregisterWaitWake(parent, wake)
	t, err := m.store.GetTask(ctx, parent, id)
	if err != nil {
		m.mu.Unlock()
		return t, err
	}
	e := m.active[id]
	m.mu.Unlock()
	if e != nil {
		select {
		case <-ctx.Done():
			return t, ctx.Err()
		case <-wake:
			// A sibling wake can race the target's failed terminal write.
			// Preserve that failure regardless of which ready channel won.
			select {
			case <-e.done:
				if e.persistErr != nil {
					return t, fmt.Errorf("persist task outcome: %w", e.persistErr)
				}
			default:
			}
			// A same-chat sibling finished while this wait was registered.
			// Re-read the record so the caller sees an accurate status. If the
			// waited task itself finished in the same window, return its real
			// terminal record with no error.
			current, gerr := m.store.GetTask(ctx, parent, id)
			if gerr != nil {
				return t, gerr
			}
			if current.Terminal() {
				return current, nil
			}
			return current, conversation.ErrTaskWaitInterrupted
		case <-e.done:
		}
		if e.persistErr != nil {
			return t, fmt.Errorf("persist task outcome: %w", e.persistErr)
		}
	}
	return m.store.GetTask(ctx, parent, id)
}

// registerWaitWakeLocked adds a one-shot wake channel for parent and returns
// it. Callers must hold m.mu.
func (m *Manager) registerWaitWakeLocked(parent string) chan struct{} {
	wake := make(chan struct{}, 1)
	if parent != "" {
		m.waitWake[parent] = append(m.waitWake[parent], wake)
	}
	return wake
}

// unregisterWaitWake removes a wake registration once its wait returns.
func (m *Manager) unregisterWaitWake(parent string, wake chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if parent == "" {
		return
	}
	chans := m.waitWake[parent]
	for i, ch := range chans {
		if ch == wake {
			m.waitWake[parent] = append(chans[:i], chans[i+1:]...)
			break
		}
	}
	if len(m.waitWake[parent]) == 0 {
		delete(m.waitWake, parent)
	}
}

// signalWaitWakeLocked wakes every waiter registered for parent. The buffered
// one-shot channels keep this nonblocking; each waiter observes at most one
// completion per registration. Callers must hold m.mu.
func (m *Manager) signalWaitWakeLocked(parent string) {
	for _, wake := range m.waitWake[parent] {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	if _, ok := m.waitWake[parent]; ok {
		m.waitWake[parent] = nil
	}
}

// UnresolvedTerminalOutcomes returns a snapshot of failed or rejected terminal
// writes for parent (empty selects all conversations). These outcomes block
// safe ownership release even after execution has stopped. No automatic retry
// or clearing is performed: resolving them requires durable ownership evidence.
// An empty result is not a full quiescence check; running work, admissions and
// completion callbacks must also be drained by the ownership integration.
func (m *Manager) UnresolvedTerminalOutcomes(parent string) map[string]error {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]error)
	for id, e := range m.active {
		if e.persistErr != nil && (parent == "" || e.parent == parent) {
			out[id] = e.persistErr
		}
	}
	return out
}

// Close stops host-owned execution; it does not replay or wait without bounds.
// Success only means goroutines drained, not durable quiescence; callers must
// also account for UnresolvedTerminalOutcomes before releasing ownership.
// The caller must keep persistence open until this returns successfully.
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	m.cancel()
	m.mu.Unlock()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
