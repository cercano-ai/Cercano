package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// The journal is private, store-adjacent and bound to the exact destination.
// Like development host records it is atomically published and directory-synced.
// It is intentionally not part of the shared conversation schema contract.
func resumeJournal(l *coexistenceLifecycle, id string) string {
	return filepath.Join(l.options.StorePath+".resume-operations", fmt.Sprintf("%x.json", sha256.Sum256([]byte(l.options.Host+"\x00"+id))))
}
func saveResume(path string, r *pb.LifecycleControlRequest) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// Persist the journal directory itself too: syncing only its contents
	// cannot preserve a newly created directory across a crash.
	parent, err := os.Open(filepath.Dir(filepath.Dir(path)))
	if err != nil {
		return err
	}
	err = parent.Sync()
	closeErr := parent.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".resume-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// LifecycleResume runs only on the launcher-selected destination. It never
// discovers a destination or treats an unavailable RPC as death. Legacy stops
// require a durable, exact approval and a trusted platform adapter.
func (s *Server) LifecycleResume(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, err := s.control(ctx, r)
	if err != nil {
		return nil, err
	}
	id := r.GetOwner().GetConversation()
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "conversation required")
	}
	l.resumeMu.Lock()
	defer l.resumeMu.Unlock()
	var approved *pb.LifecycleControlRequest
	if r.ConfirmLegacyStop {
		approved, err = validateLegacyApproval(l, r)
		if err != nil {
			return nil, err
		}
	}
	acquired, acquireErr := s.LifecycleAcquire(ctx, r)
	if acquireErr == nil {
		acquired.Ready = true
		// A ready retry still reports the same durable operation, if this
		// acquisition came from resume rather than ordinary unowned acquire.
		if b, e := os.ReadFile(resumeJournal(l, id)); e == nil {
			var op pb.LifecycleControlRequest
			if json.Unmarshal(b, &op) == nil && proto.Equal(op.Destination, l.controlIdentity()) {
				epoch, e := conversation.HandoverResult(ctx, l.store, id, l.options.Host, op.HandoverId)
				if e == nil && epoch == acquired.GetOwner().GetEpoch() {
					acquired.OperationId = op.HandoverId
				}
			}
		}
		return acquired, nil
	}
	v, err := conversation.ReadOwnership(ctx, l.store, id)
	if err != nil {
		return nil, err
	}
	if approved != nil {
		// If a response was lost after the stop, guarded recovery can finish
		// without signalling again. Never retry a signal against a changed PID.
		if l.options.RecoveryGuard != nil {
			if inspected, e := s.LifecycleRecover(ctx, &pb.LifecycleRecoverRequest{Target: r.Target, Conversation: id}); e == nil && proto.Equal(inspected.Owner, approved.Owner) {
				return s.LifecycleRecover(ctx, &pb.LifecycleRecoverRequest{Target: r.Target, Conversation: id, ExpectedOwner: approved.Owner, Confirm: true})
			}
		}
		return s.resumeConfirmedLegacy(ctx, r, approved)
	}
	// Recovery repeats both the guarded death check and exact owner/epoch check.
	if v.State == "owned" && v.Owner != l.options.Host && l.options.RecoveryGuard != nil {
		inspected, e := s.LifecycleRecover(ctx, &pb.LifecycleRecoverRequest{Target: r.Target, Conversation: id})
		if e == nil {
			return s.LifecycleRecover(ctx, &pb.LifecycleRecoverRequest{Target: r.Target, Conversation: id, ExpectedOwner: inspected.Owner, Confirm: true})
		}
	}
	if !controlLoopback(r.DestinationEndpoint) {
		return nil, status.Error(codes.InvalidArgument, "selected destination endpoint required")
	}
	path := resumeJournal(l, id)
	op := new(pb.LifecycleControlRequest)
	b, readErr := os.ReadFile(path)
	if readErr == nil {
		if err = json.Unmarshal(b, op); err != nil {
			return nil, err
		}
		if !proto.Equal(op.Destination, l.controlIdentity()) || op.GetOwner().GetConversation() != id || op.HandoverId == "" || op.Receipt == "" || op.Source == nil || op.Owner.Host == "" || op.Owner.Epoch == "" {
			return nil, status.Error(codes.FailedPrecondition, "resume journal selection mismatch")
		}
		// Routing can change on restart; the exact authenticated identity cannot.
		if op.DestinationEndpoint != r.DestinationEndpoint {
			op.DestinationEndpoint = r.DestinationEndpoint
			if err = saveResume(path, op); err != nil {
				return nil, err
			}
		}
		// A completed historical operation may be replaced for a new foreign
		// epoch. An incomplete operation is never silently abandoned.
		if v.State == "owned" && v.Owner != l.options.Host && (v.Owner != op.Owner.Host || v.Epoch != op.Owner.Epoch) {
			if _, e := conversation.HandoverResult(ctx, l.store, id, l.options.Host, op.HandoverId); e == nil {
				readErr = os.ErrNotExist
			}
		}
	}
	if readErr != nil && !os.IsNotExist(readErr) {
		return nil, readErr
	}
	if os.IsNotExist(readErr) {
		if v.State != "owned" || v.Owner == l.options.Host {
			return nil, acquireErr
		}
		if l.options.ResolveSource == nil || l.options.ControlDial == nil {
			return nil, status.Error(codes.FailedPrecondition, "authenticated source resolver unavailable")
		}
		source, endpoint, e := l.options.ResolveSource(ctx, v.Owner)
		if e != nil {
			return nil, e
		}
		if source == nil || source.Host != v.Owner || source.Store != l.options.StorePath || source.Contract != l.options.Contract.Contract || source.Schema != int32(l.options.Contract.SchemaVersion) || source.Build == "" || !controlLoopback(endpoint) {
			return nil, status.Error(codes.FailedPrecondition, "exact compatible source required")
		}
		var token [32]byte
		if _, err = rand.Read(token[:]); err != nil {
			return nil, err
		}
		op = &pb.LifecycleControlRequest{Target: source, Source: source, Destination: l.controlIdentity(), DestinationEndpoint: r.DestinationEndpoint, Owner: &pb.LifecycleOwner{Conversation: id, Host: v.Owner, Epoch: v.Epoch}, HandoverId: hex.EncodeToString(token[:]), Management: true}
		// Receipt and exact source identity are journaled before the first source mutation.
		a, e := s.AcceptIncoming(controlOwner(op), op.HandoverId, source.Build, l.options.Contract)
		if e != nil {
			return nil, e
		}
		op.Receipt = a.receipt
		if err = saveResume(path, op); err != nil {
			return nil, err
		}
		// Resolve again below and require the same authenticated exact identity.
		_ = endpoint
	}
	claimedEpoch, _ := conversation.HandoverResult(ctx, l.store, id, l.options.Host, op.HandoverId)
	localClaim := v.State == "owned" && v.Owner == l.options.Host && v.Epoch == claimedEpoch && claimedEpoch != ""
	if (!localClaim && v.State == "owned" && (v.Owner != op.Owner.Host || v.Epoch != op.Owner.Epoch)) || (v.State == "reserved" && (v.Owner != op.Owner.Host || v.Epoch != op.Owner.Epoch || v.Destination != l.options.Host || v.Handover != op.HandoverId)) || (v.State != "owned" && v.State != "reserved") {
		return nil, status.Error(codes.FailedPrecondition, "resume owner changed; refusing stale operation")
	}
	a, err := s.AcceptIncoming(controlOwner(op), op.HandoverId, op.Source.Build, l.options.Contract)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	a.receipt = op.Receipt
	l.mu.Unlock() // restore the durable acceptance after a destination restart
	claim := proto.Clone(op).(*pb.LifecycleControlRequest)
	claim.Target = op.Destination
	if localClaim {
		l.mu.Lock()
		owner, e := l.acquireUsing(ctx, id, a, nil, func() (*conversation.Ownership, error) {
			return conversation.RestoreClaimedHandover(ctx, l.store, id, l.options.Host, op.HandoverId)
		})
		l.mu.Unlock()
		if e != nil {
			return nil, e
		}
		return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: wireOwner(owner), Ready: true, OperationId: op.HandoverId}, nil
	}
	if v.State == "reserved" {
		return s.resumeClaim(ctx, claim)
	}
	if l.options.ResolveSource == nil || l.options.ControlDial == nil {
		return nil, status.Error(codes.FailedPrecondition, "authenticated source resolver unavailable")
	}
	source, endpoint, err := l.options.ResolveSource(ctx, op.Owner.Host)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(source, op.Source) {
		return nil, status.Error(codes.FailedPrecondition, "source identity changed")
	}
	conn, err := l.options.ControlDial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	client := pb.NewAgentClient(conn)
	check, err := client.LifecycleList(ctx, &pb.LifecycleControlRequest{Target: op.Source})
	if err != nil {
		return nil, err
	}
	if !proto.Equal(check.Identity, op.Source) {
		return nil, status.Error(codes.PermissionDenied, "authenticated source identity mismatch")
	}
	response, err := client.LifecycleForceRelinquish(ctx, op)
	if status.Code(err) == codes.Unimplemented {
		response, err = client.LifecycleRelinquish(ctx, op)
		if err != nil {
			// A lost response may already have durably reserved the exact transfer.
			current, e := conversation.ReadOwnership(ctx, l.store, id)
			if e == nil && current.State == "reserved" && current.Owner == op.Owner.Host && current.Epoch == op.Owner.Epoch && current.Destination == l.options.Host && current.Handover == op.HandoverId {
				return s.resumeClaim(ctx, claim)
			}
			if e != nil {
				return nil, e
			}
			if current.State != "owned" || current.Owner != op.Owner.Host || current.Epoch != op.Owner.Epoch {
				return nil, status.Error(codes.FailedPrecondition, "resume owner changed before legacy confirmation; refusing stale source")
			}
			if status.Code(err) == codes.Unknown || status.Code(err) == codes.FailedPrecondition || status.Code(err) == codes.Unimplemented {
				reason := err.Error()
				if l.options.LegacyStop != nil && op.LegacyProcessIdentity == "" {
					identity, inspectErr := l.options.LegacyStop.Prepare(ctx, op.Source)
					if inspectErr != nil {
						reason += "; safe stop unavailable: " + inspectErr.Error()
					} else if identity != "" {
						op.LegacyProcessIdentity = identity
						// Bind the requester shown when approval was offered too; a
						// later UI cannot substitute a host to bypass self-protection.
						if r.Requester != nil {
							op.Requester = proto.Clone(r.Requester).(*pb.LifecycleIdentity)
						}
						if err := saveResume(path, op); err != nil {
							return nil, err
						}
					}
				}
				if op.LegacyProcessIdentity == "" {
					reason += "; verified platform process stop unavailable; no stop authorized"
				}
				return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: op.Owner, Source: op.Source, OperationId: op.HandoverId, LegacyProcessIdentity: op.LegacyProcessIdentity, ConfirmationRequired: true, BlockedReason: reason}, nil
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if !proto.Equal(response.Identity, op.Source) || response.Receipt != op.Receipt {
		return nil, status.Error(codes.FailedPrecondition, "source receipt mismatch")
	}
	return s.resumeClaim(ctx, claim)
}
func (s *Server) resumeClaim(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	response, err := s.LifecycleClaim(ctx, r)
	if err == nil {
		l := s.coexistence
		l.mu.Lock()
		cap := l.owners[r.Owner.Conversation]
		ready := cap != nil && l.ready[ownerOf(cap)] && proto.Equal(wireOwner(ownerOf(cap)), response.Owner)
		l.mu.Unlock()
		v, e := conversation.ReadOwnership(ctx, l.store, r.Owner.Conversation)
		if e != nil {
			return nil, e
		}
		if !ready || v.State != "owned" || v.Owner != response.GetOwner().GetHost() || v.Epoch != response.GetOwner().GetEpoch() {
			return nil, status.Error(codes.FailedPrecondition, "resume claim is not the ready current epoch")
		}
		response.Ready = true
		response.OperationId = r.HandoverId
	}
	return response, err
}
