package main

import (
	"context"
	"fmt"
	"io"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/paths"
)

// dialProfile connects to a profile's agent, launching it when none runs.
// Tests replace it.
var dialProfile = agentclient.DialProfile

// dialActiveProfile connects to the agent for the active profile
// (CERCANO_PROFILE, default "default"), launching it when none runs, and
// reports progress to status.
func dialActiveProfile(ctx context.Context, status io.Writer) (*agentclient.Client, error) {
	profile, err := paths.Profile()
	if err != nil {
		return nil, fmt.Errorf("CERCANO_PROFILE: %w", err)
	}
	fmt.Fprintf(status, "cercano: connecting to the agent for profile %q…\n", profile)
	client, err := dialProfile(ctx, profile)
	if err != nil {
		return nil, err
	}
	if client.AutoLaunched {
		fmt.Fprintln(status, "cercano: auto-launched agent server (log:", client.ServerLog+")")
	}
	return client, nil
}
