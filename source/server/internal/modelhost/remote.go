package modelhost

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"cercano/source/server/internal/localruntime"
	"cercano/source/server/pkg/proto"
)

const (
	// localCallTimeout bounds calls made by Manager methods that take no
	// context (Providers, WriteLog, EnrollDownload, RegisterProvider). It
	// covers a reconnect, which may launch a host.
	localCallTimeout = 30 * time.Second
	// attachTimeout bounds how long the host may take to confirm an Attach.
	attachTimeout = probeTimeout
	// eventQueue bounds decoded events waiting for the observer goroutine.
	eventQueue = 256
	// maxRecoverBackoff caps the delay between background reconnect attempts.
	maxRecoverBackoff = 5 * time.Second
)

var errRemoteClosed = errors.New("model host client is closed")

// RemoteOptions configures a Remote.
type RemoteOptions struct {
	// CoexistenceProtection prohibits replacement and shared runtime mutation.
	CoexistenceProtection bool
	// Ensure finds or launches the model host, on first connect and on every
	// reconnect.
	Ensure EnsureOptions
	// ClientID identifies this client's holds on the host. Empty generates a
	// random ID.
	ClientID string
	// Profile is reported to the host on Attach.
	Profile string
}

// Remote is a localruntime.Manager backed by the shared model host. Runtime
// state lives in the host; endpoints and observers stay local to this client.
//
// Remote never lets gRPC reconnect a verified connection on its own: when a
// call fails with Unavailable, the Attach stream breaks, or the connection
// leaves READY, it runs Ensure, verifies the host afresh, re-attaches, and
// (for a call) retries once. The old connection is closed first when Ensure
// names a different host, and only after the new stream is attached when it
// names the same one, so a live host keeps this client's holds. Concurrent
// callers share one reconnect.
type Remote struct {
	opts     RemoteOptions
	clientID string
	ensure   func(context.Context, EnsureOptions) (Record, error)

	// ctx is cancelled by Close; it bounds every background goroutine.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
	done   chan struct{}

	// reconnectMu serializes reconnects.
	reconnectMu sync.Mutex

	mu      sync.Mutex
	closed  bool
	conn    *grpc.ClientConn
	client  proto.ModelHostClient
	gen     uint64             // incremented on every successful connect
	endConn context.CancelFunc // ends the current Attach stream
	rec     Record             // the host the current connection is to

	epMu      sync.RWMutex
	endpoints []localruntime.EndpointRecord

	obsMu     sync.RWMutex
	observers []localruntime.Observer

	events chan remoteEvent
}

type remoteEvent struct {
	download *localruntime.DownloadEvent
	instance *localruntime.InstanceEvent
}

var (
	_ localruntime.Manager          = (*Remote)(nil)
	_ localruntime.SharedManager    = (*Remote)(nil)
	_ localruntime.DownloadEnroller = (*Remote)(nil)
)

// NewRemote connects to the model host (launching it if needed through
// opts.Ensure) and attaches. It returns once the host has registered the
// Attach stream, so no event after the first call is missed.
func NewRemote(ctx context.Context, opts RemoteOptions) (*Remote, error) {
	return newRemote(ctx, opts, Ensure)
}

func newRemote(ctx context.Context, opts RemoteOptions, ensure func(context.Context, EnsureOptions) (Record, error)) (*Remote, error) {
	opts.CoexistenceProtection = opts.CoexistenceProtection || opts.Ensure.CoexistenceProtection
	opts.Ensure.CoexistenceProtection = opts.CoexistenceProtection
	id := opts.ClientID
	if id == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		id = "client-" + hex.EncodeToString(b)
	}
	rctx, cancel := context.WithCancel(context.Background())
	r := &Remote{
		opts:     opts,
		clientID: id,
		ensure:   ensure,
		ctx:      rctx,
		cancel:   cancel,
		done:     make(chan struct{}),
		events:   make(chan remoteEvent, eventQueue),
	}
	r.wg.Add(1)
	go r.dispatch()
	if err := r.reconnect(ctx, 0); err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}

