package main

import (
	"context"
	"fmt"
	"os"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/config"
)

// productionStartupPolicy is invocation-local. Legacy preserves the ordinary
// opener; generation admission is read-only and never adopts a database. The
// prepared host supplies its explicit snapshot resolvers until service parity
// permits canonical generation activation.
type productionStartupPolicy struct {
	legacy      bool
	authorities productionAuthorityResolvers
	contract    conversation.CoexistenceDeclaration
}

func legacyProductionStartup() productionStartupPolicy {
	return productionStartupPolicy{legacy: true, authorities: productionAuthorityDefaults()}
}

func canonicalGenerationStartup() productionStartupPolicy {
	return productionStartupPolicy{authorities: productionAuthorityDefaults(), contract: conversation.DevelopmentCoexistenceDeclaration()}
}

type productionStartupAdmission struct {
	paths  productionAuthorityPaths
	legacy bool
}

// admit must precede enterprise, runtime, credentials, config loading/migration
// and store opening. A failed preflight has no fallback to copied configuration.
func (p productionStartupPolicy) admit(ctx context.Context) (productionStartupAdmission, error) {
	if p.legacy && p.contract != (conversation.CoexistenceDeclaration{}) {
		return productionStartupAdmission{}, fmt.Errorf("legacy startup cannot carry a generation declaration")
	}
	if !p.legacy && p.contract.Contract == "" {
		return productionStartupAdmission{}, fmt.Errorf("generation declaration required")
	}
	paths, err := p.authorities.resolve()
	if err != nil {
		return productionStartupAdmission{}, err
	}
	if !p.legacy {
		if err := conversation.PreflightCoexistence(ctx, paths.conversations, p.contract); err != nil {
			return productionStartupAdmission{}, fmt.Errorf("generation authority admission: %w", err)
		}
	}
	return productionStartupAdmission{paths: paths, legacy: p.legacy}, nil
}

// conversationStore is the shared store-selection stage. Prepared callers pass
// the exact lifecycle-owned store, after the lifecycle's own open-time check.
// It is borrowed: this stage never closes it or substitutes a legacy store.
func (a productionStartupAdmission) conversationStore(prepared conversation.Store, openLegacy func(string) (conversation.Store, error)) (conversation.Store, error) {
	if a.paths.conversations == "" {
		return nil, fmt.Errorf("startup admission required")
	}
	if !a.legacy {
		if prepared == nil {
			return nil, fmt.Errorf("exact prepared lifecycle store required")
		}
		return prepared, nil
	}
	if prepared != nil || openLegacy == nil {
		return nil, fmt.Errorf("legacy store opener required without prepared store")
	}
	return openLegacy(a.paths.conversations)
}

// runAgentWithStartup is the config-opening boundary used by both the agent
// subcommand and ordinary server/MCP dispatch. Keep legacy loading (including
// its warning/default fallback) unchanged; generation admission and the parity
// gate must run before even attempting config.Load, which may migrate and save.
// Dependencies are invocation-local so tests cannot change another invocation.
func runAgentWithStartup(ctx context.Context, policy productionStartupPolicy, load func(string) (config.Config, error), run func(config.Config)) error {
	// Do not move legacy authority resolution ahead of loading: it historically
	// happens during server startup, after config warning/default fallback.
	// A mixed legacy/generation declaration must still fail before loading.
	if !policy.legacy || policy.contract != (conversation.CoexistenceDeclaration{}) {
		if _, err := policy.admitActivation(ctx); err != nil {
			return err
		}
	}
	cfg, err := load(policy.authorities.configPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to load config: %v (using defaults)\n", err)
		cfg = config.Defaults()
	}
	run(cfg)
	return nil
}

func (p productionStartupPolicy) admitActivation(ctx context.Context) (productionStartupAdmission, error) {
	a, err := p.admit(ctx)
	if err != nil {
		return productionStartupAdmission{}, err
	}
	if !a.legacy {
		return productionStartupAdmission{}, fmt.Errorf("canonical generation startup requires service parity; activation disabled")
	}
	return a, nil
}
