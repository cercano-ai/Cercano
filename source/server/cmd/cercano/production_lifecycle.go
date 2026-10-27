package main

import (
	"cercano/source/server/internal/inference"
	"cercano/source/server/pkg/config"
	"net"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/worker"
)

type productionLifecycleDependencies struct {
	// Optional invocation-local external dependencies; nil retains real defaults.
	openProvider func(config.Config) inference.Provider
	mcp          productionMCPDependencies
	watchdog     server.WatchdogDependencies
	cloudBuilder providers.CloudBuilder
	enterprise   productionEnterpriseDependencies
	openStore    func(string) (conversation.Store, error)
	listen       func(string, string) (net.Listener, error)
	reap         func() int
}

func productionLifecycleDefaults() productionLifecycleDependencies {
	return productionLifecycleDependencies{openStore: openAgentConversationStore, listen: net.Listen, reap: worker.ReapOrphanWorkers}
}
