package conversation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// RecoverDeadOwner is an explicit recovery operation, never an ordinary acquire.
// verifyDead must be supplied by trusted host startup, not by an RPC caller. It
// must establish absence of the exact source host while preventing its relaunch.
// The caller holds that exclusion through this call. Network failure alone is
// not evidence. Exact reserved roots serialize against ClaimHandover using the
// same stable conversation lock; descendant reservations remain blockers.
func RecoverDeadOwner(ctx context.Context, store Store, conversationID string, expected OwnershipState, host string, verifyDead func(context.Context) error) (*Ownership, error) {
	return recoverDeadOwner(ctx, store, conversationID, expected, host, verifyDead, (*sql.Tx).Commit)
}

// commit is a per-call transaction boundary seam; production always uses sql.Tx.Commit.
func recoverDeadOwner(ctx context.Context, store Store, conversationID string, expected OwnershipState, host string, verifyDead func(context.Context) error, commit func(*sql.Tx) error) (*Ownership, error) {
	s, err := ownershipStore(store)
	if err != nil {
		return nil, err
	}
	if (expected.State != "owned" && expected.State != "reserved") || conversationID == "" || expected.Owner == "" || expected.Epoch == "" || host == "" || host == expected.Owner || verifyDead == nil {
		return nil, fmt.Errorf("exact abandoned owner, distinct destination and death verification required")
	}
	if expected.State == "reserved" && (expected.Destination == "" || expected.Handover == "") {
		return nil, fmt.Errorf("exact reserved destination and handover required")
	}
	// UNION (not UNION ALL) bounds corrupt cyclic ancestry. Include tasks whose
	// child transcript was never created, as well as persisted subagent ancestry.
	const tree = `WITH RECURSIVE tree(id) AS (SELECT ? UNION SELECT c.id FROM conversations c JOIN tree t ON c.parent_id=t.id UNION SELECT d.child_id FROM delegated_tasks d JOIN tree t ON d.parent_id=t.id WHERE d.child_id!='') SELECT id FROM tree ORDER BY id`
	ids := []string{}
	rows, err := s.db.QueryContext(ctx, tree, conversationID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	sort.Strings(ids)
	held := []*os.File{}
	keep := false
	defer func() {
		if !keep {
			for _, f := range held {
				f.Close()
			}
		}
	}()
	for _, id := range ids {
		f, e := openOwnershipLock(s.coexistencePath, id, true)
		if e != nil {
			return nil, e
		}
		held = append(held, f)
		if e = lockOwnershipFile(f); e != nil {
			return nil, fmt.Errorf("recovery lock %q: %w", id, e)
		}
	}
	if err = verifyDead(ctx); err != nil {
		return nil, err
	}
	// A source that was alive during tree discovery must not have added children
	// before absence was established. Compare again under the writer transaction.
	tx, err := s.beginWrite(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = s.checkOwnershipContract(ctx, tx); err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, tree, conversationID)
	if err != nil {
		return nil, err
	}
	n := 0
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		if n >= len(ids) || ids[n] != id {
			rows.Close()
			return nil, fmt.Errorf("recovery tree changed; retry")
		}
		n++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if n != len(ids) {
		return nil, fmt.Errorf("recovery tree changed; retry")
	}
	// These connection-local capabilities are justified only by all exclusive
	// locks and verified death. They are never returned as old-epoch authority.
	if _, err = tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS ownership_capabilities (conversation_id TEXT PRIMARY KEY, owner TEXT NOT NULL, epoch TEXT NOT NULL); DELETE FROM ownership_capabilities;`); err != nil {
		return nil, err
	}
	states := make(map[string]OwnershipState)
	for _, id := range ids {
		var v OwnershipState
		e := tx.QueryRowContext(ctx, `SELECT owner,epoch,state,destination,handover FROM conversation_ownership WHERE conversation_id=?`, id).Scan(&v.Owner, &v.Epoch, &v.State, &v.Destination, &v.Handover)
		if e == sql.ErrNoRows && id != conversationID {
			continue
		}
		if e != nil {
			return nil, e
		}
		if id == conversationID && v != expected {
			return nil, fmt.Errorf("stale recovery owner")
		}
		if (v.State == "reserved" && id != conversationID) || (v.State == "owned" && v.Owner != expected.Owner) {
			return nil, fmt.Errorf("descendant has independent or reserved ownership")
		}
		if v.State == "owned" || (id == conversationID && v.State == "reserved") {
			// The exact reservation was checked above under the stable claim lock
			// and writer transaction. Restore source state only inside this same
			// transaction so existing write fences authorize reconciliation. No
			// old capability or intermediate owned state is published.
			if v.State == "reserved" {
				if _, err = tx.ExecContext(ctx, `UPDATE conversation_ownership SET state='owned' WHERE conversation_id=?`, id); err != nil {
					return nil, err
				}
			}
			states[id] = v
			if _, err = tx.ExecContext(ctx, `INSERT INTO ownership_capabilities VALUES(?,?,?)`, id, v.Owner, v.Epoch); err != nil {
				return nil, err
			}
		}
	}
	blocker, _ := json.Marshal(AutonomyBlocker{Reason: "Owning server exited. Review interrupted work and external effects before continuing.", RecordedAt: time.Now().UTC()})
	for _, id := range ids {
		// Only abandoned owned parents may have execution reconciled. Released
		// descendants must already be quiescent; do not silently repair corruption.
		if _, ok := states[id]; !ok {
			var count int
			if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM delegated_tasks WHERE parent_id=? AND status IN ('accepted','running')`, id).Scan(&count); err != nil {
				return nil, err
			}
			if count != 0 {
				return nil, fmt.Errorf("unowned descendant has unfinished tasks")
			}
			continue
		}
		_, err = tx.ExecContext(ctx, `UPDATE delegated_tasks SET status='interrupted',error=CASE WHEN error='' THEN 'Owning server exited; external effects may be incomplete. Not replayed.' ELSE error END,notification_pending=1,wake_claimed=1,updated_at=? WHERE parent_id=? AND status IN ('accepted','running')`, time.Now().UnixMilli(), id)
		if err != nil {
			return nil, err
		}
		// Retain results and notifications, but prevent automatic completion turns
		// from replaying uncertain external actions during recovery hydration.
		if _, err = tx.ExecContext(ctx, `UPDATE delegated_tasks SET wake_claimed=1 WHERE parent_id=? AND notification_pending=1`, id); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE autonomy_runs SET blocker_json=CASE WHEN blocker_json='' THEN ? ELSE blocker_json END WHERE conversation_id=? AND state IN ('running','review_pending')`, string(blocker), id); err != nil {
			return nil, err
		}
	}
	epoch := newID()
	receipt := RecoveryID(conversationID, expected, host)
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_handovers(handover,conversation_id,destination,epoch) VALUES(?,?,?,?)`, receipt, conversationID, host, epoch); err != nil {
		return nil, err
	}
	for id, v := range states {
		state, owner, next := "released", v.Owner, v.Epoch
		if id == conversationID {
			state, owner, next = "owned", host, epoch
		}
		if _, err = tx.ExecContext(ctx, `UPDATE conversation_ownership SET owner=?,epoch=?,state=?,destination='',handover='' WHERE conversation_id=?`, owner, next, state, id); err != nil {
			return nil, err
		}
	}
	var root *os.File
	others := []*os.File{}
	for i, id := range ids {
		if id == conversationID {
			root = held[i]
		} else {
			others = append(others, held[i])
		}
	}
	o := &Ownership{store: s, conversation: conversationID, owner: host, epoch: epoch, lock: root, recoveryLocks: others}
	if err = commit(tx); err != nil {
		keep = true
		return o, fmt.Errorf("recovery commit uncertain; locks retained: %w", err)
	}
	for _, f := range others {
		f.Close()
	}
	o.recoveryLocks = nil
	o.usable.Store(true)
	keep = true
	return o, nil
}

