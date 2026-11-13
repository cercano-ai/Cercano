package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"path/filepath"
	"time"

	"cercano/source/server/internal/brewrestart"
	"cercano/source/server/internal/server"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/protobuf/proto"
)

type developmentContainedSource struct {
	state        string
	destination  *pb.LifecycleIdentity
	inspect      func(context.Context, string, netip.AddrPort) (brewrestart.Identity, error)
	stop         func(context.Context, string, netip.AddrPort, brewrestart.Identity, func() error) error
	terminate    func(context.Context, string, netip.AddrPort, brewrestart.Identity, func() error) error
	absent       func(context.Context, string) error
	authenticate func(context.Context, string, *pb.LifecycleIdentity) error
}

var _ server.ContainedSourceStop = (*developmentContainedSource)(nil)

type preparedSourceSnapshot struct {
	Record  launchSelection
	Process brewrestart.Identity
	Absent  bool
}

func newDevelopmentContainedSource(state string, destination *pb.LifecycleIdentity) *developmentContainedSource {
	p := &developmentContainedSource{state: state, destination: destination, inspect: brewrestart.PrepareLegacyHost, terminate: brewrestart.StopLegacyHost}
	p.stop = p.shutdownAuthenticated
	p.authenticate = func(ctx context.Context, endpoint string, identity *pb.LifecycleIdentity) error {
		phase, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		conn, err := developmentPeerDial(state, destination)(phase, endpoint, identity)
		if err != nil {
			return err
		}
		return conn.Close()
	}
	return p
}

// Production absence is bounded to the canonical prepared generation, not all
// same-user argv/FDs. The existing absent seam is retained for adapter unit tests.
func (p *developmentContainedSource) preparedAbsent(ctx context.Context, host, generation, store string) error {
	if p.absent != nil {
		return p.absent(ctx, host)
	}
	return brewrestart.VerifyPreparedWritersAbsent(ctx, generation, store)
}

// withSource uses exactly the recovery path's existing per-generation launch
// exclusion. Verification runs inside that exclusion, never upgrade-wide stop.
func (p *developmentContainedSource) withSource(ctx context.Context, source *pb.LifecycleIdentity, fn func(launchSelection, netip.AddrPort) error) error {
	if source == nil || source.Host == "" || source.Host == p.destination.GetHost() {
		return fmt.Errorf("refusing destination source")
	}
	guard := developmentRecoveryGuardWithVerifier(p.state, p.destination, func(ctx context.Context, host string) error {
		files, err := filepath.Glob(filepath.Join(p.state, "hosts", "*.json"))
		if err != nil {
			return err
		}
		var record *launchSelection
		for _, path := range files {
			var r launchSelection
			if err := launchRead(path, &r); err != nil {
				return err
			}
			if r.Identity.GetHost() != host {
				continue
			}
			if record != nil {
				return fmt.Errorf("ambiguous source record")
			}
			var intent map[string]string
			if err := launchRead(path[:len(path)-len(".json")]+".starting", &intent); err != nil {
				return err
			}
			if intent["host"] != host || intent["generation"] == "" {
				return fmt.Errorf("source generation changed")
			}
			r.Generation = intent["generation"]
			record = &r
		}
		if record == nil || !proto.Equal(record.Identity, source) {
			return fmt.Errorf("source record identity changed")
		}
		endpoint, err := netip.ParseAddrPort(record.Endpoint)
		if err != nil {
			return err
		}
		return fn(*record, endpoint)
	})
	release, err := guard(ctx, source.Host)
	if err != nil {
		return err
	}
	release()
	return nil
}

