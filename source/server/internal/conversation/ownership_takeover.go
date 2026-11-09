package conversation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
)

// ReserveTakeover supersedes this exact source host and epoch's owned or
// reserved row with a reservation for destination and operation. The caller
// MUST already have quiesced and drained the source, including external work;
// this low-level primitive does not stop work and must not be exposed as an
// independent user/RPC operation. Operation/source binding is the caller's
// responsibility. A successful reservation is pending, not a destination claim.
//
// Exact retries consult current durable authority, never historical receipts.
// Errors retain an existing lock; uncertain commits disable the capability and
// retain even a reacquired lock for explicit inspection/Close, like relinquish.
func (o *Ownership) ReserveTakeover(ctx context.Context, destination, operation string) error {
	if o == nil || o.store == nil || o.store.coexistencePath == "" || o.conversation == "" || o.owner == "" || o.epoch == "" || destination == "" || destination == o.owner || operation == "" {
		return fmt.Errorf("exact source identity, distinct destination and explicit operation required")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	acquired := false
	if o.lock == nil {
		// ReserveHandover closes the source lock. Reopen precisely the same stable
		// conversation path, never an epoch-specific path or a blocking lock.
		dir := o.store.coexistencePath + ".ownership-locks"
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(fmt.Sprintf("%s/%x.lock", dir, sha256.Sum256([]byte(o.conversation))), os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		if err = lockOwnershipFile(f); err != nil {
			f.Close()
			return fmt.Errorf("conversation %q lock contention: %w", o.conversation, err)
		}
		o.lock = f
		acquired = true
	}
	// A failed pre-commit attempt must not strand a newly acquired lock. A
	// previously held lock, however, remains the caller's responsibility.
	retain := false
	defer func() {
		if acquired && !retain {
			o.lock.Close()
			o.lock = nil
		}
	}()
	tx, err := o.store.beginWrite(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = o.store.checkOwnershipContract(ctx, tx); err != nil {
		return err
	}
	var owner, epoch, state, dest, pending string
	err = tx.QueryRowContext(ctx, `SELECT owner,epoch,state,destination,handover FROM conversation_ownership WHERE conversation_id=?`, o.conversation).Scan(&owner, &epoch, &state, &dest, &pending)
	if err != nil {
		return err
	}
	if owner != o.owner || epoch != o.epoch || (state != "owned" && state != "reserved") {
		return fmt.Errorf("stale ownership epoch or released source")
	}
	if state == "owned" && !o.usable.Load() {
		return fmt.Errorf("ownership capability is closed or uncertain")
	}
	exact := state == "reserved" && dest == destination && pending == operation
	var n int
	err = tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM conversation_handovers WHERE handover=?) + (SELECT count(*) FROM conversation_ownership WHERE handover=? AND conversation_id<>?)`, operation, operation, o.conversation).Scan(&n)
	if err != nil {
		return err
	}
	if n != 0 || (pending == operation && !exact) {
		return fmt.Errorf("operation ID already used")
	}
	if !exact {
		res, err := tx.ExecContext(ctx, `UPDATE conversation_ownership SET state='reserved',destination=?,handover=? WHERE conversation_id=? AND owner=? AND epoch=? AND state IN ('owned','reserved')`, destination, operation, o.conversation, o.owner, o.epoch)
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
	}
	if err = tx.Commit(); err != nil {
		retain = true
		o.usable.Store(false)
		return fmt.Errorf("takeover relinquishment commit uncertain; lock retained; inspect durable state: %w", err)
	}
	o.usable.Store(false)
	for _, f := range o.recoveryLocks {
		f.Close()
	}
	o.recoveryLocks = nil
	err = o.lock.Close()
	o.lock = nil
	retain = true
	return err
}
