package main

import (
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/worker"
	"net"
)

type productionLifecycleDependencies struct {
	enterprise productionEnterpriseDependencies
	openStore  func(string) (conversation.Store, error)
	listen     func(string, string) (net.Listener, error)
	reap       func() int
}

func productionLifecycleDefaults() productionLifecycleDependencies {
	return productionLifecycleDependencies{openStore: openAgentConversationStore, listen: net.Listen, reap: worker.ReapOrphanWorkers}
}
