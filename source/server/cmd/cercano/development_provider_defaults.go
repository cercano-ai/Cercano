//go:build !cercano_development_acceptance

package main

// A tagged actual-entrypoint acceptance helper may supply this symbol instead.
// Production has no mutable hook or environment-selected dependency override.
func developmentProductionDefaults() developmentProductionDependencies {
	return developmentProductionDependencies{
		openSecrets:    productionCredentialDefaults().openSecrets,
		connectRuntime: connectDevelopmentRuntime,
	}
}
