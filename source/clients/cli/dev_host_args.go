package main

import (
	cliui "cercano/source/clients/cli/internal/ui"
	"fmt"
)

// Validate presence as well as values, before config, credentials or dialing.
func validateDevHostArgs(present map[string]bool, host, action, destination, handover, receipt string) error {
	values := map[string]string{"dev-host": host, "dev-host-action": action, "dev-host-destination": destination, "dev-host-handover": handover, "dev-host-receipt": receipt}
	for name, value := range values {
		if present[name] && value == "" {
			return fmt.Errorf("--%s requires a nonempty value", name)
		}
	}
	management := present["dev-host-action"] || present["dev-host-destination"] || present["dev-host-handover"] || present["dev-host-receipt"]
	if management && host == "" {
		return fmt.Errorf("dev-host management flags require --dev-host")
	}
	if action == "" {
		if management {
			return fmt.Errorf("dev-host management arguments require --dev-host-action")
		}
		return nil
	}
	switch action {
	case "list", "acquire", "release", "retire":
		if destination != "" || handover != "" || receipt != "" {
			return fmt.Errorf("--dev-host-action=%s does not accept destination, handover or receipt", action)
		}
	case "transfer", "claim":
		if destination == "" || handover == "" {
			return fmt.Errorf("--dev-host-action=%s requires destination and handover", action)
		}
		if action == "claim" && receipt == "" {
			return fmt.Errorf("claim requires --dev-host-receipt")
		}
		if action == "transfer" && receipt != "" {
			return fmt.Errorf("transfer does not accept --dev-host-receipt")
		}
		if _, err := cliui.ReadDevHostSelection(destination); err != nil {
			return fmt.Errorf("dev-host destination: %w", err)
		}
	default:
		return fmt.Errorf("unknown host action %q", action)
	}
	return nil
}
