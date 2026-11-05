package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/devhostauth"
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

type ordinaryGenerationBinding struct {
	options developmentHostOptions
	auth    *devhostauth.Auth
	retire  func(context.Context) error
	retired <-chan struct{} // set by ordinary composition, observed by its transport owner
}

type productionStartupAdmission struct {
	generation *ordinaryGenerationBinding
	paths      productionAuthorityPaths
	legacy     bool
	// Set only after read-only generation compatibility preflight succeeds.
	generationPreflight bool
	bounded             *validatedDevelopmentBinding
}

// validatedDevelopmentBinding is an invocation-local snapshot, not an activation
// switch. Only the validated launcher mints it in production. Copy scalar identity
// fields so later mutation of the protobuf cannot change the admitted identity.
type validatedDevelopmentBinding struct {
	state, config, store, generation, host, build, contract string
	schema                                                  int32
	installed                                               bool
}

func developmentBinding(o developmentHostOptions) validatedDevelopmentBinding {
	b := validatedDevelopmentBinding{state: o.State, config: o.ConfigAuthority, generation: o.Generation, host: o.Host, installed: o.Installed}
	if o.Identity != nil {
		b.store, b.build, b.contract, b.schema = o.Identity.Store, o.Identity.Build, o.Identity.Contract, o.Identity.Schema
	}
	return b
}

func (a productionStartupAdmission) validateDevelopmentBinding(o developmentHostOptions) error {
	if a.legacy || !a.generationPreflight || a.bounded == nil || o.Identity == nil ||
		*a.bounded != developmentBinding(o) || o.Identity.Host != o.Host ||
		a.paths.config != o.ConfigAuthority || a.paths.conversations != o.Identity.Store {
		return fmt.Errorf("validated development startup binding required")
	}
	return a.validateActivation()
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
	return productionStartupAdmission{paths: paths, legacy: p.legacy, generationPreflight: !p.legacy}, nil
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
func runAgentWithStartup(ctx context.Context, policy productionStartupPolicy, load func(string) (config.Config, error), run func(config.Config, productionStartupAdmission)) error {
	var admission productionStartupAdmission
	if !policy.legacy || policy.contract != (conversation.CoexistenceDeclaration{}) {
		var err error
		admission, err = policy.admitActivation(ctx)
		if err != nil {
			return err
		}
	}
	configPath := admission.paths.config
	if policy.legacy {
		if policy.authorities.configPath == nil {
			return fmt.Errorf("authority resolvers required")
		}
		configPath = policy.authorities.configPath()
	}
	cfg, err := load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to load config: %v (using defaults)\n", err)
		cfg = config.Defaults()
	}
	if policy.legacy {
		// Preserve legacy DB resolution after config warning/default fallback,
		// but pin the config authority actually loaded, never resolve it again.
		policy.authorities.configPath = func() string { return configPath }
		admission, err = policy.admitActivation(ctx)
		if err != nil {
			return err
		}
	}
	run(cfg, admission)
	return nil
}

// validateActivation also guards downstream entries: an explicit generation
// admission must never be reinterpreted as legacy by a convenience wrapper.
func (a productionStartupAdmission) validateActivation() error {
	if a.paths.config == "" || a.paths.conversations == "" {
		return fmt.Errorf("startup admission required")
	}
	if a.generation != nil {
		if a.legacy || a.generation.auth == nil {
			return fmt.Errorf("validated generation authentication required")
		}
		if err := a.validateGenerationOptions(); err != nil {
			return err
		}
	}
	if !a.legacy {
		b := a.bounded
		contract := conversation.DevelopmentCoexistenceDeclaration()
		if !a.generationPreflight || b == nil || b.state == "" || b.generation == "" || b.host == "" || b.build == "" ||
			b.config != a.paths.config || b.store != a.paths.conversations ||
			b.store != filepath.Join(b.state, "conversations.db") || b.contract != contract.Contract || b.schema != int32(contract.SchemaVersion) {
			return fmt.Errorf("canonical generation startup requires service parity; activation disabled")
		}
	}
	return nil
}

func (p productionStartupPolicy) admitActivation(ctx context.Context) (productionStartupAdmission, error) {
	a, err := p.admit(ctx)
	if err != nil {
		return productionStartupAdmission{}, err
	}
	if err := a.validateActivation(); err != nil {
		return productionStartupAdmission{}, err
	}
	return a, nil
}

func (a productionStartupAdmission) validateGenerationOptions() error {
	if a.bounded == nil || *a.bounded != developmentBinding(a.generation.options) {
		return fmt.Errorf("generation binding mismatch")
	}
	return nil
}
