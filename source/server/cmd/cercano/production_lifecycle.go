package main

import (
	"net"
	"net/http"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/engine"
	"cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/config"
)

type productionOllamaEngine interface {
	engine.InferenceEngine
	engine.EmbeddingService
}

type productionLifecycleDependencies struct {
	newOllama                      func(string) productionOllamaEngine
	routingLogPath, failureLogPath string
	// Optional invocation-local external dependencies; nil retains real defaults.
	// runtimeHTTPClient replaces the llama/mistral engine transports; nil retains defaults.
	runtimeHTTPClient *http.Client
	// afterWiring observes the fully installed server before serving; nil is a no-op.
	beforeInitialRebuild func() // invocation-local callback barrier for isolated lifecycle tests
	afterWiring          func(*server.Server)
	openProvider         func(config.Config) inference.Provider
	mcp                  productionMCPDependencies
	watchdog             server.WatchdogDependencies
	cloudBuilder         providers.CloudBuilder
	enterprise           productionEnterpriseDependencies
	openStore            func(string) (conversation.Store, error)
	listen               func(string, string) (net.Listener, error)
	reap                 func() int
}

func productionLifecycleDefaults() productionLifecycleDependencies {
	return productionLifecycleDependencies{openStore: openAgentConversationStore, listen: net.Listen, reap: worker.ReapOrphanWorkers}
}
