package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"cercano/source/server/internal/crashlog"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/pkg/config"
)

// productionRuntimeDependencies is invocation-local ordinary startup policy.
// Defaults retain the complete existing manager (all providers, callbacks,
// orphan handling and download resumption), not a reduced test implementation.
// Prepared hosts deliberately do not use this legacy fallback policy: they keep
// their explicit authority connector and exact lifecycle-owned store.
type productionRuntimeDependencies struct {
	probe  func(string) error
	mode   func() string
	remote func(context.Context) (productionRuntimeRemote, error)
	local  func(config.Config, *crashlog.Writer) localruntime.Manager
	warm   func(localruntime.Manager, config.Config)
}

type productionRuntimeRemote interface {
	localruntime.Manager
	Close() error
}

// Creating defaults performs no I/O; activation is checked before using them.
func productionRuntimeDefaults() productionRuntimeDependencies {
	return productionRuntimeDependencies{
		probe:  checkOllama,
		mode:   func() string { return os.Getenv(modelHostEnv) },
		remote: func(ctx context.Context) (productionRuntimeRemote, error) { return newModelHostRemote(ctx) },
		local:  buildRuntimeManager,
		warm:   warmActiveRuntime,
	}
}

func (d productionRuntimeDependencies) validate() error {
	if d.probe == nil || d.mode == nil || d.remote == nil || d.local == nil || d.warm == nil {
		return fmt.Errorf("complete production runtime dependencies required")
	}
	return nil
}

// connect preserves ordinary fallback and warming order. Cleanup releases only
// this invocation's attachment, after Server.Shutdown; it never stops shared
// resources. Once guards duplicate cleanup within this invocation, not siblings.
func (d productionRuntimeDependencies) connect(ctx context.Context, cfg config.Config, events *crashlog.Writer) (localruntime.Manager, func(), error) {
	if err := d.validate(); err != nil {
		return nil, nil, err
	}
	if strings.EqualFold(strings.TrimSpace(d.mode()), "inprocess") {
		return d.local(cfg, events), func() {}, nil
	}
	remote, err := d.remote(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, fmt.Errorf("connecting to model host: %w", ctxErr)
		}
		log.Printf("[WARN] shared model host unavailable (%v); hosting local models in this process", err)
		events.LogRuntimeEvent(crashlog.EventModelHostUnavailable, err.Error(), crashlog.RuntimeInfo{}, map[string]any{"fallback": "inprocess"})
		return d.local(cfg, events), func() {}, nil
	}
	if remote == nil {
		return nil, nil, fmt.Errorf("runtime connector returned nil manager")
	}
	remote.SetEndpoints(localruntime.EndpointsFromConfig(cfg, openChatModel(cfg), openEmbeddingModel(cfg)))
	d.warm(remote, cfg)
	var once sync.Once
	return remote, func() {
		once.Do(func() {
			if err := remote.Close(); err != nil {
				log.Printf("[WARN] closing model host connection: %v", err)
			}
		})
	}, nil
}
