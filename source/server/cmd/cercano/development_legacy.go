package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"path/filepath"
	"time"

	"cercano/source/server/internal/brewrestart"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/protobuf/proto"
)

type developmentLegacyStop struct {
	state       string
	destination *pb.LifecycleIdentity
	inspect     func(context.Context, string, netip.AddrPort) (brewrestart.Identity, error)
	stop        func(context.Context, string, netip.AddrPort, brewrestart.Identity, func() error) error
}

type legacySourceSnapshot struct {
	Record  launchSelection
	Process brewrestart.Identity
}

func newDevelopmentLegacyStop(state string, destination *pb.LifecycleIdentity) *developmentLegacyStop {
	return &developmentLegacyStop{state, destination, brewrestart.PrepareLegacyHost, brewrestart.StopLegacyHost}
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
		return p.stop(ctx, source.Host, endpoint, expected.Process, validate)
	})
}
