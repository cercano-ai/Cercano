package modelhost

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"cercano/source/server/internal/localruntime"
	"cercano/source/server/pkg/proto"
)

// tokenHeader is the metadata key carrying the shared secret.
const tokenHeader = "x-cercano-modelhost-token"

// attachedHeader is sent in Attach's response header once the stream is
// registered. A client that waits for it cannot miss an event broadcast after
// its next call.
const attachedHeader = "x-cercano-modelhost-attached"

// defaultIdleTimeout is how long the host stays up with no attached client.
const defaultIdleTimeout = 5 * time.Minute

// streamBuffer bounds the per-stream event queue. A stream that falls this far
// behind loses events instead of stalling the manager.
const streamBuffer = 128

// ServerOptions configures a Server.
type ServerOptions struct {
	// Token is the shared secret every RPC must present in metadata.
	Token string
	// IdleTimeout is how long the server tolerates having no attached stream
	// before stopping all instances and calling OnIdle. Default 5 minutes.
	IdleTimeout time.Duration
	// Now returns the current time. Default time.Now.
	Now func() time.Time
	// OnIdle is called once, after StopAll, when the idle timeout expires.
	// An idle stop is final: the server rejects Hello, Attach, Start and
	// Restart with Unavailable from then on, and the host process must exit.
	OnIdle func()
	// Version is reported by Hello.
	Version string
	// OnShutdown is called after StopAll when a client calls Shutdown. Like
	// OnIdle, it ends the server: the host process must exit.
	OnShutdown func()
}

// Server exposes a localruntime.Manager over gRPC and shares running
// instances between clients through holds.
type Server struct {
	proto.UnimplementedModelHostServer

	mgr   localruntime.Manager
	opts  ServerOptions
	holds *holdTable

	// keys serializes lifecycle operations per (runtime, model), so dedupe and
	// hold bookkeeping stay consistent for one model without making a slow
	// load of one model block operations on another.
	keys keyedMutex

	// draining is set once StopAll begins; lifecycle calls then fail.
	draining atomic.Bool

	mu             sync.Mutex
	streams        map[*stream]struct{}
	clients        map[string]int // attached stream count per client ID
	generation     map[string]uint64
	nextGeneration uint64
	idleSince      time.Time
	idleFired      bool
}

type stream struct {
	ch chan *proto.ManagerEvent
	// warned is set once a drop has been logged and cleared when the stream
	// drains again, so a stuck stream logs once rather than per event.
	warned atomic.Bool
}

// keyedMutex is a set of mutexes addressed by string key.
type keyedMutex struct {
	mu sync.Mutex
	m  map[string]*keyedEntry
}

type keyedEntry struct {
	mu   sync.Mutex
	refs int
}

// lock acquires the mutex for key and returns its unlock function.
func (k *keyedMutex) lock(key string) func() {
	k.mu.Lock()
	if k.m == nil {
		k.m = map[string]*keyedEntry{}
	}
	e := k.m[key]
	if e == nil {
		e = &keyedEntry{}
		k.m[key] = e
	}
	e.refs++
	k.mu.Unlock()
	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		k.mu.Lock()
		if e.refs--; e.refs == 0 {
			delete(k.m, key)
		}
		k.mu.Unlock()
	}
}

func modelKey(runtime, model string) string { return runtime + "\x00" + model }

// NewServer wraps m. It registers itself as an observer of m once.
func NewServer(m localruntime.Manager, opts ServerOptions) *Server {
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = defaultIdleTimeout
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	s := &Server{
		mgr:        m,
		opts:       opts,
		holds:      newHoldTable(),
		streams:    map[*stream]struct{}{},
		clients:    map[string]int{},
		generation: map[string]uint64{},
	}
	s.idleSince = opts.Now()
	m.RegisterObserver(s)
	return s
}

// Register adds the ModelHost service to g.
func (s *Server) Register(g *grpc.Server) {
	proto.RegisterModelHostServer(g, s)
}

// Run checks for idleness periodically until ctx is done.
func (s *Server) Run(ctx context.Context) {
	interval := s.opts.IdleTimeout / 4
	if interval > time.Second {
		interval = time.Second
	}
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.checkIdle(ctx)
		}
	}
}

