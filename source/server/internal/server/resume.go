package server

import (
	"cercano/source/server/internal/broker"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/recoveryguard"
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

// LifecycleResume runs on the launcher-selected coordinator, which may offer
// authenticated recovery at the exact original destination before normal resume.
// An unavailable RPC is never death proof. Legacy client stop confirmations
// are rejected; only fresh contained recovery consent can authorize a stop.
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
	// Reject obsolete consent before acquisition, journals or recovery fallback.
	// Even a mixed old/new confirmation must never acquire modern authority.
	if r.GetConfirmLegacyStop() {
		return nil, refusal(status.Error(codes.FailedPrecondition, "legacy stop confirmation is no longer supported; upgrade your client and reopen the conversation for fresh recovery confirmation; no stop authorized"))
	}
	stage = "validate_conversation"
	id := r.GetOwner().GetConversation()
	if id == "" {
		return nil, refusal(status.Error(codes.InvalidArgument, "conversation required"))
	}
	l.resumeMu.Lock()
	defer l.resumeMu.Unlock()
	// Recovery commits its receipt before publishing readiness. Reconcile on
	// successful retries too, including restart after commit but before cleanup.
	defer func() {
		if finalErr == nil && result.GetReady() {
			if _, e := l.reconcileRecoveredResume(ctx, id); e != nil {
				result, finalErr = nil, e
			}
		}
	}()
	// Confirmation is a single-use recovery continuation, never another
	// cooperative attempt (which could change the consent-bound row).
	_, containedRecovery := l.options.RecoverySource.(ContainedSourceStop)
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
	// Ordinary execution returns an explicit discovery-eligible refusal only at
	// known non-authoritative boundaries. Every other error is terminal.
	// Guard uncertainty blocks discovery, not authenticated cooperation or an
	// exact reservation claim. Only a typed live-source refusal permits fallback.
	var guardFailure error
	ordinary := func() (*pb.LifecycleControlResponse, error) {
		eligible := func(err error) error {
			if guardFailure != nil {
				stage = "guarded_recovery"
				return refusal(guardFailure)
			}
			return &resumeDiscoveryEligible{refusal(err)}
		}
		unavailable := func(err error) error {
			if errors.Is(err, os.ErrNotExist) || status.Code(err) == codes.NotFound || status.Code(err) == codes.Unavailable {
				return eligible(err)
			}
			return refusal(err)
		}
		stage = "acquire"
		acquired, acquireErr := s.LifecycleAcquire(ctx, r)
		if acquireErr == nil {
			acquired.Ready = true
			acquired.CurrentOwner = true
			// A ready retry still reports the same durable operation, if this
			// acquisition came from resume rather than ordinary unowned acquire.
			stage = "ready_journal"
			if b, e := os.ReadFile(resumeJournal(l, id)); e == nil {
				var op pb.LifecycleControlRequest
				if e = json.Unmarshal(b, &op); e != nil {
					return nil, refusal(e)
				}
				if !proto.Equal(op.Destination, l.controlIdentity()) || op.GetOwner().GetConversation() != id || op.HandoverId == "" {
					return nil, refusal(status.Error(codes.FailedPrecondition, "resume journal selection mismatch"))
				}
				epoch, e := conversation.HandoverResult(ctx, l.store, id, l.options.Host, op.HandoverId)
				if e != nil && !errors.Is(e, sql.ErrNoRows) {
					return nil, refusal(e)
				}
				if e == nil && epoch == acquired.GetOwner().GetEpoch() {
					acquired.OperationId = op.HandoverId
				}
			} else if !os.IsNotExist(e) {
				return nil, refusal(e)
			}

			// Contained recovery has no cooperative journal. Restore its durable,
			// source-qualified receipt only after acquisition established readiness.
			// Never replace cooperative metadata or use a receipt as authority.
			if acquired.OperationId == "" {
				stage = "recovery_receipt"
				operation, e := conversation.CurrentRecoveryResult(ctx, l.store, id, l.options.Host, acquired.GetOwner().GetEpoch())
				if e != nil && !errors.Is(e, sql.ErrNoRows) {
					return nil, refusal(e)
				}
				acquired.OperationId = operation
			}
			return acquired, nil
		}
		var conflict *conversation.OwnershipConflict
		if !errors.As(acquireErr, &conflict) {
			return nil, refusal(acquireErr)
		}
		stage = "read_ownership"
		v, err := conversation.ReadOwnership(ctx, l.store, id)
		if err != nil {
			return nil, refusal(err)
		}
		// Cleanup is independent of readiness: after a crash, a recovered local
		// row still requires explicit ownership restoration/recovery. Its durable
		// receipt can nevertheless resolve the obsolete cooperative participation.
		if _, e := l.reconcileRecoveredResume(ctx, id); e != nil {
			return nil, refusal(e)
		}
		// Recovery repeats both the guarded death check and exact owner/epoch check.
		stage = "guarded_recovery"
		if v.State == "owned" && v.Owner != l.options.Host && l.options.RecoveryGuard != nil {
			inspected, e := s.LifecycleRecover(ctx, &pb.LifecycleRecoverRequest{Target: r.Target, Conversation: id})
			if e == nil {
				mark()
				return s.resumeRecover(ctx, &pb.LifecycleRecoverRequest{Target: r.Target, Conversation: id, ExpectedOwner: inspected.Owner, Confirm: true})
			}
			if !errors.Is(e, recoveryguard.ErrLiveSource) {
				guardFailure = e
			}
		}
		stage = "validate_destination"
		if r.DestinationEndpoint != "" && !controlLoopback(r.DestinationEndpoint) {
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
				} else if !errors.Is(e, sql.ErrNoRows) {
					return nil, refusal(e)
				} else if recovered, e := l.reconcileRecoveredResume(ctx, id); e != nil {
					return nil, refusal(e)
				} else if recovered {
					readErr = os.ErrNotExist
				}
			}
		}
		if readErr != nil && !os.IsNotExist(readErr) {
			return nil, refusal(readErr)
		}
		if r.DestinationEndpoint == "" {
			return nil, eligible(status.Error(codes.InvalidArgument, "selected destination endpoint required"))
		}
		stage = "new_resume_operation"
		if os.IsNotExist(readErr) {
			if v.State != "owned" || v.Owner == l.options.Host {
				stage = "acquire"
				if v.State == "reserved" && v.Owner != l.options.Host {
					return nil, eligible(acquireErr)
				}
				return nil, refusal(acquireErr)
			}
			if l.options.ResolveSource == nil || l.options.ControlDial == nil {
				return nil, eligible(status.Error(codes.FailedPrecondition, "authenticated source resolver unavailable"))
			}
			source, endpoint, e := l.options.ResolveSource(ctx, v.Owner)
			if e != nil {
				return nil, unavailable(e)
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
		claimedEpoch, lookupErr := conversation.HandoverResult(ctx, l.store, id, l.options.Host, op.HandoverId)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
			return nil, refusal(lookupErr)
		}
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
			return nil, eligible(status.Error(codes.FailedPrecondition, "authenticated source resolver unavailable"))
		}
		stage = "resolve_source"
		source, endpoint, err := l.options.ResolveSource(ctx, op.Owner.Host)
		if err != nil {
			return nil, unavailable(err)
		}
		if !proto.Equal(source, op.Source) {
			return nil, refusal(status.Error(codes.FailedPrecondition, "source identity changed"))
		}
		stage = "dial_source"
		conn, err := l.options.ControlDial(ctx, endpoint, source)
		if err != nil {
			return nil, unavailable(err)
		}
		defer conn.Close()
		client := pb.NewAgentClient(conn)
		stage = "check_source"
		check, err := client.LifecycleList(ctx, &pb.LifecycleControlRequest{Target: op.Source})
		if err != nil {
			return nil, unavailable(err)
		}
		if !proto.Equal(check.Identity, op.Source) {
			return nil, refusal(status.Error(codes.PermissionDenied, "authenticated source identity mismatch"))
		}
		stage = "force_relinquish"
		cooperative, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		response, err := client.LifecycleForceRelinquish(cooperative, op)
		if status.Code(err) == codes.Unimplemented {
			stage = "legacy_relinquish"
			response, err = client.LifecycleRelinquish(cooperative, op)
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
					return nil, refusal(status.Error(codes.FailedPrecondition, "resume owner changed during cooperative transfer; refusing stale source"))
				}

				// Legacy failures use the same exact busy/timeout classifier
				// below; FailedPrecondition alone grants no stop authority.
			}
		}
		if err != nil {
			// Only the exact historical admission refusal or a bounded source
			// timeout permits a new confirmation, never an auth/identity error.
			busy := status.Code(err) == codes.Unknown && status.Convert(err).Message() == broker.ErrAdmissionBusy.Error()
			if busy || status.Code(err) == codes.DeadlineExceeded {
				current, e := conversation.ReadOwnership(ctx, l.store, id)
				if e != nil {
					return nil, refusal(e)
				}
				if current.State == "reserved" && current.Owner == op.Owner.Host && current.Epoch == op.Owner.Epoch && current.Destination == l.options.Host && current.Handover == op.HandoverId {
					return s.resumeClaim(ctx, claim)
				}
				if current.State != "owned" || current.Owner != op.Owner.Host || current.Epoch != op.Owner.Epoch {
					return nil, refusal(status.Error(codes.FailedPrecondition, "resume owner changed during cooperative transfer"))
				}
				return nil, eligible(err)
			}
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
	result, finalErr = ordinary()
	var discovery *resumeDiscoveryEligible
	if !errors.As(finalErr, &discovery) {
		return result, finalErr
	}
	finalErr = discovery.error
	if !containedRecovery || ctx.Err() != nil || errors.Is(finalErr, context.Canceled) || errors.Is(finalErr, context.DeadlineExceeded) {
		return result, finalErr
	}
	// Preserve the ordinary cause even when discovery succeeds or fails with a
	// different cause. The logger records only stage/check/code/classification.
	s.logLifecycleResumeRefusal(l, r, stage, refusalCheck, finalErr)
	stage, refusalCheck = "containment_discovery", ""
	recovered, handled, recoveryErr := s.liveSourceRecovery(ctx, l, r)
	if handled {
		return recovered, recoveryErr
	}
	return result, finalErr
}