// CurrentRecoveryResult returns metadata only for the exact current owned epoch.
// Recovery receipts are source-qualified by RecoveryID; historical receipts must
// never supply metadata for a later acquisition, or grant ownership themselves.
func CurrentRecoveryResult(ctx context.Context, store Store, id, host, epoch string) (string, error) {
	s, err := ownershipStore(store)
	if err != nil {
		return "", err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT h.handover FROM conversation_handovers h
  JOIN conversation_ownership o ON o.conversation_id=h.conversation_id AND o.owner=h.destination AND o.epoch=h.epoch
  WHERE h.conversation_id=? AND h.destination=? AND h.epoch=? AND o.state='owned'
  AND h.handover GLOB 'recovery-*' LIMIT 2`, id, host, epoch)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var operation string
	count := 0
	for rows.Next() {
		if err = rows.Scan(&operation); err != nil {
			return "", err
		}
		count++
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	if count == 0 {
		return "", sql.ErrNoRows
	}
	if count > 1 {
		return "", fmt.Errorf("ambiguous recovery receipts for current owned epoch")
	}
	return operation, nil
}

// RecoveryID binds an idempotence receipt to the abandoned epoch and destination.
func RecoveryID(id string, expected OwnershipState, host string) string {
	return fmt.Sprintf("recovery-%x", sha256.Sum256([]byte(fmt.Sprintf("%q/%q/%q/%q", id, expected.Owner, expected.Epoch, host))))
}