// checkIdle stops everything and calls OnIdle once if no stream has been
// attached for IdleTimeout. It does nothing once the server is draining: a
// Shutdown already stopped everything and ended the server.
func (s *Server) checkIdle(ctx context.Context) {
	if s.draining.Load() {
		return
	}
	s.mu.Lock()
	expired := len(s.streams) == 0 && !s.idleFired &&
		s.opts.Now().Sub(s.idleSince) >= s.opts.IdleTimeout
	if expired {
		s.idleFired = true
	}
	s.mu.Unlock()
	// Claim the drain so a Shutdown that began meanwhile is not followed by a
	// second StopAll and an OnIdle.
	if !expired || !s.draining.CompareAndSwap(false, true) {
		return
	}
	if err := s.StopAll(ctx); err != nil {
		log.Printf("modelhost: idle stop: %v", err)
	}
	if s.opts.OnIdle != nil {
		s.opts.OnIdle()
	}
}

// StopAll stops every live instance and drops their holds. It also puts the
// server into a draining state for good: new Start and Restart calls fail with
// Unavailable, and a Start already in flight stops its instance when it
// completes. StopAll takes each instance's model lock while stopping it, but
// never waits behind another model's in-flight Start.
func (s *Server) StopAll(ctx context.Context) error {
	s.draining.Store(true)
	instances, err := s.mgr.Instances(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, inst := range instances {
		if !liveInstance(inst) {
			continue
		}
		if err := s.stopLive(ctx, inst); err != nil {
			errs = append(errs, fmt.Errorf("stop %s: %w", inst.ID, err))
		}
	}
	return errors.Join(errs...)
}

// stopLive stops inst under its model lock unless a concurrent Stop already did.
func (s *Server) stopLive(ctx context.Context, inst localruntime.InstanceRecord) error {
	unlock := s.keys.lock(modelKey(inst.Runtime, inst.ModelID))
	defer unlock()
	current, err := s.mgr.Instances(ctx)
	if err != nil {
		return err
	}
	for _, c := range current {
		if c.ID != inst.ID {
			continue
		}
		if liveInstance(c) {
			if err := s.mgr.Stop(ctx, localruntime.StopRequest{InstanceID: inst.ID}); err != nil {
				return err
			}
		}
		break
	}
	s.holds.drop(inst.ID)
	return nil
}

// sweepUnheld stops every live instance of runtime that no client holds, and
// drops their holds. The caller holds the model lock heldKey; an instance under
// another key is stopped under that key's lock, re-checked first, so it never
// races StopAll stopping the same instance.
func (s *Server) sweepUnheld(ctx context.Context, runtime, heldKey string) {
	instances, err := s.mgr.Instances(ctx)
	if err != nil {
		log.Printf("modelhost: sweep during shutdown: %v", err)
		return
	}
	for _, inst := range instances {
		if inst.Runtime != runtime || !liveInstance(inst) || s.holds.count(inst.ID) > 0 {
			continue
		}
		if modelKey(inst.Runtime, inst.ModelID) == heldKey {
			if err := s.mgr.Stop(ctx, localruntime.StopRequest{InstanceID: inst.ID}); err != nil {
				log.Printf("modelhost: stop %s during shutdown: %v", inst.ID, err)
			}
			s.holds.drop(inst.ID)
			continue
		}
		if err := s.stopLive(ctx, inst); err != nil {
			log.Printf("modelhost: stop %s during shutdown: %v", inst.ID, err)
		}
	}
}

// sweepModel stops every live instance of runtime whose model is one of models,
// and drops their holds. The caller holds the model lock.
func (s *Server) sweepModel(ctx context.Context, runtime string, models ...string) {
	instances, err := s.mgr.Instances(ctx)
	if err != nil {
		log.Printf("modelhost: sweep during shutdown: %v", err)
		return
	}
	for _, inst := range instances {
		if inst.Runtime != runtime || !liveInstance(inst) {
			continue
		}
		for _, m := range models {
			if m != "" && inst.ModelID == m {
				if err := s.mgr.Stop(ctx, localruntime.StopRequest{InstanceID: inst.ID}); err != nil {
					log.Printf("modelhost: stop %s during shutdown: %v", inst.ID, err)
				}
				s.holds.drop(inst.ID)
				break
			}
		}
	}
}

var errDraining = status.Error(codes.Unavailable, "model host shutting down")

// canonicalModel resolves a requested model reference to the manager's
// canonical record ID, so aliases and canonical IDs share one lock and dedupe
// key. It falls back to the raw reference when resolution fails.
func (s *Server) canonicalModel(ctx context.Context, runtime, model string) string {
	if runtime == "" || model == "" {
		return model
	}
	rec, _ := s.mgr.ResolveOpenModel(ctx, runtime, model)
	if rec.ID != "" {
		return rec.ID
	}
	return model
}

// liveInstance reports whether an instance may still hold a process. Only
// states that are definitely gone are excluded: an unreachable or unknown
// instance may be a hung process that must still be stopped and shared.
func liveInstance(i localruntime.InstanceRecord) bool {
	switch i.State {
	case localruntime.InstanceStopped, localruntime.InstanceCrashed, localruntime.InstanceFailed:
		return false
	}
	return true
}

// lockInstance takes the model lock for instance id, resolving its runtime and
// model from the manager. An unknown instance locks on its ID alone.
func (s *Server) lockInstance(ctx context.Context, id string) func() {
	if instances, err := s.mgr.Instances(ctx); err == nil {
		for _, inst := range instances {
			if inst.ID == id {
				return s.keys.lock(modelKey(inst.Runtime, inst.ModelID))
			}
		}
	}
	return s.keys.lock("instance\x00" + id)
}

// authorize checks the token in the incoming metadata.
func (s *Server) authorize(ctx context.Context) error {
	md, _ := metadata.FromIncomingContext(ctx)
	var got string
	if vals := md.Get(tokenHeader); len(vals) > 0 {
		got = vals[0]
	}
	if s.opts.Token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.opts.Token)) != 1 {
		return status.Error(codes.Unauthenticated, "invalid model host token")
	}
	return nil
}

