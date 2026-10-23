package server

import (
	"context"
	"errors"
	"strings"
	"sync"

	"cercano/source/server/pkg/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// updateAdmission counts actual admitted lifetimes, not broker generations or
// connected observers. Its zero value is ready. The drain RPC checks background coverage before arming it.
type updateAdmission struct {
	mu              sync.Mutex
	active          uint64
	drainGeneration uint64
	paused          bool
	draining        bool
	changed         chan struct{}
}

var errUpdateAdmissionPaused = errors.New("agent update preparation is active; retry shortly")

func (g *updateAdmission) notifyLocked() {
	if g.changed != nil {
		close(g.changed)
	}
	g.changed = make(chan struct{})
}
func (g *updateAdmission) enter() (func(), error) {
	g.mu.Lock()
	if g.paused {
		g.mu.Unlock()
		return nil, errUpdateAdmissionPaused
	}
	g.active++
	g.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { g.mu.Lock(); g.active--; g.notifyLocked(); g.mu.Unlock() }) }, nil
}

// updateWorkOwner is a gate-bound, revocable continuation capability. It is
// never inferred from a global active count and cannot cross gate instances.
type updateWorkOwner struct {
	gate *updateAdmission
	live bool
}
type updateWorkOwnerKey struct{}

func (g *updateAdmission) enterContext(ctx context.Context) (context.Context, func(), error) {
	g.mu.Lock()
	owner, _ := ctx.Value(updateWorkOwnerKey{}).(*updateWorkOwner)
	continuation := owner != nil && owner.gate == g && owner.live
	if (owner != nil && !continuation) || (g.paused && !(g.draining && continuation)) {
		g.mu.Unlock()
		return ctx, nil, errUpdateAdmissionPaused
	}
	child := &updateWorkOwner{gate: g, live: true}
	g.active++
	g.mu.Unlock()
	var once sync.Once
	return context.WithValue(ctx, updateWorkOwnerKey{}, child), func() {
		once.Do(func() {
			g.mu.Lock()
			child.live = false
			g.active--
			g.notifyLocked()
			g.mu.Unlock()
		})
	}, nil
}

// drain seals roots immediately. The context owns the seal even after idle is
// returned; cancellation or explicit release resumes admission. Work is never
// cancelled here, and its count retires only when each real lifetime returns.
// Optional sealed notifications run after exclusion, before waiting, without
// the mutex held. Notification failure releases this owner's seal.
func (g *updateAdmission) drain(ctx context.Context, sealed ...func() error) (func(), error) {
	return g.drainObserved(ctx, nil, sealed...)
}