// ClientID is the ID this Remote attaches and calls with.
func (r *Remote) ClientID() string { return r.clientID }

// HoldEpoch implements localruntime.SharedManager. It changes on every
// reconnect: a host that saw this client's Attach stream end may have
// released its holds, and re-taking a hold that survived is harmless.
func (r *Remote) HoldEpoch() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gen
}

// Close ends the Attach stream, which releases this client's holds on the
// host, and stops all background work. Observers are not called after Close
// returns. Observers must not call Close.
func (r *Remote) Close() error {
	r.once.Do(func() {
		r.mu.Lock()
		r.closed = true
		conn, end := r.conn, r.endConn
		r.conn, r.client, r.endConn = nil, nil, nil
		r.mu.Unlock()
		r.cancel()
		if end != nil {
			end()
		}
		if conn != nil {
			conn.Close()
		}
		r.wg.Wait()
		close(r.done)
	})
	<-r.done
	return nil
}

// reconnect replaces the connection of generation failedGen with a freshly
// ensured and verified one. If another caller already replaced it, it returns
// nil at once.
func (r *Remote) reconnect(ctx context.Context, failedGen uint64) error {
	r.reconnectMu.Lock()
	defer r.reconnectMu.Unlock()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return errRemoteClosed
	}
	if r.gen != failedGen {
		r.mu.Unlock()
		return nil
	}
	prev := &prevConn{conn: r.conn, end: r.endConn, rec: r.rec}
	r.conn, r.client, r.endConn = nil, nil, nil
	r.mu.Unlock()
	// Whatever happens, the old connection is gone once reconnect returns.
	defer prev.close()
	return r.connect(ctx, prev)
}

// prevConn is the connection a reconnect replaces. It stays open while the
// replacement attaches to the same host, so the host never sees this client
// without a stream and keeps its holds.
type prevConn struct {
	conn *grpc.ClientConn
	end  context.CancelFunc
	rec  Record
	once sync.Once
}

// close ends the old Attach stream and connection; later calls do nothing.
func (p *prevConn) close() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		if p.end != nil {
			p.end()
		}
		if p.conn != nil {
			p.conn.Close()
		}
	})
}

// sameHost reports whether rec names the host the old connection is to.
func (p *prevConn) sameHost(rec Record) bool {
	return p != nil && p.conn != nil && rec.PID == p.rec.PID && rec.Addr == p.rec.Addr
}

// connect ensures a host, dials it with fresh verification and attaches. A
// host that is draining answers Unavailable; connect then waits and tries
// again (Ensure relaunches once it has gone) until the Ensure timeout. prev,
// when not nil, is the connection being replaced: it is closed before
// attaching to a different host, and only after attaching to the same one.
func (r *Remote) connect(ctx context.Context, prev *prevConn) error {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(r.ctx, cancel)
	defer stop()

	timeout := r.opts.Ensure.Timeout
	if timeout <= 0 {
		timeout = defaultEnsureTimeout
	}
	deadline := time.Now().Add(timeout)
	for {
		err := r.connectOnce(cctx, prev)
		if err == nil {
			return nil
		}
		if r.ctx.Err() != nil {
			return errRemoteClosed
		}
		if status.Code(err) != codes.Unavailable || cctx.Err() != nil || time.Now().After(deadline) {
			return err
		}
		sleep(cctx, pollInterval)
	}
}

