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
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/proto"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

// The only executable launched by these managers is this bounded stdio helper.
func TestProductionMCPStdioHelper(t *testing.T) {
	if os.Getenv("CERCANO_MCP_STDIO_FIXTURE") != "1" {
		return
	}
	if os.Getenv("CERCANO_DISABLE_CREDENTIALS") != "1" {
		os.Exit(42)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fixture := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	mcp.AddTool(fixture, &mcp.Tool{Name: "echo", Description: "fixture echo"}, func(ctx context.Context, req *mcp.CallToolRequest, in struct {
		Text string `json:"text"`
	}) (*mcp.CallToolResult, any, error) { return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: os.Getenv("CERCANO_MCP_HOST") + ":" + in.Text}}}, nil, nil })
	if err := fixture.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		os.Exit(43)
	}
	os.Exit(0) // Never write the Go test runner's PASS line onto protocol stdout.
}

func TestProductionMCPPreparedStdioOwnership(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	t.Setenv("HOME", t.TempDir())
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Use the same validated prepared execution input as production composition.
	opts, _ := developmentProductionFixture(t)
	spawn, err := worker.NewSpawnPolicy(worker.SpawnOptions{GenerationDir: opts.Generation, ManifestPath: filepath.Join(opts.Generation, "manifest.json"), HostInstance: opts.Host, ResourceRoot: opts.RuntimeRoot})
	if err != nil {
		t.Fatal(err)
	}
	policy := productionExecutionPolicy{prepared: &spawn}
	dir := t.TempDir()
	writeConfig := func(command string, args []string) {
		t.Helper()
		data, err := yaml.Marshal(map[string]any{"mcpServers": map[string]any{"fixture": map[string]any{"command": command, "args": args, "env": map[string]string{"CERCANO_DISABLE_CREDENTIALS": "1", "CERCANO_MCP_STDIO_FIXTURE": "1", "HOME": t.TempDir()}}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "mcp.yaml"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig(exe, []string{"-test.run=^TestProductionMCPStdioHelper$"})
	makeHost := func() (*server.Server, *mcphost.Manager, func()) {
		t.Helper()
		s := mcpCompositionServer(t)
		var manager *mcphost.Manager
		closeHost, err := composeProductionMCP(s, dir, productionMCPDependencies{newManager: func(r *agenttools.Registry, d string, w time.Duration) *mcphost.Manager {
			manager = mcphost.New(r, d, w)
			return manager
		}}, policy)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(closeHost)
		if manager == nil {
			t.Fatal("prepared composition silently omitted its manager")
		}
		awaitMCP(t, manager, mcphost.StateReady)
		return s, manager, closeHost
	}
	a, ma, closeA := makeHost()
	b, mb, _ := makeHost()
	if ma == mb {
		t.Fatal("hosts share a manager")
	}
	call := func(s *server.Server) {
		t.Helper()
		tool, ok := s.ToolRegistry().Get("mcp__fixture__echo")
		if !ok {
			t.Fatal("tool not discovered")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		got, err := tool.Execute(ctx, json.RawMessage(`{"text":"alive"}`))
		if err != nil || got.Text != ":alive" {
			t.Fatalf("tool call: %+v %v", got, err)
		}
	}
	call(a)
	call(b)
	closeA()
	closeA()
	if len(ma.List()) != 0 {
		t.Fatal("A manager retained connections")
	}
	if _, ok := a.ToolRegistry().Get("mcp__fixture__echo"); ok {
		t.Fatal("A retained tool")
	}
	call(b)
	// A genuinely failed configured command remains registered and visibly failed.
	writeConfig(filepath.Join(t.TempDir(), "missing-fixture-command"), nil)
	failed := mcpCompositionServer(t)
	var mf *mcphost.Manager
	closeFailed, err := composeProductionMCP(failed, dir, productionMCPDependencies{newManager: func(r *agenttools.Registry, d string, w time.Duration) *mcphost.Manager {
		mf = mcphost.New(r, d, w)
		return mf
	}}, policy)
	if err != nil {
		t.Fatalf("ordinary command failure became fatal: %v", err)
	}
	t.Cleanup(closeFailed)
	awaitMCP(t, mf, mcphost.StateFailed)
	listed, err := failed.ListMcpServers(context.Background(), &proto.ListMcpServersRequest{})
	if err != nil || len(listed.Servers) != 1 || listed.Servers[0].State != "failed" || mf.List()[0].Err == "" {
		t.Fatalf("command failure omitted: %+v %v", listed, err)
	}
	call(b)
}