// helloProof is HMAC-SHA256(key=token, msg=nonce): proof that the responder
// knows the host token without revealing it.
func helloProof(token string, nonce []byte) []byte {
	m := hmac.New(sha256.New, []byte(token))
	m.Write(nonce)
	return m.Sum(nil)
}

// Hello implements the version handshake. It needs no token, so a client can
// authenticate the host (via the proof) before ever sending it the token.
func (s *Server) Hello(_ context.Context, req *proto.HelloRequest) (*proto.HelloResponse, error) {
	if s.draining.Load() {
		return nil, errDraining
	}
	return &proto.HelloResponse{
		ApiVersion: APIVersion,
		Version:    s.opts.Version,
		Pid:        int32(os.Getpid()),
		Proof:      helloProof(s.opts.Token, req.GetNonce()),
	}, nil
}

// Shutdown stops all instances, then calls OnShutdown.
func (s *Server) Shutdown(ctx context.Context, _ *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	err := s.StopAll(ctx)
	if s.opts.OnShutdown != nil {
		s.opts.OnShutdown()
	}
	if err != nil {
		return nil, encodeError(err)
	}
	return &proto.ShutdownResponse{}, nil
}

// Attach streams manager events to the client until it disconnects, then
// releases the client's holds.
func (s *Server) Attach(req *proto.AttachRequest, srv grpc.ServerStreamingServer[proto.ManagerEvent]) error {
	ctx := srv.Context()
	if err := s.authorize(ctx); err != nil {
		return err
	}
	if s.draining.Load() {
		return errDraining
	}
	st := s.addStream(req.GetClientId())
	defer func() {
		if s.removeStream(req.GetClientId(), st) {
			s.releaseClient(req.GetClientId())
		}
	}()
	if err := srv.SendHeader(metadata.Pairs(attachedHeader, "1")); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-st.ch:
			st.warned.Store(false)
			if err := srv.Send(ev); err != nil {
				return err
			}
		}
	}
}

func (s *Server) addStream(clientID string) *stream {
	st := &stream{ch: make(chan *proto.ManagerEvent, streamBuffer)}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streams[st] = struct{}{}
	if s.clients[clientID] == 0 {
		s.nextGeneration++
		s.generation[clientID] = s.nextGeneration
	}
	s.clients[clientID]++
	return st
}

// removeStream detaches st and reports whether clientID has no streams left.
func (s *Server) removeStream(clientID string, st *stream) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.streams, st)
	s.clients[clientID]--
	gone := s.clients[clientID] <= 0
	if gone {
		delete(s.clients, clientID)
		delete(s.generation, clientID)
	}
	if len(s.streams) == 0 {
		s.idleSince = s.opts.Now()
	}
	return gone
}

func (s *Server) attachedClients(clientID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clients[clientID]
}

