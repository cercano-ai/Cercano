package conversation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

const ownershipSchema = `CREATE TABLE conversation_ownership (
 conversation_id TEXT PRIMARY KEY, owner TEXT NOT NULL, epoch TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('owned','released','reserved')),
 destination TEXT NOT NULL DEFAULT '', handover TEXT NOT NULL DEFAULT ''
); CREATE TABLE conversation_handovers (handover TEXT PRIMARY KEY, conversation_id TEXT NOT NULL, destination TEXT NOT NULL, epoch TEXT NOT NULL);`

// Ownership is an explicit, store-bound capability, not a process-global owner.
// Acquire before creating a conversation. Keep it for the retained session.
// Callers must drain work before Release or Handover; this layer does not do so.
// Store.Close does not release capabilities: explicitly Release or Close them.
type Ownership struct {
	mu                         sync.Mutex
	store                      *sqliteStore
	conversation, owner, epoch string
	usable                     atomic.Bool
	lock                       *os.File
}

func (o *Ownership) ConversationID() string { return o.conversation }
func (o *Ownership) HostInstance() string   { return o.owner }
func (o *Ownership) Epoch() string          { return o.epoch }

type ownershipContextKey struct{}

// WithOwnership supplies the complete capability set for a short store call.
// Child creation and rollover require both the new ID and parent/precursor.
// Global pruning requires capabilities for every affected conversation; a missing
// capability rolls back the entire statement, never partially pruning siblings.
func WithOwnership(ctx context.Context, owners ...*Ownership) context.Context {
	return context.WithValue(ctx, ownershipContextKey{}, append([]*Ownership(nil), owners...))
}

func ownershipStore(store Store) (*sqliteStore, error) {
	s, ok := store.(*sqliteStore)
	if !ok || s.coexistencePath == "" {
		return nil, fmt.Errorf("ownership requires OpenCoexistence")
	}
	return s, nil
}

// AcquireOwnership explicitly acquires a new or cleanly released record. An
// available lock never authorizes recovery of an owned or reserved record.
func AcquireOwnership(ctx context.Context, store Store, conversationID, hostInstance string) (*Ownership, error) {
	return acquireOwnership(ctx, store, conversationID, hostInstance, "")
}

