package conversation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
)

// RestoreClaimedHandover reopens only the exact still-current durable claim.
// It is not recovery, does not mint an epoch, and cannot restore a released,
// transferred, or active capability. The conversation lock excludes the old
// holder; history alone never grants ownership.
func RestoreClaimedHandover(ctx context.Context, store Store, id, host, handover string) (*Ownership, error) {
	s, err := ownershipStore(store)
	if err != nil {
		return nil, err
	}
	if id == "" || host == "" || handover == "" {
		return nil, fmt.Errorf("exact claim required")
	}
	epoch, err := HandoverResult(ctx, store, id, host, handover)
	if err != nil {
		return nil, err
	}
	dir := s.coexistencePath + ".ownership-locks"
	f, err := os.OpenFile(fmt.Sprintf("%s/%x.lock", dir, sha256.Sum256([]byte(id))), os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			f.Close()
		}
	}()
	if err = lockOwnershipFile(f); err != nil {
		return nil, err
	}
	tx, err := s.beginWrite(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = s.checkOwnershipContract(ctx, tx); err != nil {
		return nil, err
	}
	var currentHost, currentEpoch, state, destination, pending string
	err = tx.QueryRowContext(ctx, `SELECT owner,epoch,state,destination,handover FROM conversation_ownership WHERE conversation_id=?`, id).Scan(&currentHost, &currentEpoch, &state, &destination, &pending)
	if err != nil {
		return nil, err
	}
	if currentHost != host || currentEpoch != epoch || state != "owned" || destination != host || pending != handover {
		return nil, fmt.Errorf("durable claim no longer current")
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	o := &Ownership{store: s, conversation: id, owner: host, epoch: epoch, lock: f}
	o.usable.Store(true)
	keep = true
	return o, nil
}
