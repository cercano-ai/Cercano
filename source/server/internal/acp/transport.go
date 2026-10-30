package acp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"
)

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}
type frame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}
type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}
type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

// Serve owns a single ACP connection. Backend creation is lazy: initialization
// does not launch a daemon. Only the reader handles requests; prompts run in
// workers, leaving cancellation and permission responses live at all times.
func Serve(ctx context.Context, in io.Reader, out io.Writer, connect func(context.Context) (Backend, error)) error {
	return newServer(ctx, connect).serve(in, out)
}

func newServer(ctx context.Context, connect func(context.Context) (Backend, error)) *server {
	ctx, stop := context.WithCancelCause(ctx)
	return &server{ctx: ctx, stop: stop, cancel: func() { stop(context.Canceled) }, output: make(chan outputFrame, 1), replies: make(chan outputFrame, maxPendingReplies), completions: make(chan outputFrame, maxOutstandingPrompts), connect: connect, sessions: make(map[string]*session), waits: make(map[string]chan frame)}
}

func (s *server) serve(in io.Reader, out io.Writer) error {
	ctx, cancel := s.ctx, s.cancel
	// A single pump owns encoding. The bounded mailbox and per-frame ack
	// preserve frame order without a growing queue or a goroutine per write.
	go s.pump(out)
	defer func() {
		cancel()
		// Close interruptible outputs before waiting for prompt cleanup. Arbitrary
		// nonclosable writers cannot be interrupted: at most this one IO pump may
		// remain blocked, never a prompt worker or permission denial.
		if closer, ok := out.(io.Closer); ok {
			_ = closer.Close()
		}
		s.wg.Wait()
		if s.backend != nil {
			_ = s.backend.Close()
		}
	}()
	// Decode separately so a failed output or parent cancellation can shut down
	// prompt workers even when the peer leaves its input open. A closable input
	// is closed on shutdown to interrupt the underlying read as well.
	if closer, ok := in.(io.Closer); ok {
		defer closer.Close()
	}
	// A terminal decode result has its own signal: it must not wait for the
	// dispatcher, which may be waiting for an output acknowledgement. Accepted
	// frames are still handed off in order before this signal can close.
	incoming := make(chan frame)
	readDone := make(chan struct{})
	var readErr error
	go func() {
		dec := json.NewDecoder(in)
		for {
			var f frame
			if err := dec.Decode(&f); err != nil {
				readErr = err
				close(readDone)
				return
			}
			select {
			case incoming <- f:
			case <-ctx.Done():
				return
			}
		}
	}()
	// EOF is a half-close: allow accepted dispatcher replies to drain first.
	// Generic io.Writer has no readiness/interrupt API, so an undrainable final
	// reply has an explicit bounded drain deadline, and is an ERROR, never a
	// successful EOF. The deadline is not used to decide which frames to accept.
	go func() {
		select {
		case <-readDone:
		case <-ctx.Done():
			return
		}
		timer := time.NewTimer(inputDrainDeadline)
		defer timer.Stop()
		select {
		case <-timer.C:
			if errors.Is(readErr, io.EOF) {
				s.stop(errOutputDrain)
			} else {
				s.stop(errors.Join(readErr, errOutputDrain))
			}
		case <-ctx.Done():
		}
	}()
	for {
		var f frame
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-readDone:
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			if s.lastReply != nil {
				select {
				case <-s.lastReply:
				case <-ctx.Done():
					return context.Cause(ctx)
				}
			}
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		case f = <-incoming:
		}
		if f.JSONRPC != "2.0" {
			s.reply(f.ID, nil, &Error{-32600, "expected JSON-RPC 2.0"})
			continue
		}
		if f.Method == "" {
			s.resolve(f)
			continue
		}
		s.handle(f)
	}
}

// Dispatcher responses never wait for output. A peer that exhausts this
// bounded mailbox is disconnected, cancelling workers and denying permissions.
const maxPendingReplies = 64

var errReplyQueueFull = errors.New("ACP reply queue full")

const inputDrainDeadline = 250 * time.Millisecond

var errOutputDrain = errors.New("ACP connection did not drain after input closed")

type server struct {
	ctx         context.Context
	cancel      context.CancelFunc
	stop        context.CancelCauseFunc
	output      chan outputFrame
	replies     chan outputFrame
	lastReply   <-chan struct{} // dispatcher-owned EOF drain watermark
	completions chan outputFrame
	outstanding int
	mu          sync.Mutex
	wg          sync.WaitGroup
	connect     func(context.Context) (Backend, error)
	backend     Backend
	initialized bool
	sessions    map[string]*session
	waits       map[string]chan frame
	sequence    uint64
}

type outputFrame struct {
	ctx     context.Context
	value   any
	done    chan struct{}
	session *session
}

func (s *server) pump(out io.Writer) {
	encoder := json.NewEncoder(out)
	for {
		select {
		case <-s.ctx.Done():
			return
		case f := <-s.output:
			if !s.deliver(encoder, f) {
				return
			}
		case f := <-s.replies:
			if !s.deliver(encoder, f) {
				return
			}
		case f := <-s.completions:
			if !s.deliver(encoder, f) {
				return
			}
		}
	}
}

func (s *server) deliver(encoder *json.Encoder, f outputFrame) bool {
	if f.session != nil {
		s.mu.Lock()
		f.session.delivering = f.done
		s.mu.Unlock()
	}
	defer func() {
		if f.session != nil {
			s.mu.Lock()
			f.session.pending = false
			f.session.delivering = nil
			s.outstanding--
			s.mu.Unlock()
		}
		// Admission wakes only after Encode and reservation cleanup finish.
		close(f.done)
	}()
	if s.ctx.Err() != nil {
		return false
	}
	if f.ctx.Err() != nil {
		return true
	}
	err := encoder.Encode(f.value)
	if err != nil {
		s.cancel()
	}
	return err == nil
}

func (s *server) writeContext(ctx context.Context, v any) {
	if ctx.Err() != nil {
		return
	}
	f := outputFrame{ctx: ctx, value: v, done: make(chan struct{})}
	select {
	case <-ctx.Done():
		return
	case s.output <- f:
	}
	select {
	case <-ctx.Done():
	case <-f.done:
	}
}
func (s *server) reply(id json.RawMessage, result any, err *Error) {
	if len(id) == 0 || string(id) == "null" {
		return
	}
	f := outputFrame{ctx: s.ctx, value: Response{JSONRPC: "2.0", ID: id, Result: result, Error: err}, done: make(chan struct{})}
	select {
	case <-s.ctx.Done():
	case s.replies <- f:
		s.lastReply = f.done
	default:
		s.stop(errReplyQueueFull)
	}
}
func (s *server) resolve(f frame) {
	var id string
	if json.Unmarshal(f.ID, &id) != nil {
		return
	}
	s.mu.Lock()
	ch := s.waits[id]
	s.mu.Unlock()
	if ch != nil {
		select {
		case ch <- f:
		default:
		}
	}
}

// Every admitted prompt reserves a terminal mailbox slot until delivery. Thus
// workers can finish cleanup and enqueue without depending on pump scheduling,
// even when the peer blocks output. No next prompt may overtake that result.
const maxOutstandingPrompts = 64

func (s *server) finishPrompt(session *session, id json.RawMessage, result any, err *Error) {
	f := outputFrame{ctx: s.ctx, value: Response{JSONRPC: "2.0", ID: id, Result: result, Error: err}, done: make(chan struct{}), session: session}
	select {
	case <-s.ctx.Done():
	case s.completions <- f:
	}
}
