package main

import (
	"context"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/contextmeter"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/engine"
	"cercano/source/server/internal/loopcompact"
	"cercano/source/server/internal/recap"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/usage"
)

// Invocation-local dependencies. No store, provider, or current-owner lookup is
// captured globally. The caller must supply authority-safe callbacks for prepared
// use; this constructor cannot confer authority on an arbitrary provider.
type productionSchedulingDependencies struct {
	loop loopcompact.WiringDeps
}

type productionSchedulingOptions struct {
	recapDebounce      time.Duration
	recapMaxTurns      int
	compactionDebounce time.Duration
	attempts           usage.AttemptSink
}

func ordinarySchedulingOptions() productionSchedulingOptions {
	return productionSchedulingOptions{recapDebounce: 8 * time.Second, recapMaxTurns: 12, compactionDebounce: 10 * time.Second}
}

type productionScheduling struct {
	recap            *recap.Generator
	compaction       *compactiongen.Generator
	newLoopCompactor func() agent.LoopCompactor
}

// composeProductionScheduling constructs only: it neither schedules by ID nor
// installs agent callbacks. Ordinary callers explicitly retain legacy policy;
// prepared callers install only through PreparedServerOptions.Scheduling.
func composeProductionScheduling(st conversation.Store, d productionSchedulingDependencies, o productionSchedulingOptions) productionScheduling {
	if st == nil {
		return productionScheduling{}
	}
	complete := func(ctx context.Context, prompt string) (string, error) {
		if d.loop.BeginWork != nil {
			scoped, finish, err := d.loop.BeginWork(ctx)
			if err != nil {
				return "", err
			}
			defer finish()
			ctx = scoped
		}
		ctx, runner, model, tier, err := resolveRecapRoute(ctx, d.loop)
		if err != nil {
			return "", err
		}
		resp, err := runner.Process(ctx, &agent.Request{Input: prompt, ModelOverride: model, Tier: tier})
		if err != nil {
			return "", err
		}
		return resp.Output, nil
	}
	r := recap.New(st, complete, o.recapDebounce, o.recapMaxTurns)
	c := compactiongen.New(st, loopcompact.BuildSummarizer(d.loop), loopcompact.BuildConfig(d.loop), contextmeter.Default(), o.compactionDebounce)
	c.SetEnabled(d.loop.Cfg.Compaction.Enabled)
	c.SetToolElisionOnly(d.loop.Cfg.Compaction.ToolElisionOnly)
	r.SetAttemptSink(o.attempts)
	c.SetAttemptSink(o.attempts)
	return productionScheduling{r, c, loopcompact.NewFactory(d.loop)}
}

// This adapter deliberately captures no store. The prepared constructor supplies
// its exact validated store and owns producer fence/join. It installs no legacy
// ID-only producers and must not be combined with ordinary scheduler options.
func preparedProductionScheduling(d productionSchedulingDependencies, o productionSchedulingOptions) server.PreparedSchedulingFactory {
	return func(st conversation.Store) (*recap.Generator, *compactiongen.Generator) {
		p := composeProductionScheduling(st, d, o)
		return p.recap, p.compaction
	}
}

// newProductionPreparedServer binds the surviving prepared composition to the
// exact-store factory. The server installs exact-owner callbacks and owns their
// fence/join, including constructor rollback; no legacy scheduler is installed.
func newProductionPreparedServer(ctx context.Context, options server.PreparedServerOptions, routing productionRouting, cloud agent.CloudFactory, registry *engine.EngineRegistry, d productionSchedulingDependencies, o productionSchedulingOptions) (*server.Server, error) {
	options.Scheduling = preparedProductionScheduling(d, o)
	s, err := server.NewPreparedDurableServer(ctx, options, routing.router, routing.coordinator, cloud, registry)
	if err != nil {
		return nil, err
	}
	s.SetLoopCompactorFactory(loopcompact.NewFactory(d.loop))
	return s, nil
}
