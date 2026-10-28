// Package approvaldisclosure defines display-only permission disclosures shared by transports.
// It never grants execution authority or changes the exact-argument approval token.
package approvaldisclosure

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ForTool returns the canonical experiment warning, or empty for ordinary tools.
func ForTool(name string, args json.RawMessage, workDir string) string {
	if name != "plan_experiment" {
		return ""
	}
	var a struct {
		Package string `json:"package"`
		Test    string `json:"test"`
	}
	_ = json.Unmarshal(args, &a)
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(workDir); err == nil {
		workDir = abs
	}
	argv, _ := json.Marshal([]string{"go", "test", "-count=1", "-timeout=60s", "-run=^" + a.Test + "$", a.Package})
	return fmt.Sprintf("Warning: tests execute arbitrary repository code and are not sandboxed; they may modify files, access credentials or the network, and start processes.\nCommand argv: %s\nDirectory: %q\nGOFLAGS is cleared; GOENV=off; other environment variables are inherited. Process timeout: 90s. Approval is for this invocation only (exact arguments), single-use, not reusable. Planning stays active.", argv, workDir)
}