func (r *Remote) connectOnce(ctx context.Context, prev *prevConn) error {
	rec, err := r.ensure(ctx, r.opts.Ensure)
	if err != nil {
		return err
	}
	if !prev.sameHost(rec) {
		prev.close()
	}
	// An empty token is no secret: anyone can compute its Hello proof.
	if rec.Token == "" {
		return errors.New("model host record has no token")
	}
	conn, _, err := dialVerified(ctx, rec, r.opts.Ensure.ClientVersion)
	if err != nil {
		return err
	}
	client := proto.NewModelHostClient(conn)
	sctx, end := context.WithCancel(context.Background())
	stream, err := client.Attach(sctx, &proto.AttachRequest{ClientId: r.clientID, Profile: r.opts.Profile, Pid: int32(os.Getpid())})
	if err == nil {
		err = awaitAttached(ctx, stream, end)
	}
	if err != nil {
		end()
		conn.Close()
		return err
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		end()
		conn.Close()
		return errRemoteClosed
	}
	r.gen++
	gen := r.gen
	r.conn, r.client, r.endConn, r.rec = conn, client, end, rec
	r.wg.Add(2)
	r.mu.Unlock()
	go r.readEvents(stream, sctx, gen)
	go r.watchConn(conn, sctx)
	// The new stream is registered: the old one can end without the host
	// releasing this client's holds.
	prev.close()
	return nil
}

// awaitAttached waits for the host's Attach header, which it sends once the
// stream is registered. A stream rejected before that (draining, bad token)
// ends with only a status, which is returned. end aborts the stream on
// timeout or when ctx is done.
func awaitAttached(ctx context.Context, stream grpc.ServerStreamingClient[proto.ManagerEvent], end context.CancelFunc) error {
	timer := time.AfterFunc(attachTimeout, end)
	stopCtx := context.AfterFunc(ctx, end)
	err := func() error {
		md, err := stream.Header()
		if err != nil {
			return err
		}
		if len(md.Get(attachedHeader)) > 0 {
			return nil
		}
		_, err = stream.Recv()
		if err == nil || errors.Is(err, io.EOF) {
			err = errors.New("model host did not confirm the attach")
		}
		return err
	}()
	timerOK, ctxOK := timer.Stop(), stopCtx()
	if !ctxOK {
		return ctx.Err()
	}
	if !timerOK {
		return status.Error(codes.Unavailable, "model host did not confirm the attach in time")
	}
	return err
}

// readEvents forwards the Attach stream to the observer goroutine. A stream
// that breaks while it is still wanted triggers a reconnect.
func (r *Remote) readEvents(stream grpc.ServerStreamingClient[proto.ManagerEvent], sctx context.Context, gen uint64) {
	defer r.wg.Done()
	for {
		msg, err := stream.Recv()
		if err != nil {
			if sctx.Err() != nil || r.ctx.Err() != nil {
				return // ended on purpose: replaced or closed
			}
			r.recoverConn(gen)
			return
		}
		ev, ok := decodeEvent(msg)
		if !ok || !r.isCurrent(gen) {
			// A replaced stream overlaps its replacement briefly during a
			// reconnect to the same host; only the current one delivers.
			continue
		}
		select {
		case r.events <- ev:
		case <-r.ctx.Done():
			return
		}
	}
}

// isCurrent reports whether gen is the current connection's generation.
func (r *Remote) isCurrent(gen uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gen == gen
}

// watchConn closes conn as soon as it leaves READY, so gRPC can never carry
// token-bearing calls over a new transport to whatever owns the address now.
// Closing it breaks the Attach stream, and readEvents reconnects.
func (r *Remote) watchConn(conn *grpc.ClientConn, sctx context.Context) {
	defer r.wg.Done()
	for {
		st := conn.GetState()
		if st != connectivity.Ready {
			conn.Close()
			return
		}
		if !conn.WaitForStateChange(sctx, st) {
			return
		}
	}
}

// recoverConn reconnects in the background until it succeeds, another caller
// replaces the connection, or the Remote is closed. Each distinct error is
// logged once.
func (r *Remote) recoverConn(gen uint64) {
	backoff := pollInterval
	// A persistent failure (a newer host this client cannot use, say) is
	// logged once rather than on every retry.
	var lastLogged string
	for {
		err := r.reconnect(r.ctx, gen)
		if err == nil || r.ctx.Err() != nil {
			return
		}
		if msg := err.Error(); msg != lastLogged {
			log.Printf("modelhost: reconnecting to model host: %v", err)
			lastLogged = msg
		}
		sleep(r.ctx, backoff)
		if backoff *= 2; backoff > maxRecoverBackoff {
			backoff = maxRecoverBackoff
		}
	}
}

