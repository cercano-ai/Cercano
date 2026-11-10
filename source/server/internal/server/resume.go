package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

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

// resumePublicationUncertain means rename succeeded but directory durability is
// unknown. The visible journal must remain recoverable; never discard acceptance.
type resumePublicationUncertain struct{ error }

func saveResume(path string, r *pb.LifecycleControlRequest) error {
	return saveResumeWithSync(path, r, nil)
}

func saveResumeWithSync(path string, r *pb.LifecycleControlRequest, syncDirectory func(*os.File) error) error {
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
	return syncResumePublication(path, syncDirectory)
}

// Both new publication and cross-host reaffirmation use the same directory
// durability barrier. A visible name alone is not proof of publication.
func syncResumePublication(path string, syncDirectory func(*os.File) error) error {
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return &resumePublicationUncertain{err}
	}
	defer d.Close()
	if syncDirectory == nil {
		syncDirectory = (*os.File).Sync
	}
	if err = syncDirectory(d); err != nil {
		return &resumePublicationUncertain{err}
	}
	return nil
}

// reaffirmResumeWithSync persists an already visible original journal without
// replacing it. In particular, a changed operation must never be overwritten by
// an old confirmation. Sync the file, parent and publication directory just as
// saveResumeWithSync does; preserve uncertain publications for exact retry.
func reaffirmResumeWithSync(path string, expected *pb.LifecycleControlRequest, syncDirectory func(*os.File) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	check := func() error {
		current, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !current.Mode().IsRegular() || current.Mode()&0077 != 0 || !os.SameFile(info, current) {
			return status.Error(codes.FailedPrecondition, "original journal publication changed")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var actual pb.LifecycleControlRequest
		if err = json.Unmarshal(b, &actual); err != nil {
			return err
		}
		if !proto.Equal(&actual, expected) {
			return status.Error(codes.FailedPrecondition, "original journal operation changed")
		}
		return nil
	}
	if err = check(); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
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
	if err = syncResumePublication(path, syncDirectory); err != nil {
		return err
	}
	return check()
}

// LifecycleResume runs on the launcher-selected coordinator, which may offer
// authenticated recovery at the exact original destination before normal resume.
// An unavailable RPC is never death proof. Legacy stops require a durable,
// exact approval and a trusted platform adapter.
func (s *Server) LifecycleResume(ctx context.Context, r *pb.LifecycleControlRequest) (result *pb.LifecycleControlResponse, finalErr error) {
	stage, refusalCheck := "control", ""
	var diagnosticLifecycle *coexistenceLifecycle
	mark := func() { _, _, line, _ := runtime.Caller(1); refusalCheck = fmt.Sprintf("resume.go:%d", line) }
	refusal := func(err error) error {
		_, _, line, _ := runtime.Caller(1)
		refusalCheck = fmt.Sprintf("resume.go:%d", line)
		return err
	}
	defer func() {
		if finalErr != nil {
			s.logLifecycleResumeRefusal(diagnosticLifecycle, r, stage, refusalCheck, finalErr)
		}
	}()
	l, err := s.control(ctx, r)
	if err != nil {
		return nil, refusal(err)
	}
	diagnosticLifecycle = l
	stage = "validate_conversation"
	id := r.GetOwner().GetConversation()
	if id == "" {
		return nil, refusal(status.Error(codes.InvalidArgument, "conversation required"))
	}
	l.resumeMu.Lock()
	defer l.resumeMu.Unlock()
	// Confirmation is a single-use recovery continuation, never another
	// cooperative attempt (which could change the consent-bound row).
	_, containedRecovery := l.options.LegacyStop.(ContainedSourceStop)
	if containedRecovery && (r.RecoveryChallenge != "" || r.ConfirmOriginalRecovery) {
		recovered, handled, e := s.liveSourceRecovery(ctx, l, r)
		if !handled {
			return nil, status.Error(codes.FailedPrecondition, "recovery confirmation no longer matches current ownership")
		}
		return recovered, e
	}
	// Try ordinary acquisition, exact reservation claims and supported cooperation
	// first. Historical journals are not an ownership-discovery prerequisite.
	// Only qualified containment may turn a failure into a fresh stop offer.
	if !containedRecovery {
		// Keep the existing receipt-based coordinator for installations without
		// qualified host containment. The compatible production path below must
		// not depend on these newer RPCs or on historical journal uniqueness.
		if offered, handled, e := s.offerTakeover(ctx, l, r); handled {
			return offered, e
		}
		stage = "original_operation_discovery"
		if recovered, handled, e := s.originalRecovery(ctx, l, r); handled {
			return recovered, e
		}
	}
	if containedRecovery {
		defer func() {
			if finalErr == nil && result.GetReady() {
				return
			}
			if ctx.Err() != nil {
				return
			}
			recovered, handled, e := s.liveSourceRecovery(ctx, l, r)
			if handled {
				result, finalErr = recovered, e
			}
		}()
	}
	var approved *pb.LifecycleControlRequest
	if r.ConfirmLegacyStop {
		stage = "validate_legacy_approval"
		approved, err = validateLegacyApproval(l, r)
		if err != nil {
			return nil, refusal(err)
		}
	}
	stage = "acquire"
	acquired, acquireErr := s.LifecycleAcquire(ctx, r)
	if acquireErr == nil {
		acquired.Ready = true
		acquired.CurrentOwner = true
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
	stage = "read_ownership"
	v, err := conversation.ReadOwnership(ctx, l.store, id)
	if err != nil {
		return nil, refusal(err)
	}
	stage = "confirmed_legacy"
	if approved != nil {
		// If a response was lost after the stop, guarded recovery can finish
		// without signalling again. Never retry a signal against a changed PID.
		if l.options.RecoveryGuard != nil {
			if inspected, e := s.LifecycleRecover(ctx, &pb.LifecycleRecoverRequest{Target: r.Target, Conversation: id}); e == nil && proto.Equal(inspected.Owner, approved.Owner) {
				mark()
				return s.resumeRecover(ctx, &pb.LifecycleRecoverRequest{Target: r.Target, Conversation: id, ExpectedOwner: approved.Owner, Confirm: true})
			}
		}
		mark()
		return s.resumeConfirmedLegacy(ctx, r, approved)
	}
	// Recovery repeats both the guarded death check and exact owner/epoch check.
	stage = "guarded_recovery"
	if v.State == "owned" && v.Owner != l.options.Host && l.options.RecoveryGuard != nil {
		inspected, e := s.LifecycleRecover(ctx, &pb.LifecycleRecoverRequest{Target: r.Target, Conversation: id})
		if e == nil {
			mark()
			return s.resumeRecover(ctx, &pb.LifecycleRecoverRequest{Target: r.Target, Conversation: id, ExpectedOwner: inspected.Owner, Confirm: true})
		}
	}
	stage = "validate_destination"
	if !controlLoopback(r.DestinationEndpoint) {
		return nil, refusal(status.Error(codes.InvalidArgument, "selected destination endpoint required"))
	}
	stage = "resume_journal"
	path := resumeJournal(l, id)
	op := new(pb.LifecycleControlRequest)
	b, readErr := os.ReadFile(path)
	if readErr == nil {
		if err = json.Unmarshal(b, op); err != nil {
			return nil, refusal(err)
		}
		if !proto.Equal(op.Destination, l.controlIdentity()) || op.GetOwner().GetConversation() != id || op.HandoverId == "" || op.Receipt == "" || op.Source == nil || op.Owner.Host == "" || op.Owner.Epoch == "" {
			return nil, refusal(status.Error(codes.FailedPrecondition, "resume journal selection mismatch"))
		}
		// Routing can change on restart; the exact authenticated identity cannot.
		op.DestinationEndpoint = r.DestinationEndpoint
		// Reaffirm durability before source mutation, including a retry after
		// rename succeeded but the previous directory sync was uncertain.
		if err = saveResumeWithSync(path, op, l.resumeJournalSync); err != nil {
			return nil, refusal(err)
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
		return nil, refusal(readErr)
	}
	stage = "new_resume_operation"
	if os.IsNotExist(readErr) {
		if v.State != "owned" || v.Owner == l.options.Host {
			stage = "acquire"
			return nil, refusal(acquireErr)
		}
		if l.options.ResolveSource == nil || l.options.ControlDial == nil {
			return nil, refusal(status.Error(codes.FailedPrecondition, "authenticated source resolver unavailable"))
		}
		source, endpoint, e := l.options.ResolveSource(ctx, v.Owner)
		if e != nil {
			return nil, refusal(e)
		}
		if source == nil || source.Host != v.Owner || source.Store != l.options.StorePath || source.Contract != l.options.Contract.Contract || source.Schema != int32(l.options.Contract.SchemaVersion) || source.Build == "" || !controlLoopback(endpoint) {
			return nil, refusal(status.Error(codes.FailedPrecondition, "exact compatible source required"))
		}
		var token [32]byte
		if _, err = rand.Read(token[:]); err != nil {
			return nil, refusal(err)
		}
		op = &pb.LifecycleControlRequest{Target: source, Source: source, Destination: l.controlIdentity(), DestinationEndpoint: r.DestinationEndpoint, Owner: &pb.LifecycleOwner{Conversation: id, Host: v.Owner, Epoch: v.Epoch}, HandoverId: hex.EncodeToString(token[:]), Management: true}
		// Receipt and exact source identity are journaled before the first source mutation.
		a, e := s.AcceptIncoming(controlOwner(op), op.HandoverId, source.Build, l.options.Contract)
		if e != nil {
			return nil, refusal(e)
		}
		op.Receipt = a.receipt
		if err = saveResumeWithSync(path, op, l.resumeJournalSync); err != nil {
			var uncertain *resumePublicationUncertain
			if !errors.As(err, &uncertain) {
				// No source mutation or published receipt is possible on this path.
				l.mu.Lock()
				cleanupErr := l.resolveResumeAcceptanceLocked(a)
				l.mu.Unlock()
				if cleanupErr != nil {
					return nil, refusal(fmt.Errorf("%w; acceptance cleanup: %v", err, cleanupErr))
				}
			}
			return nil, refusal(err)
		}
		// Resolve again below and require the same authenticated exact identity.
		_ = endpoint
	}
	stage = "validate_operation_owner"
	claimedEpoch, _ := conversation.HandoverResult(ctx, l.store, id, l.options.Host, op.HandoverId)
	localClaim := v.State == "owned" && v.Owner == l.options.Host && v.Epoch == claimedEpoch && claimedEpoch != ""
	if (!localClaim && v.State == "owned" && (v.Owner != op.Owner.Host || v.Epoch != op.Owner.Epoch)) || (v.State == "reserved" && (v.Owner != op.Owner.Host || v.Epoch != op.Owner.Epoch || v.Destination != l.options.Host || v.Handover != op.HandoverId)) || (v.State != "owned" && v.State != "reserved") {
		return nil, refusal(status.Error(codes.FailedPrecondition, "resume owner changed; refusing stale operation"))
	}
	stage = "accept_incoming"
	a, err := s.AcceptIncoming(controlOwner(op), op.HandoverId, op.Source.Build, l.options.Contract)
	if err != nil {
		return nil, refusal(err)
	}
	l.mu.Lock()
	a.receipt = op.Receipt
	l.mu.Unlock() // restore the durable acceptance after a destination restart
	claim := proto.Clone(op).(*pb.LifecycleControlRequest)
	claim.Target = op.Destination
	stage = "restore_local_claim"
	if localClaim {
		l.mu.Lock()
		owner, e := l.acquireUsing(ctx, id, a, nil, func() (*conversation.Ownership, error) {
			return conversation.RestoreClaimedHandover(ctx, l.store, id, l.options.Host, op.HandoverId)
		})
		l.mu.Unlock()
		if e != nil {
			return nil, refusal(e)
		}
		return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: wireOwner(owner), Ready: true, OperationId: op.HandoverId}, nil
	}
	stage = "claim_reserved"
	if v.State == "reserved" {
		mark()
		return s.resumeClaim(ctx, claim)
	}
	if l.options.ResolveSource == nil || l.options.ControlDial == nil {
		return nil, refusal(status.Error(codes.FailedPrecondition, "authenticated source resolver unavailable"))
	}
	stage = "resolve_source"
	source, endpoint, err := l.options.ResolveSource(ctx, op.Owner.Host)
	if err != nil {
		return nil, refusal(err)
	}
	if !proto.Equal(source, op.Source) {
		return nil, refusal(status.Error(codes.FailedPrecondition, "source identity changed"))
	}
	stage = "dial_source"
	conn, err := l.options.ControlDial(ctx, endpoint, source)
	if err != nil {
		return nil, refusal(err)
	}
	defer conn.Close()
	client := pb.NewAgentClient(conn)
	stage = "check_source"
	check, err := client.LifecycleList(ctx, &pb.LifecycleControlRequest{Target: op.Source})
	if err != nil {
		return nil, refusal(err)
	}
	if !proto.Equal(check.Identity, op.Source) {
		return nil, refusal(status.Error(codes.PermissionDenied, "authenticated source identity mismatch"))
	}
	stage = "force_relinquish"
	response, err := client.LifecycleForceRelinquish(ctx, op)
	if status.Code(err) == codes.Unimplemented {
		stage = "legacy_relinquish"
		response, err = client.LifecycleRelinquish(ctx, op)
		if err != nil {
			// A lost response may already have durably reserved the exact transfer.
			current, e := conversation.ReadOwnership(ctx, l.store, id)
			if e == nil && current.State == "reserved" && current.Owner == op.Owner.Host && current.Epoch == op.Owner.Epoch && current.Destination == l.options.Host && current.Handover == op.HandoverId {
				mark()
				return s.resumeClaim(ctx, claim)
			}
			if e != nil {
				return nil, refusal(e)
			}
			if current.State != "owned" || current.Owner != op.Owner.Host || current.Epoch != op.Owner.Epoch {
				return nil, refusal(status.Error(codes.FailedPrecondition, "resume owner changed before legacy confirmation; refusing stale source"))
			}
			if status.Code(err) == codes.Unknown || status.Code(err) == codes.FailedPrecondition || status.Code(err) == codes.Unimplemented {
				reason := err.Error()
				op.LegacyProcessIdentity = "" // Every new offer requires a fresh process snapshot.
				if l.options.LegacyStop != nil {
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
							return nil, refusal(err)
						}
					}
				}
				if err := saveResume(path, op); err != nil {
					return nil, refusal(err)
				}
				if op.LegacyProcessIdentity == "" {
					reason += "; verified platform process stop unavailable; no stop authorized"
				}
				return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: op.Owner, Source: op.Source, OperationId: op.HandoverId, LegacyProcessIdentity: op.LegacyProcessIdentity, ConfirmationRequired: true, BlockedReason: reason}, nil
			}
		}
	}
	if err != nil {
		return nil, refusal(err)
	}
	stage = "validate_source_receipt"
	if !proto.Equal(response.Identity, op.Source) || response.Receipt != op.Receipt {
		return nil, refusal(status.Error(codes.FailedPrecondition, "source receipt mismatch"))
	}
	mark()
	stage = "claim"
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

