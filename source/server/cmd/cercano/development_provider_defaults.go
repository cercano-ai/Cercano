//go:build !cercano_development_acceptance

package main

import (
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/devhostauth"
	"context"
)

// A tagged actual-entrypoint acceptance helper may supply this symbol instead.
// Production has no mutable hook or environment-selected dependency override.
func developmentProductionDefaults() developmentProductionDependencies {
	return developmentProductionDependencies{
		openSecrets:    productionCredentialDefaults().openSecrets,
		connectRuntime: connectDevelopmentRuntime,
	}
}

func newProductionDevelopmentHost(ctx context.Context, o developmentHostOptions, auth *devhostauth.Auth) (*server.Server, *preparedShutdown, error) {
	return newProductionDevelopmentHostWithDependencies(ctx, o, auth, developmentProductionDefaults())
}