func decodeEvent(msg *proto.ManagerEvent) (remoteEvent, bool) {
	switch {
	case msg.GetDownloadEventJson() != "":
		var ev localruntime.DownloadEvent
		if err := json.Unmarshal([]byte(msg.GetDownloadEventJson()), &ev); err != nil {
			log.Printf("modelhost: bad download event: %v", err)
			return remoteEvent{}, false
		}
		return remoteEvent{download: &ev}, true
	case msg.GetInstanceEventJson() != "":
		var ev localruntime.InstanceEvent
		if err := json.Unmarshal([]byte(msg.GetInstanceEventJson()), &ev); err != nil {
			log.Printf("modelhost: bad instance event: %v", err)
			return remoteEvent{}, false
		}
		return remoteEvent{instance: &ev}, true
	}
	return remoteEvent{}, false
}

// dispatch delivers events to observers, in order, on one goroutine.
func (r *Remote) dispatch() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ctx.Done():
			return
		case ev := <-r.events:
			r.obsMu.RLock()
			obs := append([]localruntime.Observer(nil), r.observers...)
			r.obsMu.RUnlock()
			for _, o := range obs {
				if r.ctx.Err() != nil {
					return
				}
				if ev.download != nil {
					o.OnDownloadStateChange(*ev.download)
				} else {
					o.OnInstanceStateChange(*ev.instance)
				}
			}
		}
	}
}

// current returns the live client, connecting first if there is none.
func (r *Remote) current(ctx context.Context) (proto.ModelHostClient, uint64, error) {
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return nil, 0, errRemoteClosed
		}
		if r.client != nil {
			c, g := r.client, r.gen
			r.mu.Unlock()
			return c, g, nil
		}
		g := r.gen
		r.mu.Unlock()
		if err := r.reconnect(ctx, g); err != nil {
			return nil, 0, err
		}
	}
}

// connLost reports whether a call error means the connection is gone (rather
// than the method failing): Unavailable, or Canceled from a closed connection
// while the caller's context is still live.
func (r *Remote) connLost(ctx context.Context, err error) bool {
	if ctx.Err() != nil || r.ctx.Err() != nil {
		return false
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.Canceled:
		return true
	}
	return false
}

// call invokes method on the host with req as its JSON payload and decodes
// the JSON response into resp (when non-nil). On a lost connection it
// reconnects and retries once.
func (r *Remote) call(ctx context.Context, method string, req, resp any) error {
	if r.opts.CoexistenceProtection {
		if disruptiveMethod(method) {
			return ErrCoexistenceProtected
		}
		ctx = metadata.AppendToOutgoingContext(ctx, protectionHeader, "true")
	}
	var payload string
	if req != nil {
		b, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("model host %s: encode request: %w", method, err)
		}
		payload = string(b)
	}
	for attempt := 0; ; attempt++ {
		client, gen, err := r.current(ctx)
		if err != nil {
			return err
		}
		out, err := client.Call(ctx, &proto.CallRequest{ClientId: r.clientID, Method: method, PayloadJson: payload})
		if err != nil {
			if attempt == 0 && r.connLost(ctx, err) {
				if rerr := r.reconnect(ctx, gen); rerr != nil {
					if errors.Is(rerr, errRemoteClosed) {
						return rerr
					}
					return status.Errorf(codes.Unavailable, "model host unavailable: %v (reconnect: %v)", err, rerr)
				}
				continue
			}
			return decodeError(err)
		}
		if resp != nil && out.GetPayloadJson() != "" {
			if err := json.Unmarshal([]byte(out.GetPayloadJson()), resp); err != nil {
				return fmt.Errorf("model host %s: decode response: %w", method, err)
			}
		}
		return nil
	}
}

// localCtx bounds a call made by a Manager method that takes no context.
func (r *Remote) localCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.ctx, localCallTimeout)
}

