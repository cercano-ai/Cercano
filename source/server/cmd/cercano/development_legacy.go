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

type developmentLegacyStop struct {
	state       string
	destination *pb.LifecycleIdentity
	inspect     func(context.Context, string, netip.AddrPort) (brewrestart.Identity, error)
	stop        func(context.Context, string, netip.AddrPort, brewrestart.Identity, func() error) error
	absent      func(context.Context, string) error
	// qualifyWriters is installed only by a bounded, qualified historical runtime.
	// Registry identity/build labels alone cannot certify a writer graph.
	qualifyWriters func(context.Context, *pb.LifecycleIdentity, string) error
}

var _ server.ContainedSourceStop = (*developmentLegacyStop)(nil)

type legacySourceSnapshot struct {
	Record  launchSelection
	Process brewrestart.Identity
}

func newDevelopmentLegacyStop(state string, destination *pb.LifecycleIdentity) *developmentLegacyStop {
	p := &developmentLegacyStop{state: state, destination: destination, inspect: brewrestart.PrepareLegacyHost, absent: brewrestart.VerifyHostAbsent}
	p.stop = p.shutdownAuthenticated
	return p
}

// withSource uses exactly the recovery path's existing per-generation launch
// exclusion. Verification runs inside that exclusion, never upgrade-wide stop.
func (p *developmentLegacyStop) withSource(ctx context.Context, source *pb.LifecycleIdentity, fn func(launchSelection, netip.AddrPort) error) error {
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

func (p *developmentLegacyStop) Prepare(ctx context.Context, source *pb.LifecycleIdentity) (string, error) {
	var snapshot string
	err := p.withSource(ctx, source, func(record launchSelection, endpoint netip.AddrPort) error {
		id, err := p.inspect(ctx, source.Host, endpoint)
		if err != nil {
			return err
		}
		if id.Executable != filepath.Join(record.Generation, "cercano") {
			return fmt.Errorf("source executable does not match recorded generation")
		}
		b, err := json.Marshal(legacySourceSnapshot{record, id})
		snapshot = string(b)
		return err
	})
	return snapshot, err
}

func (p *developmentLegacyStop) Stop(ctx context.Context, source *pb.LifecycleIdentity, snapshot string, validate func() error) error {
	return p.withStoppedSource(ctx, source, snapshot, validate, nil)
}

// withStoppedSource keeps the existing launch exclusion continuously across stop,
// source absence verification and use. It is a source-only building block, NOT
// writer containment. A recovery coordinator must establish the relevant writer
// contract before reassigning ownership, and recheck consent/epoch inside use.
// Stop uses authenticated historical ShutdownAgent, never a PID signal. Acceptance
// is not completion: kernel host absence is required under the held launch lock.
func (p *developmentLegacyStop) withStoppedSource(ctx context.Context, source *pb.LifecycleIdentity, snapshot string, validate func() error, use func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if validate == nil {
		return fmt.Errorf("source stop requires fresh approval validation")
	}
	var expected legacySourceSnapshot
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
		id, err := p.inspect(ctx, source.Host, endpoint)
		if err != nil {
			return err
		}
		if !id.SameProcess(expected.Process) {
			return fmt.Errorf("approved source process changed")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := p.stop(ctx, source.Host, endpoint, expected.Process, validate); err != nil {
			return err
		}
		if use == nil {
			return nil
		}
		if err := p.absent(ctx, source.Host); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return use(ctx)
	})
}

// WithStoppedSource is the coordinator boundary, distinct from source-only Stop.
// Unqualified production generations refuse before any shutdown request.
func (p *developmentLegacyStop) WithStoppedSource(ctx context.Context, source *pb.LifecycleIdentity, snapshot string, validate func() error, use func(context.Context) error) error {
	if p.qualifyWriters == nil {
		return fmt.Errorf("historical writer contract is not qualified for this selected generation; no host stopped")
	}
	if err := p.qualifyWriters(ctx, source, snapshot); err != nil {
		return err
	}
	return p.withStoppedSource(ctx, source, snapshot, func() error {
		if err := validate(); err != nil {
			return err
		}
		return p.qualifyWriters(ctx, source, snapshot)
	}, use)
}