// releaseClient drops all of clientID's holds, stopping instances left unheld.
// Under each instance's model lock it first re-checks that the client has not
// re-attached: a reconnect that raced the old stream's teardown keeps its holds.
func (s *Server) releaseClient(clientID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, id := range s.holds.heldBy(clientID) {
		func() {
			unlock := s.lockInstance(ctx, id)
			defer unlock()
			s.mu.Lock()
			if s.clients[clientID] > 0 {
				s.mu.Unlock()
				return
			}
			remaining := s.holds.remove(clientID, id)
			s.mu.Unlock()
			if remaining > 0 {
				return
			}
			if err := s.mgr.Stop(ctx, localruntime.StopRequest{InstanceID: id}); err != nil {
				log.Printf("modelhost: release %s for client %s: %v", id, clientID, err)
			}
		}()
	}
}

// releaseLocked removes clientID's hold on id and stops the instance when no
// holds remain. The instance's model lock must be held.
func (s *Server) releaseLocked(ctx context.Context, clientID, id string) error {
	if s.holds.remove(clientID, id) > 0 {
		return nil
	}
	return s.mgr.Stop(ctx, localruntime.StopRequest{InstanceID: id})
}

// OnDownloadStateChange implements localruntime.Observer.
func (s *Server) OnDownloadStateChange(ev localruntime.DownloadEvent) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	s.broadcast(&proto.ManagerEvent{Event: &proto.ManagerEvent_DownloadEventJson{DownloadEventJson: string(b)}})
}

// OnInstanceStateChange implements localruntime.Observer.
func (s *Server) OnInstanceStateChange(ev localruntime.InstanceEvent) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	s.broadcast(&proto.ManagerEvent{Event: &proto.ManagerEvent_InstanceEventJson{InstanceEventJson: string(b)}})
}

// broadcast queues ev on every stream without blocking; a full stream drops it.
func (s *Server) broadcast(ev *proto.ManagerEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for st := range s.streams {
		select {
		case st.ch <- ev:
		default:
			if st.warned.CompareAndSwap(false, true) {
				log.Printf("modelhost: dropping events for slow stream")
			}
		}
	}
}

// EnsureModelsPresentRequest is the payload of the "EnsureModelsPresent" call.
type EnsureModelsPresentRequest struct {
	Runtime string
	Want    []string
}

// ResolveOpenModelRequest is the payload of the "ResolveOpenModel" call.
type ResolveOpenModelRequest struct {
	Runtime string
	Want    string
}

// ResolveOpenModelResult is the response of the "ResolveOpenModel" call. The
// manager returns a record alongside ErrModelNotPresent, so the sentinel
// travels in the body: Error is the sentinel's name ("ErrModelNotFound",
// "ErrModelNotPresent") or empty. Other errors are returned as statuses.
type ResolveOpenModelResult struct {
	Record localruntime.ModelRecord `json:"record"`
	Error  string                   `json:"error"`
}

// Call dispatches a localruntime.Manager method by name.
func (s *Server) Call(ctx context.Context, req *proto.CallRequest) (*proto.CallResponse, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	out, err := s.dispatch(ctx, req.GetClientId(), req.GetMethod(), []byte(req.GetPayloadJson()))
	if err != nil {
		return nil, encodeError(err)
	}
	return &proto.CallResponse{PayloadJson: out}, nil
}

// decode unmarshals payload into v; an empty payload leaves v zero.
func decode(payload []byte, v any) error {
	if len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, v); err != nil {
		return status.Errorf(codes.InvalidArgument, "bad payload: %v", err)
	}
	return nil
}

func encode(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", status.Errorf(codes.Internal, "encode response: %v", err)
	}
	return string(b), nil
}

