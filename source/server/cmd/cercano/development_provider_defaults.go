//go:build !cercano_development_acceptance

package main

import "cercano/source/server/internal/secrets"

// A tagged actual-entrypoint acceptance helper may supply this symbol instead.
// Production has no mutable hook or environment-selected dependency override.
func developmentProductionDefaults() developmentProductionDependencies {
	return developmentProductionDependencies{
		openSecrets:    func() (secrets.Store, error) { return secrets.OpenKeychainWithOptions(secrets.Options{ReadOnly: true}) },
		connectRuntime: connectDevelopmentRuntime,
	}
}
