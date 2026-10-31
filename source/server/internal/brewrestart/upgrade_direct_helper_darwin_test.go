//go:build darwin && cgo

package brewrestart

import "testing"

// Compiled disposable image, not the installed .NET runtime or an MCP server.
// Reproduces the observed direct-child identity/role classification only.
func TestUpgradeDirectRuntimeHelperRemainsBlocked(t *testing.T) {
	testUpgradeStopWithWorker(t, false, "dotnet", "fixture-secret.dll")
}

func TestUpgradePrivateStdioExternalImage(t *testing.T) {
	testUpgradeStopWithWorker(t, false, "external-stdio", "private-stdio")
}
