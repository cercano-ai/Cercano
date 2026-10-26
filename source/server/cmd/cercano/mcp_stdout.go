package main

import (
	"context"
	"fmt"
	"os"

	mcpserver "cercano/source/server/internal/mcp"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpStdio reserves standard output for MCP's JSON-RPC messages and returns
// the transport that carries them over stdin and the reserved output. Every
// other write to standard output — the embedded agent's diagnostics
// (internal/server prints with fmt.Printf), any other package's, and a child
// process's that inherits it — goes to standard error, which MCP hosts treat
// as a log, so it cannot corrupt the protocol stream. Call it once, before
// anything in MCP mode can print. It exits when standard output cannot be
// reserved: serving anyway would let diagnostics corrupt the stream.
func mcpStdio() gomcp.Transport {
	out, err := reserveStdoutForMCP()
	if err != nil {
		fmt.Fprintf(os.Stderr, "MCP: cannot reserve standard output for the protocol: %v\n", err)
		os.Exit(1)
	}
	return &gomcp.IOTransport{Reader: os.Stdin, Writer: out}
}

// runMCPServer serves s over transport until the client disconnects.
func runMCPServer(s *gomcp.Server, transport gomcp.Transport) {
	if err := s.Run(context.Background(), transport); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

// serveDegradedMCP serves MCP with every tool answering err, so the client
// sees a clear error instead of a dead pipe, until the client disconnects.
func serveDegradedMCP(err error) {
	transport := mcpStdio()
	runMCPServer(mcpserver.NewDegradedServer(err).MCPServer(), transport)
}
