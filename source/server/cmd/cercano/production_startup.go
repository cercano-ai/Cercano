package main

import (
	"context"
	"fmt"

	"cercano/source/server/internal/conversation"
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
