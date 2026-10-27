package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/engine"
	mcphost "cercano/source/server/internal/mcp_host"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/proto"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mcpCompositionServer(t *testing.T) *server.Server {
	t.Helper()
	r := composeProductionRouting(productionRoutingDependencies{open: &compositionRunner{name: "fixture"}, classificationDisabled: true})
	a := agent.NewAgent(r.router, r.coordinator)
	s := server.NewServer(a, r.router, r.coordinator, nil, engine.NewEngineRegistry())
	t.Cleanup(s.Shutdown)
	s.InstallCapabilities()
	return s
}

func awaitMCP(t *testing.T, m *mcphost.Manager, state mcphost.ServerState) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		list := m.List()
		if len(list) == 1 && list[0].State == state {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("MCP status: %+v", list)
		case <-time.After(time.Millisecond):
		}
	}
}

// Real manager, real initialize/list/call protocol; only transport is replaced.
// Each fixture owns its directory, session and bounded goroutine cleanup.
func TestProductionMCPInvocationOwnership(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	t.Setenv("HOME", t.TempDir())
	var opens, transports atomic.Int32
	makeHost := func() (*server.Server, func()) {
		s := mcpCompositionServer(t)
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "mcp.yaml"), []byte("mcpServers:\n  fixture:\n    command: must-not-execute\n"), 0600); err != nil {
			t.Fatal(err)
		}
		var manager *mcphost.Manager
		done := make(chan struct{})
		deps := productionMCPDependencies{newManager: func(reg *agenttools.Registry, got string, wait time.Duration) *mcphost.Manager {
			opens.Add(1)
			if got != dir || reg != s.ToolRegistry() || wait != 10*time.Second {
				t.Fatal("factory inputs changed")
			}
			manager = mcphost.NewWithTransportFactory(reg, got, wait, func(ctx context.Context, cfg mcphost.ServerConfig) (mcp.Transport, error) {
				transports.Add(1)
				fixture := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
				mcp.AddTool(fixture, &mcp.Tool{Name: "echo", Description: "echo"}, func(ctx context.Context, req *mcp.CallToolRequest, in struct {
					Text string `json:"text"`
				}) (*mcp.CallToolResult, any, error) { return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: in.Text}}}, nil, nil })
				st, ct := mcp.NewInMemoryTransports()
				go func() { defer close(done); _ = fixture.Run(ctx, st) }()
				return ct, nil
			})
			return manager
		}}
		closeHost, err := composeProductionMCP(s, dir, deps, productionExecutionPolicy{legacy: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(closeHost)
		t.Cleanup(func() {
			closeHost()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("fixture did not stop")
			}
		})
		awaitMCP(t, manager, mcphost.StateReady)
		listed, err := s.ListMcpServers(context.Background(), &proto.ListMcpServersRequest{})
		if err != nil || len(listed.Servers) != 1 || listed.Servers[0].State != "ready" {
			t.Fatalf("manager not registered: %v %v", listed, err)
		}
		return s, closeHost
	}
	first, closeFirst := makeHost()
	second, closeSecond := makeHost()
	call := func(s *server.Server) {
		tool, ok := s.ToolRegistry().Get("mcp__fixture__echo")
		if !ok {
			t.Fatal("missing protocol-discovered tool")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		result, err := tool.Execute(ctx, json.RawMessage(`{"text":"fixture reply"}`))
		if err != nil || result.Text != "fixture reply" {
			t.Fatalf("call: %+v %v", result, err)
		}
	}
	call(first)
	call(second)
	closeFirst()
	closeFirst()
	if _, ok := first.ToolRegistry().Get("mcp__fixture__echo"); ok {
		t.Fatal("closed host retained tool")
	}
	call(second)
	closeSecond()
	if opens.Load() != 2 || transports.Load() != 2 {
		t.Fatalf("openers: managers=%d transports=%d", opens.Load(), transports.Load())
	}
}

func TestProductionMCPFailures(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	t.Setenv("HOME", t.TempDir())
	for _, failure := range []string{"malformed", "connect"} {
		t.Run(failure, func(t *testing.T) {
			s := mcpCompositionServer(t)
			dir := t.TempDir()
			data := "mcpServers:\n  fixture:\n    command: must-not-execute\n"
			if failure == "malformed" {
				data = "mcpServers: ["
			}
			if err := os.WriteFile(filepath.Join(dir, "mcp.yaml"), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			var dials atomic.Int32
			var manager *mcphost.Manager
			closeHost, err := composeProductionMCP(s, dir, productionMCPDependencies{newManager: func(r *agenttools.Registry, d string, w time.Duration) *mcphost.Manager {
				manager = mcphost.NewWithTransportFactory(r, d, w, func(context.Context, mcphost.ServerConfig) (mcp.Transport, error) {
					dials.Add(1)
					return nil, context.DeadlineExceeded
				})
				return manager
			}}, productionExecutionPolicy{legacy: true})
			if err != nil {
				t.Fatalf("ordinary startup must remain nonfatal: %v", err)
			}
			defer closeHost()
			if failure == "connect" {
				awaitMCP(t, manager, mcphost.StateFailed)
				if dials.Load() != 1 {
					t.Fatal("missing positive opener")
				}
			} else if dials.Load() != 0 {
				t.Fatal("malformed config dialed")
			}
		})
	}
	var opens int
	_, err := composeProductionMCP(nil, "", productionMCPDependencies{newManager: func(*agenttools.Registry, string, time.Duration) *mcphost.Manager { opens++; return nil }}, productionExecutionPolicy{})
	if err == nil || opens != 0 {
		t.Fatal("invalid policy opened manager")
	}
}
