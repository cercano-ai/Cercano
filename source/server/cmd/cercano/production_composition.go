package main

import (
	"fmt"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/engine"
	"cercano/source/server/internal/loop"
	"cercano/source/server/internal/tools"
	"google.golang.org/adk/session"
)

// productionRoutingDependencies is invocation-local: callers supply resolved
// providers and embedding transport, never ambient credential/runtime openers.
// Store opening, compatibility preflight, worker and background-work policies
// deliberately remain with the host until their ownership contracts are ready.
type productionRoutingDependencies struct {
	open, cloud    agent.TurnRunner
	cloudFactory   agent.CloudFactory
	embedder       engine.EmbeddingService
	embeddingModel string
	// Prepared cloud-only hosts prohibit local classification. Ordinary startup
	// retains lazy embedding initialization even for MCP-only deployments.
	classificationDisabled bool
}

type productionRouting struct {
	sessions    session.Service
	coordinator *loop.ADKCoordinator
	router      *agent.LazyRouter
}

// composeProductionRouting is the shared routing/validation/session graph, not
// a new startup mode. It performs no I/O and does not enable generation startup.
func composeProductionRouting(d productionRoutingDependencies) productionRouting {
	sessions := session.InMemoryService()
	validator := tools.NewAutoValidator(tools.DefaultLoader(), tools.DefaultKindToValidator())
	coordinator := loop.NewADKCoordinator(d.open, d.cloud, validator, sessions)
	router := agent.NewLazyRouter(func() (*agent.SmartRouter, error) {
		if d.classificationDisabled {
			return nil, fmt.Errorf("cloud_only requires configured managed routing; local embedding classification is disabled")
		}
		return agent.NewSmartRouterFromBytes(d.open, d.cloud, d.embeddingModel, d.embedder, agent.DefaultPrototypes(), d.cloudFactory)
	}, d.open, d.cloud)
	return productionRouting{sessions: sessions, coordinator: coordinator, router: router}
}