// resolveResumeAcceptanceLocked removes only the supplied exact in-process proof.
// Callers must prove publication never happened or its source epoch is superseded.
func (l *coexistenceLifecycle) resolveResumeAcceptanceLocked(a *IncomingAcceptance) error {
	if a == nil || l.incoming[a.h] != a {
		return nil
	}
	if err := l.admission.ResolveIncomingDurably(a.h); err != nil {
		return err
	}
	delete(l.incoming, a.h)
	return nil
}

// resumeRecover is called under resumeMu: no negotiation for this journal can be
// in flight. A successful guarded exact-epoch recovery fences every late source
// reservation for that epoch. Resolve only this journal's exact acceptance, not
// unrelated participation (including other handovers for the same conversation).
func (s *Server) resumeRecover(ctx context.Context, r *pb.LifecycleRecoverRequest) (*pb.LifecycleControlResponse, error) {
	l := s.coexistence
	var op pb.LifecycleControlRequest
	b, err := os.ReadFile(resumeJournal(l, r.Conversation))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err = json.Unmarshal(b, &op); err != nil {
			return nil, err
		}
	}
	got, err := s.LifecycleRecover(ctx, r)
	if err != nil || !got.GetReady() {
		return got, err
	}
	if !proto.Equal(op.Destination, l.controlIdentity()) || !proto.Equal(op.Owner, r.ExpectedOwner) || op.HandoverId == "" || op.Receipt == "" || op.Source == nil || op.Source.Host != r.ExpectedOwner.GetHost() {
		return got, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for h, a := range l.incoming {
		if h.Conversation == op.Owner.Conversation && h.SourceHost == op.Owner.Host && h.SourceEpoch == op.Owner.Epoch && h.HandoverID == op.HandoverId && h.DestinationHost == l.options.Host && a.receipt == op.Receipt && a.sourceBuild == op.Source.Build {
			if err = l.resolveResumeAcceptanceLocked(a); err != nil {
				return nil, err
			}
		}
	}
	return got, nil
}
