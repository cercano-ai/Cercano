package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	cliui "cercano/source/clients/cli/internal/ui"
)

// The agent owns admission, acquisition and persistence. The independently
// installed CLI only selects an executable and consumes its existing binding.
func selectInstalledSession(ctx context.Context, state, runtime string) (string, error) {
	if !filepath.IsAbs(state) || !filepath.IsAbs(runtime) {
		return "", fmt.Errorf("absolute prepared --installed-state and --installed-runtime-root required")
	}
	binary, err := exec.LookPath("cercano")
	if err != nil {
		return "", err
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, binary, "development-launch", "--installed", "--state", state, "--runtime-root", runtime)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("selected agent did not admit a session (no singleton fallback): %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	path := strings.TrimSpace(string(output))
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("selected agent returned invalid session path")
	}
	selection, err := cliui.ReadDevHostSelection(path)
	if err != nil {
		return "", err
	}
	if selection.Fixture {
		return "", fmt.Errorf("installed agent returned a fixture selection")
	}
	return path, nil
}