func (s *Server) dispatch(ctx context.Context, clientID, method string, payload []byte) (string, error) {
	switch method {
	case "Providers":
		return encode(s.mgr.Providers())
	case "Inventory":
		v, err := s.mgr.Inventory(ctx)
		return reply(v, err)
	case "Instances":
		v, err := s.mgr.Instances(ctx)
		return reply(v, err)
	case "Endpoints":
		v, err := s.mgr.Endpoints(ctx)
		return reply(v, err)
	case "Start":
		var r localruntime.StartRequest
		if err := decode(payload, &r); err != nil {
			return "", err
		}
		v, err := s.start(ctx, clientID, r)
		return reply(v, err)
	case "Stop":
		var r localruntime.StopRequest
		if err := decode(payload, &r); err != nil {
			return "", err
		}
		return "", s.stop(ctx, clientID, r)
	case "Restart":
		var r localruntime.RestartRequest
		if err := decode(payload, &r); err != nil {
			return "", err
		}
		v, err := s.restart(ctx, clientID, r)
		return reply(v, err)
	case "DownloadModel":
		var r localruntime.DownloadRequest
		if err := decode(payload, &r); err != nil {
			return "", err
		}
		v, err := s.mgr.DownloadModel(ctx, r)
		return reply(v, err)
	case "EnsureModelsPresent":
		var r EnsureModelsPresentRequest
		if err := decode(payload, &r); err != nil {
			return "", err
		}
		return "", s.mgr.EnsureModelsPresent(ctx, r.Runtime, r.Want)
	case "ResolveOpenModel":
		var r ResolveOpenModelRequest
		if err := decode(payload, &r); err != nil {
			return "", err
		}
		rec, err := s.mgr.ResolveOpenModel(ctx, r.Runtime, r.Want)
		res := ResolveOpenModelResult{Record: rec}
		if err != nil {
			name, ok := sentinelName(err)
			if !ok {
				return "", err
			}
			res.Error = name
		}
		return encode(res)
	case "CancelDownload":
		var r localruntime.DownloadRequest
		if err := decode(payload, &r); err != nil {
			return "", err
		}
		v, err := s.mgr.CancelDownload(ctx, r)
		return reply(v, err)
	case "DeleteModel":
		var r localruntime.DeleteModelRequest
		if err := decode(payload, &r); err != nil {
			return "", err
		}
		return "", s.mgr.DeleteModel(ctx, r)
	case "Status":
		v, err := s.mgr.Status(ctx)
		return reply(v, err)
	case "Logs":
		var r localruntime.LogRequest
		if err := decode(payload, &r); err != nil {
			return "", err
		}
		v, err := s.mgr.Logs(ctx, r)
		return reply(v, err)
	case "WriteLog":
		var r localruntime.LogEntry
		if err := decode(payload, &r); err != nil {
			return "", err
		}
		s.mgr.WriteLog(r)
		return "", nil
	case "EnrollDownload":
		enroller, ok := s.mgr.(localruntime.DownloadEnroller)
		if !ok {
			return "", status.Error(codes.FailedPrecondition, "model host manager does not support EnrollDownload")
		}
		var r localruntime.ModelRecord
		if err := decode(payload, &r); err != nil {
			return "", err
		}
		enroller.EnrollDownload(r)
		return "", nil
	}
	return "", status.Errorf(codes.Unimplemented, "unknown model host method %q", method)
}

// reply encodes a successful result, or returns err untouched. A nil pointer
// result with a nil error encodes as JSON null.
func reply(v any, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return encode(v)
}

// start shares a running instance for the same runtime and model, otherwise
// starts one, and records the client's hold either way. Model-acquiring calls
// require an active Attach stream and cannot outlive its attachment generation.
func (s *Server) start(ctx context.Context, clientID string, req localruntime.StartRequest) (*localruntime.InstanceRecord, error) {
	if s.draining.Load() {
		return nil, errDraining
	}
	generation, err := s.attachment(clientID)
	if err != nil {
		return nil, err
	}
	canon := s.canonicalModel(ctx, req.Runtime, req.ModelID)
	unlock := s.keys.lock(modelKey(req.Runtime, canon))
	defer unlock()
	if s.draining.Load() {
		return nil, errDraining
	}
	instances, err := s.mgr.Instances(ctx)
	if err != nil {
		return nil, err
	}
	for _, inst := range instances {
		if liveInstance(inst) && inst.Runtime == req.Runtime &&
			(inst.ModelID == canon || inst.ModelID == req.ModelID) {
			inst := inst
			if err := s.acquire(clientID, generation, &inst, true); err != nil {
				return nil, err
			}
			return &inst, nil
		}
	}
	inst, err := s.mgr.Start(ctx, req)
	if s.draining.Load() {
		// StopAll ran while this launch was in flight and could not see it.
		// A failed Start may still leave a live instance (a slow load), so
		// sweep by model rather than trusting the result.
		s.sweepModel(ctx, req.Runtime, canon, req.ModelID)
		return nil, errDraining
	}
	if err != nil {
		return nil, err
	}
	if err := s.acquire(clientID, generation, inst, true); err != nil {
		return nil, err
	}
	return inst, nil
}