// RegisterProvider is a no-op: providers live in the model host. It logs a
// warning, locally and to the host's log.
func (r *Remote) RegisterProvider(p localruntime.Provider) {
	name := "<nil>"
	if p != nil {
		name = p.Name()
	}
	msg := fmt.Sprintf("RegisterProvider(%q) ignored: providers are registered in the model host", name)
	log.Printf("modelhost: %s", msg)
	r.WriteLog(localruntime.LogEntry{Source: "cercano.modelhost.remote", Level: "warn", RuntimeID: name, Message: msg})
}

// RegisterObserver adds an observer fed from the host's Attach stream.
func (r *Remote) RegisterObserver(o localruntime.Observer) {
	if o == nil {
		return
	}
	r.obsMu.Lock()
	defer r.obsMu.Unlock()
	r.observers = append(r.observers, o)
}

// Providers returns the host's providers, or nil if the host is unreachable.
func (r *Remote) Providers() []localruntime.ProviderInfo {
	ctx, cancel := r.localCtx()
	defer cancel()
	var out []localruntime.ProviderInfo
	if err := r.call(ctx, "Providers", nil, &out); err != nil {
		log.Printf("modelhost: Providers: %v", err)
		return nil
	}
	return out
}

func (r *Remote) Inventory(ctx context.Context) ([]localruntime.ModelRecord, error) {
	var out []localruntime.ModelRecord
	if err := r.call(ctx, "Inventory", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Remote) Instances(ctx context.Context) ([]localruntime.InstanceRecord, error) {
	var out []localruntime.InstanceRecord
	if err := r.call(ctx, "Instances", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Endpoints returns this client's endpoints; they are never sent to the host.
func (r *Remote) Endpoints(context.Context) ([]localruntime.EndpointRecord, error) {
	return r.localEndpoints(), nil
}

// SetEndpoints stores endpoints for this client only.
func (r *Remote) SetEndpoints(endpoints []localruntime.EndpointRecord) {
	cp := cloneEndpointRecords(endpoints)
	r.epMu.Lock()
	defer r.epMu.Unlock()
	r.endpoints = cp
}

func (r *Remote) localEndpoints() []localruntime.EndpointRecord {
	r.epMu.RLock()
	defer r.epMu.RUnlock()
	return cloneEndpointRecords(r.endpoints)
}

func cloneEndpointRecords(in []localruntime.EndpointRecord) []localruntime.EndpointRecord {
	if in == nil {
		return nil
	}
	out := make([]localruntime.EndpointRecord, len(in))
	for i, e := range in {
		e.ActiveRoles = append([]string(nil), e.ActiveRoles...)
		e.Models = append([]string(nil), e.Models...)
		out[i] = e
	}
	return out
}

// UpdateInstance is not remotely callable: instance records are written by
// providers inside the model host. It logs a warning and does nothing.
func (r *Remote) UpdateInstance(inst localruntime.InstanceRecord) {
	log.Printf("modelhost: UpdateInstance(%q) ignored: instance records are owned by the model host", inst.ID)
}

func (r *Remote) Start(ctx context.Context, req localruntime.StartRequest) (*localruntime.InstanceRecord, error) {
	var out *localruntime.InstanceRecord
	if err := r.call(ctx, "Start", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Stop releases this client's hold on the instance; the host stops it once
// no client holds it. Before returning, Stop tells every HoldReleaseObserver
// the hold is gone, even on error (the host may have released it before the
// call failed), so an engine reusing the instance takes a hold again.
func (r *Remote) Stop(ctx context.Context, req localruntime.StopRequest) error {
	err := r.call(ctx, "Stop", req, nil)
	r.holdReleased(req.InstanceID)
	return err
}

// holdReleased notifies the observers that track holds that this client no
// longer holds instance id. It runs on the caller's goroutine so the
// notification lands before Stop returns.
func (r *Remote) holdReleased(id string) {
	if id == "" {
		return
	}
	r.obsMu.RLock()
	obs := append([]localruntime.Observer(nil), r.observers...)
	r.obsMu.RUnlock()
	for _, o := range obs {
		if ho, ok := o.(localruntime.HoldReleaseObserver); ok {
			ho.OnHoldReleased(id)
		}
	}
}

func (r *Remote) Restart(ctx context.Context, req localruntime.RestartRequest) (*localruntime.InstanceRecord, error) {
	var out *localruntime.InstanceRecord
	if err := r.call(ctx, "Restart", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Remote) DownloadModel(ctx context.Context, req localruntime.DownloadRequest) (*localruntime.ModelRecord, error) {
	var out *localruntime.ModelRecord
	if err := r.call(ctx, "DownloadModel", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Remote) EnsureModelsPresent(ctx context.Context, runtime string, want []string) error {
	return r.call(ctx, "EnsureModelsPresent", EnsureModelsPresentRequest{Runtime: runtime, Want: want}, nil)
}

// ResolveOpenModel returns the record together with the restored sentinel, so
// ErrModelNotPresent arrives with its record exactly as from InMemoryManager.
// The host sends only the sentinel's name; the message is rebuilt in the
// in-process manager's format.
func (r *Remote) ResolveOpenModel(ctx context.Context, runtime, want string) (localruntime.ModelRecord, error) {
	var res ResolveOpenModelResult
	if err := r.call(ctx, "ResolveOpenModel", ResolveOpenModelRequest{Runtime: runtime, Want: want}, &res); err != nil {
		return localruntime.ModelRecord{}, err
	}
	if res.Error == "" {
		return res.Record, nil
	}
	sentinel, ok := sentinelByName(res.Error)
	if !ok {
		return res.Record, fmt.Errorf("model host: %s", res.Error)
	}
	runtime, want = strings.TrimSpace(runtime), strings.TrimSpace(want)
	switch {
	case errors.Is(sentinel, localruntime.ErrModelNotPresent):
		return res.Record, fmt.Errorf("%w: %q (%s)", sentinel, res.Record.ID, runtime)
	case want == "":
		return res.Record, fmt.Errorf("%w: empty ref", sentinel)
	default:
		return res.Record, fmt.Errorf("%w: %q (%s)", sentinel, want, runtime)
	}
}

// sentinelByName returns the localruntime sentinel with the given wire name.
func sentinelByName(name string) (error, bool) {
	for _, s := range sentinels {
		if s.name == name {
			return s.err, true
		}
	}
	return nil, false
}

func (r *Remote) CancelDownload(ctx context.Context, req localruntime.DownloadRequest) (*localruntime.ModelRecord, error) {
	var out *localruntime.ModelRecord
	if err := r.call(ctx, "CancelDownload", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Remote) DeleteModel(ctx context.Context, req localruntime.DeleteModelRequest) error {
	return r.call(ctx, "DeleteModel", req, nil)
}

// Status returns the host's snapshot with this client's own endpoints.
func (r *Remote) Status(ctx context.Context) (*localruntime.StatusSnapshot, error) {
	var out *localruntime.StatusSnapshot
	if err := r.call(ctx, "Status", nil, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = &localruntime.StatusSnapshot{}
	}
	out.Endpoints = r.localEndpoints()
	return out, nil
}

func (r *Remote) Logs(ctx context.Context, req localruntime.LogRequest) ([]localruntime.LogEntry, error) {
	var out []localruntime.LogEntry
	if err := r.call(ctx, "Logs", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// WriteLog forwards entry to the host's log. Failures are logged locally.
func (r *Remote) WriteLog(entry localruntime.LogEntry) {
	ctx, cancel := r.localCtx()
	defer cancel()
	if err := r.call(ctx, "WriteLog", entry, nil); err != nil {
		log.Printf("modelhost: WriteLog: %v", err)
	}
}

// EnrollDownload forwards an online-catalog download record to the host.
// Failures are logged; the following DownloadModel then reports not found.
func (r *Remote) EnrollDownload(rec localruntime.ModelRecord) {
	ctx, cancel := r.localCtx()
	defer cancel()
	if err := r.call(ctx, "EnrollDownload", rec, nil); err != nil {
		log.Printf("modelhost: EnrollDownload(%q): %v", rec.ID, err)
	}
}
