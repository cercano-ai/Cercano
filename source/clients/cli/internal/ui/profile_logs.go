package ui

import (
	"path/filepath"

	"cercano/source/server/pkg/paths"
)

// profileLogsDir is where the CLI's opt-in debug logs go: logs/ in the active
// profile's directory. main checks the home directory and profile name at
// startup, so this never resolves to a relative path.
func profileLogsDir() string {
	return filepath.Join(paths.ActiveProfileDir(), "logs")
}