// attachment captures the current lifetime before any blocking model work.
func (s *Server) attachment(clientID string) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients[clientID] == 0 {
		return 0, errUnattached
	}
	return s.generation[clientID], nil
}

var errUnattached = status.Error(codes.FailedPrecondition, "an active Attach stream is required for model acquisition")

// acquire is called under the model lock. Validation and insertion are atomic
// with stream removal, so disconnect either sees the hold or invalidates it.
func (s *Server) acquire(clientID string, generation uint64, inst *localruntime.InstanceRecord, add bool) error {
	s.mu.Lock()
	valid := s.clients[clientID] > 0 && s.generation[clientID] == generation
	if valid && add {
		s.holds.add(clientID, inst.ID)
	}
	s.mu.Unlock()
	if valid {
		return nil
	}
	if s.holds.count(inst.ID) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.mgr.Stop(ctx, localruntime.StopRequest{InstanceID: inst.ID}); err != nil {
			return errors.Join(errUnattached, err)
		}
	}
	return errUnattached
}

// stop releases the client's hold and stops the instance once unheld.
func (s *Server) stop(ctx context.Context, clientID string, req localruntime.StopRequest) error {
	unlock := s.lockInstance(ctx, req.InstanceID)
	defer unlock()
	return s.releaseLocked(ctx, clientID, req.InstanceID)
}

// restartTarget resolves the instance a restart will act on, the way the
// manager does: by ID, or for an empty ID the newest non-stopped instance of
// the named runtime.
func (s *Server) restartTarget(ctx context.Context, req localruntime.RestartRequest) (localruntime.InstanceRecord, bool) {
	instances, err := s.mgr.Instances(ctx)
	if err != nil {
		return localruntime.InstanceRecord{}, false
	}
	if req.InstanceID != "" {
		for _, inst := range instances {
			if inst.ID == req.InstanceID {
				return inst, true
			}
		}
		return localruntime.InstanceRecord{}, false
	}
	if req.Runtime == "" {
		return localruntime.InstanceRecord{}, false
	}
	var running []localruntime.InstanceRecord
	for _, inst := range instances {
		if inst.Runtime == req.Runtime && inst.State != localruntime.InstanceStopped {
			running = append(running, inst)
		}
	}
	if len(running) == 0 {
		return localruntime.InstanceRecord{}, false
	}
	sort.Slice(running, func(i, j int) bool { return running[i].ID < running[j].ID })
	return running[len(running)-1], true
}

// restart restarts for every holder; holds follow the instance if its ID changes.
func (s *Server) restart(ctx context.Context, clientID string, req localruntime.RestartRequest) (*localruntime.InstanceRecord, error) {
	if s.draining.Load() {
		return nil, errDraining
	}
	generation, err := s.attachment(clientID)
	if err != nil {
		return nil, err
	}
	target, found := s.restartTarget(ctx, req)
	canon := s.canonicalModel(ctx, req.Runtime, req.ModelID)
	key := modelKey(req.Runtime, canon)
	if found {
		key = modelKey(target.Runtime, target.ModelID)
	}
	unlock := s.keys.lock(key)
	defer unlock()
	if s.draining.Load() {
		return nil, errDraining
	}
	inst, err := s.mgr.Restart(ctx, req)
	if s.draining.Load() {
		// As in start: the relaunch may have left a live instance even on
		// error. It carries the canonical model, not an alias the client sent.
		runtimeName, models := req.Runtime, []string{req.ModelID, canon}
		if found {
			runtimeName = target.Runtime
			models = append(models, target.ModelID)
		}
		s.sweepModel(ctx, runtimeName, models...)
		if !found && req.ModelID == "" && runtimeName != "" {
			// A fresh start with no model launched the provider's default,
			// whose name is unknown here. Draining blocks new starts, so an
			// unheld live instance of this runtime is that launch (or one an
			// in-flight start is about to sweep itself).
			s.sweepUnheld(ctx, runtimeName, key)
		}
		return nil, errDraining
	}
	if err != nil {
		return nil, err
	}
	if found {
		s.holds.move(target.ID, inst.ID)
	}
	if err := s.acquire(clientID, generation, inst, s.holds.count(inst.ID) == 0); err != nil {
		return nil, err
	}
	return inst, nil
}
