package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// developmentConfigAuthority never discovers ambient configuration. Explicit
// authorities stay inside the prepared state, shared by its A/B generations.
// config.Load and adjacent MCP/permissions retain ordinary production semantics.
func developmentConfigAuthority(state, selected string) (string, error) {
	if selected == "" {
		selected = filepath.Join(state, "config.yaml")
	}
	rel, err := filepath.Rel(state, selected)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("config authority must remain inside isolated prepared state")
	}
	if err := launchPrivateDir(filepath.Dir(selected)); err != nil {
		return "", err
	}
	if err := developmentPrivatePath(selected, false); err != nil {
		return "", err
	}
	if err := launchPrivateFile(selected); err != nil {
		return "", err
	}
	return selected, nil
}
