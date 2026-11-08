package server

import (
	"errors"
	"os"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/failurelog"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/status"
)

// lifecycleBlockers is local, observational accounting, not an ownership check
// against the store. There is deliberately no RPC and no receipt observation.
type lifecycleBlockers struct {
	Owner            broker.Owner
	Known            bool
	Ready            bool
	Reason           string
	AcquisitionStage string
	Admission        broker.AdmissionSnapshot
}

// snapshotBlockers requires an exact capability identity. Lock ordering matches
// lifecycle operations: lifecycle mu then the admission leaf lock. No I/O, hooks,
// receipt observation, or state updates are performed here.
func (l *coexistenceLifecycle) snapshotBlockers(o broker.Owner) lifecycleBlockers {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.snapshotBlockersLocked(o)
}
func (l *coexistenceLifecycle) snapshotBlockersLocked(o broker.Owner) lifecycleBlockers {
	out := lifecycleBlockers{Owner: o, Reason: "unknown_or_mismatched_owner"}
	cap := l.owners[o.Conversation]
	if cap == nil || ownerOf(cap) != o {
		return out
	}
	out.Known = true
	out.Ready = l.ready[o]
	out.AcquisitionStage = l.acquisitionStage[o]
	out.Admission = l.admission.Snapshot(o)
	switch {
	case !out.Ready:
		out.Reason = "acquisition_unresolved"
	case out.Admission.Retired:
		out.Reason = "host_retired"
	case !out.Admission.Retained:
		out.Reason = "admission_owner_missing"
	case out.Admission.Closed:
		out.Reason = "admission_closed"
	case len(out.Admission.Work) > 0:
		out.Reason = "session_work"
	case out.Admission.Attachments > 0:
		out.Reason = "attachments"
	case len(out.Admission.HostWork) > 0:
		out.Reason = "host_work"
	default:
		out.Reason = "ready"
	}
	return out
}

func (s *Server) logLifecycleResumeRefusal(l *coexistenceLifecycle, r *pb.LifecycleControlRequest, stage, check string, err error) {
	if s.failureLog == nil {
		return
	}
	kind := classifyResumeRPCError(err)
	switch {
	case errors.Is(err, broker.ErrAdmissionBusy):
		kind = "admission_busy"
	case errors.Is(err, broker.ErrAdmissionStale):
		kind = "admission_stale"
	}
	fields := failurelog.Event{"scope": "lifecycle", "operation": "resume", "stage": stage, "check": check, "grpc_code": status.Code(err).String(), "error_kind": kind, "pid": os.Getpid()}
	// l is non-nil only after exact target selection and authentication succeeded.
	// Do not log even caller-supplied identifiers on a denied control request.
	if l != nil {
		l.mu.Lock()
		requested := controlOwner(r)
		if requested.Host == "" && requested.Epoch == "" {
			if cap := l.owners[requested.Conversation]; cap != nil {
				requested = ownerOf(cap)
			}
		}
		snapshot := l.snapshotBlockersLocked(requested)
		l.mu.Unlock()
		fields["conversation_id"] = requested.Conversation
		fields["blockers"] = snapshot
	}
	// Raw errors, endpoints, store paths, receipts, and request bodies are never logged.
	s.failureLog.Log("lifecycle.resume_refused", fields)
}
