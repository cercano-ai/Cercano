package main

import (
	"context"
	"fmt"
	"os"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/locus"
	"cercano/source/server/internal/loopcompact"
	"cercano/source/server/internal/managedrouting"
	"cercano/source/server/pkg/config"
)

// Recap shares the configured Compaction route, including managed policy.
// Resolve on each admission from the same immutable graph as task dispatch.
// Never reuse the startup open-tier override for a cloud or redirected route.
func resolveRecapRoute(ctx context.Context, d loopcompact.WiringDeps) (context.Context, agent.TurnRunner, string, string, error) {
	if d.Candidates == nil {
		return ctx, nil, "", "", fmt.Errorf("recap routing graph unavailable")
	}
	tiers := d.Candidates()
	ctx = managedrouting.PinCandidates(ctx, tiers)
	selected, assignment, model, managed, err := managedrouting.Select(ctx, config.TaskCompaction, tiers, managedrouting.Request{})
	source := "managed"
	if !managed {
		source = "task_default"
		assignment = d.Cfg.TaskAssignment(config.TaskCompaction)
		if _, ok := d.Cfg.TaskAssignments[config.TaskCompaction]; ok {
			source = "task_override"
		}
		if tiers.TaskFor != nil {
			assignment = tiers.TaskFor(config.TaskCompaction)
			source = "task_graph_default"
			if tiers.DeveloperConfig != nil {
				if _, ok := tiers.DeveloperConfig.TaskAssignments[config.TaskCompaction]; ok {
					source = "task_graph_override"
				}
			}
		}
		mode, modeErr := locus.ParseMode(d.Cfg.LocusMode)
		if tiers.Mode != "" {
			mode, modeErr = tiers.Mode, nil
		}
		err = modeErr
		if err == nil {
			selected, err = inference.SelectDestination(mode, assignment.Destination, tiers)
		}
		if err == nil && tiers.ModelFor != nil {
			model = tiers.ModelFor(selected, assignment.Quality.CapabilityTier())
		}
	}
	// Metadata only: no prompt, output, credentials, or token accounting. Quote
	// configurable labels so a model/profile cannot inject diagnostic lines.
	fmt.Fprintf(os.Stderr, "[recap] route source=%q task=%q configured_destination=%q destination=%q profile=%q provider=%q model=%q quality=%q cloud=%t selection_failed=%t\n", source, config.TaskCompaction, assignment.Destination, selected.PolicyDestination, selected.Profile, providerName(selected), model, assignment.Quality, selected.IsCloud, err != nil)
	if err != nil {
		return ctx, nil, "", "", err
	}
	provider := inference.WithTaskRoute(selected.Provider, config.TaskCompaction, assignment, selected.PolicyDestination, model)
	return ctx, agent.InferenceTurnRunner(provider, model), model, string(assignment.Quality.CapabilityTier()), nil
}

func providerName(s inference.Selection) string {
	if s.Provider == nil {
		return ""
	}
	return s.Provider.Name()
}
