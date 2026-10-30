//go:build !developmentfixture || !cercano_development_acceptance

package main

import "cercano/source/server/pkg/config"

func startOrdinaryGenerationServer(cfg config.Config, o developmentHostOptions, a productionStartupAdmission) (string, *productionShutdown, error) {
	return startGRPCServerWithStartup(cfg, o.Listen, nil, productionCredentialDefaults(), a)
}
