package server

// Launch receipts grant a narrowly scoped right to undo a NEW acquisition. They
// are not session leases: observation/delivery consumes this right permanently.
import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/protobuf/proto"
)

func (l *coexistenceLifecycle) launchJournal(operation string) string {
	return filepath.Join(l.options.StorePath+".launch-operations", fmt.Sprintf("%x.json", sha256.Sum256([]byte(l.options.Host+"\x00"+operation))))
}
func (l *coexistenceLifecycle) saveLaunch(r *pb.LifecycleControlRequest, state string) error {
	next := proto.Clone(r).(*pb.LifecycleControlRequest)
	next.LaunchAction = state
	// Update memory even if directory sync is uncertain. Never restore cleanup
	// authority after a possibly visible delivery/abort publication.
	l.launches[r.LaunchOperation] = next
	return saveResume(l.launchJournal(r.LaunchOperation), next)
}

// Called with mu held, before exposing any capability or accepting observation.
// Failed publication refuses observation, but still consumes authority in memory.
func (l *coexistenceLifecycle) observeLaunchLocked(o broker.Owner) error {
	for _, r := range l.launches {
		if r.GetOwner().GetConversation() != o.Conversation {
			continue
		}
		// Receipts fence only their exact acquisition, never a later legitimate
		// owner of the same conversation after explicit release/resume.
		if r.Owner.Host != "" && controlOwner(r) != o {
			continue
		}
		switch r.LaunchAction {
		case "aborting", "aborted":
			return broker.ErrAdmissionStale
		case "acquiring", "pending", "delivered":
			// During reload scheduling the exact owner may not yet be journaled. A
			// conservative observation still permanently consumes this operation.
			r = proto.Clone(r).(*pb.LifecycleControlRequest)
			r.Owner = wireOwner(o)
			if err := l.saveLaunch(r, "delivered"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) launchControl(ctx context.Context, l *coexistenceLifecycle, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r.LaunchOperation == "" || len(r.LaunchOperation) > 128 || r.GetOwner().GetConversation() == "" {
		return nil, fmt.Errorf("launch operation and conversation required")
	}
	if r.LaunchAction != "acquire" && r.LaunchAction != "deliver" && r.LaunchAction != "abort" {
		return nil, fmt.Errorf("unknown launch receipt action")
	}
	if l.launches == nil {
		l.launches = make(map[string]*pb.LifecycleControlRequest)
	}
	op := l.launches[r.LaunchOperation]
	if op == nil {
		b, err := os.ReadFile(l.launchJournal(r.LaunchOperation))
		if err == nil {
			op = new(pb.LifecycleControlRequest)
			if err = json.Unmarshal(b, op); err != nil {
				return nil, err
			}
			l.launches[r.LaunchOperation] = op
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	if op != nil && (!proto.Equal(op.Target, r.Target) || op.LaunchOperation != r.LaunchOperation || op.GetOwner().GetConversation() != r.GetOwner().GetConversation()) {
		return nil, broker.ErrAdmissionStale
	}
	if op == nil {
		if r.LaunchAction != "acquire" || r.Owner.Host != "" || r.Owner.Epoch != "" {
			return nil, broker.ErrAdmissionStale
		}
		// Ownership is acquired first; the initializer then checks absence under the
		// exact capability. Existing/resumed rows can never gain launch authority.
		if l.owners[r.Owner.Conversation] != nil {
			return nil, fmt.Errorf("launch cannot adopt existing ownership")
		}
		if _, err := l.store.Get(ctx, r.Owner.Conversation); !errors.Is(err, sql.ErrNoRows) {
			if err == nil {
				return nil, fmt.Errorf("launch cannot adopt an existing conversation")
			}
			return nil, err
		}
		op = proto.Clone(r).(*pb.LifecycleControlRequest)
		if err := l.saveLaunch(op, "acquiring"); err != nil {
			return nil, err
		}
		o, err := l.acquireInitialized(ctx, r.Owner.Conversation, nil, func(ctx context.Context, cap *conversation.Ownership) error {
			_, err := l.store.Get(ctx, cap.ConversationID())
			if !errors.Is(err, sql.ErrNoRows) {
				if err == nil {
					return fmt.Errorf("launch receipt requires a new conversation")
				}
				return err
			}
			return l.store.EnsureConversation(ctx, cap.ConversationID(), "", "")
		})
		if err != nil {
			return nil, fmt.Errorf("launch %s acquisition unresolved; retry same operation, never infer an epoch: %w", r.LaunchOperation, err)
		}
		// Reload may have admitted observation while mu was temporarily unlocked.
		state := l.launches[r.LaunchOperation].LaunchAction
		op.Owner = wireOwner(o)
		if state != "delivered" {
			state = "pending"
		}
		if err = l.saveLaunch(op, state); err != nil {
			return nil, err
		}
		op = l.launches[r.LaunchOperation]
	}
	response := func() *pb.LifecycleControlResponse {
		return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: op.Owner, Receipt: op.LaunchOperation, Pending: op.LaunchAction == "pending"}
	}
	if r.LaunchAction == "acquire" {
		if op.LaunchAction != "pending" && op.LaunchAction != "delivered" {
			return nil, fmt.Errorf("launch %s is %s; do not reacquire", r.LaunchOperation, op.LaunchAction)
		}
		return response(), nil
	}
	if !proto.Equal(r.Owner, op.Owner) || r.Owner.Host != l.options.Host || r.Owner.Epoch == "" {
		return nil, broker.ErrAdmissionStale
	}
	if r.LaunchAction == "deliver" {
		if op.LaunchAction != "pending" && op.LaunchAction != "delivered" {
			return nil, broker.ErrAdmissionStale
		}
		if !l.ready[controlOwner(op)] {
			return nil, broker.ErrAdmissionStale
		}
		if err := l.saveLaunch(op, "delivered"); err != nil {
			return nil, err
		}
		op = l.launches[r.LaunchOperation]
		return response(), nil
	}
	if op.LaunchAction == "aborted" {
		if err := l.saveLaunch(op, "aborted"); err != nil {
			return nil, err
		}
		return response(), nil
	}
	if op.LaunchAction != "pending" && op.LaunchAction != "aborting" {
		return nil, fmt.Errorf("launch %s was observed/delivered or unresolved; cleanup forbidden", r.LaunchOperation)
	}
	o := controlOwner(op)
	// Persist the irrevocable decision before closing admission. Both observation
	// and abort are serialized by mu; detached sessions cannot become pending again.
	if err := l.saveLaunch(op, "aborting"); err != nil {
		return nil, err
	}
	reserved := l.closures[o]
	var err error
	if reserved == nil {
		terminal := ""
		reserved, err = l.reserveRelinquishLocked(o, nil, &terminal)
		if reserved != nil {
			reserved.control = &outgoingControl{}
		}
	}
	if err == nil {
		if l.owners[o.Conversation] == nil && reserved != nil {
			if l.options.Cleanup != nil {
				err = l.options.Cleanup(ctx, o)
			}
			if err == nil {
				err = reserved.ticket.Complete()
			}
		} else {
			err = l.commitRelinquishLocked(ctx, o, reserved, nil)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("launch %s abort unresolved; retry abort with exact owner %s/%s/%s: %w", op.LaunchOperation, o.Conversation, o.Host, o.Epoch, err)
	}
	if err = l.saveLaunch(op, "aborted"); err != nil {
		return nil, err
	}
	op = l.launches[r.LaunchOperation]
	return response(), nil
}
