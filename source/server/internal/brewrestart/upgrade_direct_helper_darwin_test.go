//go:build darwin && cgo

package brewrestart

import "testing"

// Compiled disposable image, not the installed .NET runtime or an MCP server.
// Reproduces the observed direct-child identity/role classification only.
func TestUpgradeDirectRuntimeHelperAllowed(t *testing.T) {
	testUpgradeStopWithWorker(t, false, "dotnet", "fixture-secret.dll")
}

func TestUpgradeExternalStdioChildAllowed(t *testing.T) {
	testUpgradeStopWithWorker(t, false, "external-stdio", "private-stdio")
}