func (p *developmentContainedSource) Prepare(ctx context.Context, source *pb.LifecycleIdentity) (string, error) {
	var snapshot string
	err := p.withSource(ctx, source, func(record launchSelection, endpoint netip.AddrPort) error {
		id, err := p.inspect(ctx, source.Host, endpoint)
		if err != nil {
			if absentErr := p.preparedAbsent(ctx, source.Host, record.Generation, source.Store); absentErr != nil {
				return err
			}
			b, marshalErr := json.Marshal(preparedSourceSnapshot{Record: record, Absent: true})
			snapshot = string(b)
			return marshalErr
		}
		if p.authenticate != nil {
			if err := p.authenticate(ctx, endpoint.String(), source); err != nil {
				return err
			}
		}
		if id.Executable != filepath.Join(record.Generation, "cercano") {
			return fmt.Errorf("source executable does not match recorded generation")
		}
		b, err := json.Marshal(preparedSourceSnapshot{Record: record, Process: id})
		snapshot = string(b)
		return err
	})
	return snapshot, err
}

// containSource retains launch exclusion across authenticated native termination,
// prepared-generation absence and use. Workers are never signalled. The supported
// host owns persistence and independent tasks; canonical workers use host callbacks.
// The coordinator still rechecks consent/epoch transactionally inside use. This is
// not a fence against arbitrary same-user tools; external effects are not replayed.
func (p *developmentContainedSource) containSource(ctx context.Context, source *pb.LifecycleIdentity, snapshot string, validate func() error, verifyAndUse func(context.Context, string, string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if verifyAndUse == nil {
		return fmt.Errorf("recovery callback required")
	}
	if validate == nil {
		return fmt.Errorf("source stop requires fresh approval validation")
	}
	var expected preparedSourceSnapshot
	if snapshot == "" || json.Unmarshal([]byte(snapshot), &expected) != nil {
		return fmt.Errorf("invalid approved source snapshot")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return p.withSource(ctx, source, func(record launchSelection, endpoint netip.AddrPort) error {
		a, _ := json.Marshal(record)
		b, _ := json.Marshal(expected.Record)
		if string(a) != string(b) {
			return fmt.Errorf("approved source record changed")
		}
		id, inspectErr := p.inspect(ctx, source.Host, endpoint)
		if inspectErr == nil {
			if expected.Absent || !id.SameProcess(expected.Process) {
				return fmt.Errorf("approved source process changed")
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := p.stop(ctx, source.Host, endpoint, expected.Process, validate); err != nil {
				return err
			}
		} else {
			if err := p.preparedAbsent(ctx, source.Host, record.Generation, source.Store); err != nil {
				return fmt.Errorf("source attribution uncertain: %v; absence: %w", inspectErr, err)
			}
			if err := validate(); err != nil {
				return err
			}
		}

		if err := ctx.Err(); err != nil {
			return err
		}
		return verifyAndUse(ctx, record.Generation, source.Store)
	})
}

// WithStoppedSource is the only production mutation entry point. The private
// containSource helper permits isolated orchestration tests, but its absence
// seam cannot bypass this final native check. All observations remain inside the same
// launch exclusion, including the ownership mutation and reload callback.
// WithStoppedSource holds the source launcher exclusion through native writer
// observation, durable ownership recovery and reload. No worker is signalled.
func (p *developmentContainedSource) WithStoppedSource(ctx context.Context, source *pb.LifecycleIdentity, snapshot string, validate func() error, use func(context.Context) error) error {
	if use == nil {
		return fmt.Errorf("recovery callback required")
	}
	var expected preparedSourceSnapshot
	if json.Unmarshal([]byte(snapshot), &expected) != nil || expected.Record.Generation == "" || !proto.Equal(expected.Record.Identity, source) {
		return fmt.Errorf("exact prepared source snapshot required")
	}
	return p.containSource(ctx, source, snapshot, validate, func(ctx context.Context, generation, store string) error {
		for {
			err := brewrestart.VerifyPreparedWritersAbsent(ctx, generation, store)
			if err == nil {
				return use(ctx)
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("source writer containment refused: %v: %w", err, ctx.Err())
			case <-time.After(25 * time.Millisecond):
			}
		}
	})
}
