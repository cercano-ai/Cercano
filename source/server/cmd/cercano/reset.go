package main

import (
	"context"
	"fmt"
	"os"

	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/setupresetcmd"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/setupstate"
	"golang.org/x/term"
)

func runReset(args []string) int {
	return setupresetcmd.Run(context.Background(), args, os.Stdin, os.Stdout, os.Stderr, term.IsTerminal(int(os.Stdin.Fd())), func(ctx context.Context) (setupresetcmd.Outcome, error) {
		profile, err := paths.Profile()
		if err != nil {
			return setupresetcmd.Outcome{}, fmt.Errorf("CERCANO_PROFILE: %w", err)
		}
		// A running agent for the profile resets its own settings so they
		// apply live; without one the reset runs here. No agent is started.
		return setupresetcmd.Perform(ctx, setupresetcmd.Dependencies{
			ConfigPath: config.DefaultPath(), WizardPath: setupstate.StatePath(),
			OpenStore: func() (secrets.Store, error) {
				return secrets.OpenKeychain(profile)
			}, TryAgent: setupresetcmd.TryProfileAgent(profile),
		})
	})
}