func (g *updateAdmission) drainObserved(ctx context.Context, observe func(uint64, proto.UpdateReadiness_Phase), sealed ...func() error) (func(), error) {
	g.mu.Lock()
	if err := ctx.Err(); err != nil {
		g.mu.Unlock()
		return nil, err
	}
	if g.paused {
		g.mu.Unlock()
		return nil, errUpdateAdmissionPaused
	}
	g.drainGeneration++
	generation := g.drainGeneration
	notice := func(phase proto.UpdateReadiness_Phase) {
		if observe != nil {
			observe(generation, phase)
		}
	}
	g.paused = true
	g.draining = true
	g.notifyLocked()
	g.mu.Unlock()
	// Serialize observation with release, independently of the admission mutex.
	var observationMu sync.Mutex
	released := false
	observeLive := func(phase proto.UpdateReadiness_Phase) {
		observationMu.Lock()
		defer observationMu.Unlock()
		if !released {
			notice(phase)
		}
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			observationMu.Lock()
			defer observationMu.Unlock()
			released = true
			// Keep ownership sealed until the published live state is withdrawn.
			notice(proto.UpdateReadiness_RELEASED)
			g.mu.Lock()
			g.paused = false
			g.draining = false
			g.notifyLocked()
			g.mu.Unlock()
		})
	}
	stop := context.AfterFunc(ctx, release)
	observeLive(proto.UpdateReadiness_DRAINING)
	ownedRelease := func() { stop(); release() }
	for _, notify := range sealed {
		if err := notify(); err != nil {
			ownedRelease()
			return nil, err
		}
	}
	for {
		g.mu.Lock()
		if err := ctx.Err(); err != nil {
			g.mu.Unlock()
			ownedRelease()
			return nil, err
		}
		if g.active == 0 {
			g.mu.Unlock()
			observeLive(proto.UpdateReadiness_IDLE)
			return ownedRelease, nil
		}
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			ownedRelease()
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

// pauseWhenIdle does not cancel work or freeze nested calls while work exists.
// Once zero is observed under the admission mutex, new work is refused atomically.
// Cancellation while waiting leaves normal admission untouched. The successful
// caller owns release; its eventual stream/process lifetime will govern release.
func (g *updateAdmission) pauseWhenIdle(ctx context.Context) (func(), error) {
	for {
		g.mu.Lock()
		if err := ctx.Err(); err != nil {
			g.mu.Unlock()
			return nil, err
		}
		if g.paused {
			g.mu.Unlock()
			return nil, errUpdateAdmissionPaused
		}
		if g.active == 0 {
			g.paused = true
			g.mu.Unlock()
			var once sync.Once
			return func() { once.Do(func() { g.mu.Lock(); g.paused = false; g.notifyLocked(); g.mu.Unlock() }) }, nil
		}
		if g.changed == nil {
			g.changed = make(chan struct{})
		}
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

// sealForForce refuses new work immediately without pretending active work is
// idle. It is permanent for process shutdown; canceled pre-commit requests and
// competing gate owners leave admission unchanged.
func (g *updateAdmission) sealForForce(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if g.paused {
		return errUpdateAdmissionPaused
	}
	g.paused = true
	g.notifyLocked()
	return nil
}

func (s *Server) admitUpdateWork(ctx context.Context) (func(), error) {
	_, release, err := s.updateWork.enterContext(ctx)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	return release, nil
}

// agentFullMethodPrefix derives the Agent service's gRPC full-method prefix
// from the generated Agent_ServiceDesc — the actual registered service name,
// never a guessed string — so admission coverage stays pinned to the real
// service identity if the proto is ever regenerated. Non-Agent services
// (worker accounting, anything else registered on the same gRPC server) never
// match the prefix and are left untouched.
func agentFullMethodPrefix() string {
	return "/" + proto.Agent_ServiceDesc.ServiceName + "/"
}

// updateAdmissionExemptUnary lists the DOCUMENTED administrative Agent unary
// methods that stay callable while update preparation holds the pause. Both
// entries are stop-path methods whose handlers never pin a work lifetime, so
// admitting them cannot defeat the idle observation; the list must not grow
// casually.
//
//   - ShutdownAgent (unchanged legacy bounce): its handler only schedules the
//     process's normal stop (scheduleSelfShutdown — configured stop path or
//     self-SIGTERM fallback), so refusing it under the pause would strand the
//     paused process with no way for a client to restart it.
//   - ShutdownAgentWhenIdle (safe stop, see safe_stop.go): its CONTRACT is to
//     wait for the fully observed idle gate itself — a second lifetime here
//     would make its own wait never go idle (deadlock). Exempting it also
//     keeps idempotent repeat requests servable while the committed seal
//     holds, so clients can confirm a committed stop. The commit deliberately
//     leaves admission sealed through the actual process exit; only the
//     passive observer streams below continue serving new work.
//
// Deliberately NOT exempt: RestartRuntime and RestartMcpServer perform actual
// component work (stopping/starting inference instances; synchronous MCP child
// reconnects), which is exactly the work class update preparation waits for.
// GetAgentInfo does not exist in this proto (there is no such RPC on
// agent.Agent), so there is nothing to exempt under that name.
// GetAgentHealth (see agent_health.go) is the third: a read-only lifecycle
// probe with zero side effects, no work lifetime, and no pause mutation
// (admissionPausedSnapshot is observational). Its whole purpose is to answer
// "is this agent usable?" from outside — including DURING the pause — so it
// must be servable while update preparation holds the gate.
func updateAdmissionExemptUnary(prefix string) map[string]bool {
	return map[string]bool{
		prefix + "ShutdownAgent":              true,
		prefix + "ShutdownAgentWhenIdle":      true,
		prefix + "GetAgentHealth":             true,
		prefix + "CancelRuntimeModelDownload": true,
	}
}

// updateAdmissionExemptStreams lists passive observers plus the drain owner.
// DrainForUpdate validates before sealing and must not count its own lifetime.
// Observers hold no update-relevant state and must never
// pin the work count — clients need to keep attaching and watching events
// (including the disconnect that tells them an update bounce is happening)
// while preparation is paused.
func updateAdmissionExemptStreams(prefix string) map[string]bool {
	return map[string]bool{
		prefix + "DrainForUpdate":     true, // owns the gate; counting it would deadlock idle
		prefix + "SubscribeEvents":    true,
		prefix + "AttachConversation": true,
		prefix + "StreamRuntimeLogs":  true,
	}
}

// UpdateAdmissionUnaryInterceptor returns a grpc.UnaryServerInterceptor that
// counts every Agent unary request lifetime against update admission by
// default. Unlisted (and future) Agent methods are admitted automatically —
// nothing silently escapes coverage when a new RPC ships. Non-Agent methods
// pass straight through to their handler.
//
// The count is held for the WHOLE handler lifetime and released when the
// handler returns (success or error); panics unwind through the defer too, so
// the chain must keep RecoveryUnaryInterceptor OUTERMOST.
//
// Handlers that already call admitUpdateWork keep their per-method guards:
// the nested enter/release pairs simply hold two counts for the same call
// (each release is once-guarded). The interceptor propagates a gate-bound
// capability so handler guards can cross a reversible drain seal. The call
// is only idle again when all admitted lifetimes are retired.
func (s *Server) UpdateAdmissionUnaryInterceptor() grpc.UnaryServerInterceptor {
	prefix := agentFullMethodPrefix()
	exempt := updateAdmissionExemptUnary(prefix)
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !strings.HasPrefix(info.FullMethod, prefix) || exempt[info.FullMethod] {
			return handler(ctx, req)
		}
		ctx, release, err := s.updateWork.enterContext(ctx)
		if err != nil {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
		defer release()
		return handler(ctx, req)
	}
}

// UpdateAdmissionStreamInterceptor returns a grpc.StreamServerInterceptor that
// counts every non-observer Agent stream lifetime against update admission,
// with the same default-admit semantics as the unary interceptor: unlisted and
// future Agent streams count; only the documented passive observers and the
// drain owner pass through uncounted. Non-Agent services are untouched.
//
// The count is released only when the stream HANDLER returns — not when the
// stream's context is canceled. A canceled context still has to unwind
// through the handler (RecvMsg/SendMsg errors) before the lifetime retires,
// which is what pauseWhenIdle waits on. Existing per-stream guards double
// count safely, exactly as in the unary case.
func (s *Server) UpdateAdmissionStreamInterceptor() grpc.StreamServerInterceptor {
	prefix := agentFullMethodPrefix()
	exempt := updateAdmissionExemptStreams(prefix)
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !strings.HasPrefix(info.FullMethod, prefix) || exempt[info.FullMethod] {
			return handler(srv, ss)
		}
		ctx, release, err := s.updateWork.enterContext(ss.Context())
		if err != nil {
			return status.Error(codes.Unavailable, err.Error())
		}
		defer release()
		return handler(srv, &updateOwnedStream{ServerStream: ss, ctx: ctx})
	}
}

type updateOwnedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *updateOwnedStream) Context() context.Context { return s.ctx }