// resumeDiscoveryEligible is local resume policy, not stop authority. It never
// crosses the RPC boundary; discovery still requires fresh separate consent.
type resumeDiscoveryEligible struct{ error }

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

// reconcileRecoveredResume runs under resumeMu. A historical transactional
// recovery receipt proves only that this exact source epoch was superseded at
// this destination; it grants neither ownership nor readiness. Keep the journal
// so a crash/retry can repeat cleanup, and reaffirm publication before resolving
// any in-process acceptance. Current ownership alone is never evidence.
func (l *coexistenceLifecycle) reconcileRecoveredResume(ctx context.Context, id string) (bool, error) {
	path := resumeJournal(l, id)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var op pb.LifecycleControlRequest
	if err = json.Unmarshal(b, &op); err != nil {
		return false, err
	}
	if !proto.Equal(op.Destination, l.controlIdentity()) || op.GetOwner().GetConversation() != id {
		return false, status.Error(codes.FailedPrecondition, "resume journal selection mismatch")
	}
	// A completed cooperative journal need not contain an acceptance. It is
	// not recovery proof, and must retain its existing completed-claim behavior.
	if op.HandoverId == "" || op.Receipt == "" || op.Source == nil || op.Owner.Host == "" || op.Owner.Epoch == "" || op.Source.Host != op.Owner.Host {
		return false, nil
	}
	recovery := conversation.RecoveryID(id, conversation.OwnershipState{Owner: op.Owner.Host, Epoch: op.Owner.Epoch}, l.options.Host)
	if _, err = conversation.HandoverResult(ctx, l.store, id, l.options.Host, recovery); errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if err = syncResumePublication(path, l.resumeJournalSync); err != nil {
		return false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for h, a := range l.incoming {
		if h.Conversation == id && h.SourceHost == op.Owner.Host && h.SourceEpoch == op.Owner.Epoch && h.HandoverID == op.HandoverId && h.DestinationHost == l.options.Host && a.receipt == op.Receipt && a.sourceBuild == op.Source.Build {
			if err = l.resolveResumeAcceptanceLocked(a); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}

// The shared successful-resume reconciliation also covers guarded recovery.
func (s *Server) resumeRecover(ctx context.Context, r *pb.LifecycleRecoverRequest) (*pb.LifecycleControlResponse, error) {
	return s.LifecycleRecover(ctx, r)
}
