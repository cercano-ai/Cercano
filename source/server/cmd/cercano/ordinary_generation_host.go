package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/devhostauth"
)

// The validated launcher supplies identity and authentication; the ordinary
// entrypoint owns the entire service graph, transport and disposer.
func runOrdinaryGenerationHost(ctx context.Context, o developmentHostOptions, auth *devhostauth.Auth, out io.Writer) (result error) {
	a := o.startupAdmission
	if err := a.validateDevelopmentBinding(o); err != nil {
		return err
	}
	a.generation = &ordinaryGenerationBinding{options: o, auth: auth}
	if err := a.validateActivation(); err != nil {
		return err
	}
	cfg, err := config.Load(a.paths.config)
	if err != nil {
		return err
	}
	address, shutdown, err := startOrdinaryGenerationServer(cfg, o, a)
	if shutdown != nil {
		defer func() { shutdown.Request(); <-shutdown.done; result = errors.Join(result, shutdown.Status().Err) }()
	}
	if err != nil {
		return err
	}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := developmentDialIdentity(probe, address, auth, o.Identity)
	if err != nil {
		return fmt.Errorf("authenticated ordinary readiness: %w", err)
	}
	conn.Close()
	stopWatch := watchCurrentTarget(ctx, filepath.Dir(o.Identity.Store), o.Identity, a.generation.retire)
	defer stopWatch()
	if err := publishDevelopmentRecord(o.Record, developmentHostSelection{OrdinaryGeneration: true, ConfigAuthority: o.ConfigAuthority, Endpoint: address, TokenFile: o.TokenFile, Identity: o.Identity}); err != nil {
		return err
	}
	fmt.Fprintf(out, "development-host ready %s build=%s host=%s\n", address, o.Identity.Build, o.Host)
	select {
	case <-ctx.Done():
	case <-shutdown.done:
	case <-a.generation.retired:
	}
	return nil
}
