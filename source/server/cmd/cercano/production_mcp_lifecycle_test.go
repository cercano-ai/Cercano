package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/agenttools"
	mcphost "cercano/source/server/internal/mcp_host"
	"cercano/source/server/internal/server"
	"gopkg.in/yaml.v3"
)

// Exercise the actual prepared constructor's exact-owned resource cleanup too,
// not just the shared service factory. Runtime/provider/accounting are fakes;
// MCP uses ordinary command transport and real per-host managers.
func TestProductionMCPOwnedPreparedCleanup(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	t.Setenv("HOME", t.TempDir())
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	makeHost := func() (*server.Server, *mcphost.Manager, func()) {
		t.Helper()
		o, auth := developmentProductionFixture(t)
		data, err := yaml.Marshal(map[string]any{"mcpServers": map[string]any{"fixture": map[string]any{"command": exe, "args": []string{"-test.run=^TestProductionMCPStdioHelper$"}, "env": map[string]string{"CERCANO_DISABLE_CREDENTIALS": "1", "CERCANO_MCP_STDIO_FIXTURE": "1", "HOME": t.TempDir()}}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(o.State, "mcp.yaml"), data, 0600); err != nil {
			t.Fatal(err)
		}
		var manager *mcphost.Manager
		deps := enabledDevelopmentFakes(t, developmentProductionDependencies{mcp: productionMCPDependencies{newManager: func(reg *agenttools.Registry, dir string, wait time.Duration) *mcphost.Manager {
			if dir != o.State {
				t.Errorf("MCP authority: %s", dir)
			}
			manager = mcphost.New(reg, dir, wait)
			return manager
		}}})
		srv, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, deps)
		if err != nil {
			t.Fatal(err)
		}
		closeHost := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := cleanup.Wait(ctx); err != nil {
				t.Error(err)
			}
		}
		t.Cleanup(closeHost)
		if manager == nil {
			t.Fatal("missing prepared manager")
		}
		awaitMCP(t, manager, mcphost.StateReady)
		return srv, manager, closeHost
	}
	call := func(s *server.Server) {
		t.Helper()
		tool, ok := s.ToolRegistry().Get("mcp__fixture__echo")
		if !ok {
			t.Fatal("missing discovered tool")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		got, err := tool.Execute(ctx, json.RawMessage(`{"text":"survived"}`))
		if err != nil || got.Text != ":survived" {
			t.Fatalf("call: %+v %v", got, err)
		}
	}
	a, ma, closeA := makeHost()
	b, mb, _ := makeHost()
	if ma == mb {
		t.Fatal("shared manager")
	}
	call(a)
	call(b)
	closeA()
	if len(ma.List()) != 0 {
		t.Fatal("prepared cleanup retained A connections")
	}
	if _, ok := a.ToolRegistry().Get("mcp__fixture__echo"); ok {
		t.Fatal("prepared cleanup retained A tool")
	}
	call(b)
}
