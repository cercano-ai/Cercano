package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cliui "cercano/source/clients/cli/internal/ui"
	"cercano/source/server/pkg/config"
)

// The agent owns admission, acquisition and persistence. The independently
// installed CLI only selects an executable and consumes its existing binding.
func selectInstalledSession(ctx context.Context, state, runtime string) (string, error) {
	// Reuse source-launcher discovery, without provisioning or legacy adoption.
	// The agent remains the authority for private paths and store admission.
	if state == "" {
		state = os.Getenv("CERCANO_DEVELOPMENT_STATE")
		if state == "" {
			state = filepath.Dir(config.DefaultPath())
		}
	}
	if override := os.Getenv("CERCANO_CONVERSATIONS_DB"); override != "" && override != filepath.Join(state, "conversations.db") {
		return "", fmt.Errorf("CERCANO_CONVERSATIONS_DB conflicts with selected state; no migration or fallback")
	}
	if runtime == "" {
		runtime = os.Getenv("CERCANO_RUNTIME_ROOT")
		if runtime == "" {
			record := filepath.Join(state, "runtime-root.json")
			info, err := os.Lstat(record)
			if err != nil {
				return "", fmt.Errorf("existing prepared runtime record required: %w", err)
			}
			if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
				return "", fmt.Errorf("private regular runtime record required")
			}
			data, err := os.ReadFile(record)
			if err != nil {
				return "", err
			}
			if err := json.Unmarshal(data, &runtime); err != nil {
				return "", err
			}
		}
	}
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
	if authority := os.Getenv("CERCANO_DEVELOPMENT_CONFIG_AUTHORITY"); authority != "" {
		cmd.Args = append(cmd.Args, "--config-authority", authority)
	}
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
