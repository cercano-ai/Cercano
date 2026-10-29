//go:build developmentfixture && cercano_development_acceptance

// Offline rehearsal client: ordinary authenticated settings APIs, no host construction.
package main

import (
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if os.Getenv("CERCANO_DISABLE_CREDENTIALS") != "1" || len(os.Args) != 3 {
		return fmt.Errorf("requires disabled credentials, selection and loop limit")
	}
	var s struct {
		Endpoint, TokenFile, Conversation, Epoch string
		Identity                                 *proto.LifecycleIdentity
	}
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s.Identity == nil {
		return fmt.Errorf("missing identity")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := agentclient.DialAuthenticatedPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: s.Endpoint, HostInstance: s.Identity.Host, Build: s.Identity.Build, Contract: s.Identity.Contract, SchemaVersion: s.Identity.Schema, ConversationID: s.Conversation, OwnershipEpoch: s.Epoch}, s.TokenFile)
	if err != nil {
		return err
	}
	defer c.Close()
	before, err := c.GetConfig(ctx)
	if err != nil {
		return err
	}
	message, err := c.UpdateConfig(ctx, agentclient.ConfigUpdate{ToolLoopMaxIterations: os.Args[2]})
	if err != nil {
		return err
	}
	after, err := c.GetConfig(ctx)
	if err != nil {
		return err
	}
	// Deliberately export only the changed non-secret field.
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"before_loop_limit": before.ToolLoopMaxIterations, "after_loop_limit": after.ToolLoopMaxIterations, "confirmation": message})
}