// ClaimHandover consumes only the exact destination instance and handover ID.
// A completed claim is not reacquired: callers retain its capability and may use
// HandoverResult to resolve retries without allocating a second epoch.
func ClaimHandover(ctx context.Context, store Store, conversationID, hostInstance, handoverID string) (*Ownership, error) {
	if handoverID == "" {
		return nil, fmt.Errorf("handover ID required")
	}
	epoch, err := HandoverResult(ctx, store, conversationID, hostInstance, handoverID)
	if err == nil {
		return nil, &HandoverAlreadyClaimed{Epoch: epoch}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return acquireOwnership(ctx, store, conversationID, hostInstance, handoverID)
}

func acquireOwnership(ctx context.Context, store Store, id, host, handover string) (*Ownership, error) {
	s, err := ownershipStore(store)
	if err != nil {
		return nil, err
	}
	if id == "" || host == "" {
		return nil, fmt.Errorf("conversation and exact host instance required")
	}
	// Hash the conversation ID, never the epoch. Lock files are NEVER removed.
	dir := s.coexistencePath + ".ownership-locks"
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(fmt.Sprintf("%s/%x.lock", dir, sha256.Sum256([]byte(id))), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockOwnershipFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("conversation %q lock contention: %w", id, err)
	}
	o := &Ownership{store: s, conversation: id, owner: host, epoch: newID(), lock: f}
	success := false
	defer func() {
		if !success {
			o.Close()
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := checkOwnershipContract(ctx, tx); err != nil {
		return nil, err
	}
	var state, destination, pending string
	err = tx.QueryRowContext(ctx, `SELECT state,destination,handover FROM conversation_ownership WHERE conversation_id=?`, id).Scan(&state, &destination, &pending)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if handover == "" {
		if err == nil && state != "released" {
			return nil, fmt.Errorf("conversation %q is %s; explicit matching handover or recovery required (crash recovery unsupported)", id, state)
		}
	} else if err != nil || state != "reserved" || destination != host || pending != handover {
		return nil, fmt.Errorf("handover reservation does not match exact destination and ID; recovery unsupported")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO conversation_ownership(conversation_id,owner,epoch,state,destination,handover) VALUES(?,?,?,'owned',?,?) ON CONFLICT(conversation_id) DO UPDATE SET owner=excluded.owner,epoch=excluded.epoch,state='owned',destination=excluded.destination,handover=excluded.handover`, id, host, o.epoch, host, handover)
	if err != nil {
		return nil, err
	}
	if handover != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_handovers(handover,conversation_id,destination,epoch) VALUES(?,?,?,?)`, handover, id, host, o.epoch); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		// Keep the lock on uncertain commit: caller receives the capability to close
		// explicitly, but must not admit work based on this failed acquisition.
		success = true
		return o, fmt.Errorf("ownership commit uncertain; retain lock and inspect durable state: %w", err)
	}
	success = true
	o.usable.Store(true)
	return o, nil
}

// HandoverResult returns the recorded successful claim epoch, even after later
// releases/transfers. No ownership is granted by this read-only receipt.
func HandoverResult(ctx context.Context, store Store, conversationID, destination, handoverID string) (string, error) {
	s, err := ownershipStore(store)
	if err != nil {
		return "", err
	}
	var epoch string
	err = s.db.QueryRowContext(ctx, `SELECT epoch FROM conversation_handovers WHERE handover=? AND conversation_id=? AND destination=?`, handoverID, conversationID, destination).Scan(&epoch)
	return epoch, err
}

// Release records clean relinquishment before unlocking. Errors retain the lock.
func (o *Ownership) Release(ctx context.Context) error { return o.relinquish(ctx, "released", "", "") }

// Handover durably reserves to an exact host instance and returns the unique ID.
// Success means PENDING, not transferred. Only the destination's durable claim
// completes transfer. There is deliberately no cancellation/timeout reassignment.
func (o *Ownership) Handover(ctx context.Context, destination string) (string, error) {
	if destination == "" || destination == o.owner {
		return "", fmt.Errorf("distinct exact destination instance required")
	}
	id := newID()
	err := o.relinquish(ctx, "reserved", destination, id)
	return id, err
}
func (o *Ownership) relinquish(ctx context.Context, state, destination, handover string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lock == nil {
		return fmt.Errorf("ownership capability is closed")
	}
	tx, err := o.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = checkOwnershipContract(ctx, tx); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE conversation_ownership SET state=?,destination=?,handover=? WHERE conversation_id=? AND owner=? AND epoch=? AND state='owned'`, state, destination, handover, o.conversation, o.owner, o.epoch)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("stale ownership epoch")
	}
	if err = tx.Commit(); err != nil {
		o.usable.Store(false)
		return fmt.Errorf("relinquishment commit uncertain; lock retained; inspect durable state: %w", err)
	}
	o.usable.Store(false)
	err = o.lock.Close()
	o.lock = nil
	return err
}

// Close drops only the OS lock, NOT durable ownership. This models abandonment
// and deliberately blocks normal reacquisition. Prefer clean Release.
func (o *Ownership) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lock == nil {
		return nil
	}
	o.usable.Store(false)
	err := o.lock.Close()
	o.lock = nil
	return err
}

func checkOwnershipContract(ctx context.Context, tx *sql.Tx) error {
	d := CurrentCoexistenceDeclaration()
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM coexistence_metadata WHERE singleton=1 AND contract=? AND schema_version=? AND (SELECT user_version FROM pragma_user_version)=?`, d.Contract, d.SchemaVersion, d.SchemaVersion).Scan(&n)
	if err != nil {
		return fmt.Errorf("coexistence contract: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("coexistence contract mismatch")
	}
	return nil
}

// HandoverAlreadyClaimed is an idempotent claim receipt, NOT a capability.
// It can be returned even after the destination has subsequently released.
type HandoverAlreadyClaimed struct{ Epoch string }

func (e *HandoverAlreadyClaimed) Error() string {
	return "handover already claimed at epoch " + e.Epoch
}

// OwnershipState is read-only diagnostic state, never authority to write.
type OwnershipState struct{ Owner, Epoch, State, Destination, Handover string }

func ReadOwnership(ctx context.Context, store Store, conversationID string) (OwnershipState, error) {
	s, err := ownershipStore(store)
	if err != nil {
		return OwnershipState{}, err
	}
	var state OwnershipState
	err = s.db.QueryRowContext(ctx, `SELECT owner,epoch,state,destination,handover FROM conversation_ownership WHERE conversation_id=?`, conversationID).Scan(&state.Owner, &state.Epoch, &state.State, &state.Destination, &state.Handover)
	return state, err
}
