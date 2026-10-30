package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"

	"cercano/source/server/internal/acp"
	"cercano/source/server/internal/acp/v1"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/config"
)

const acpProtocolVersion = v1.ProtocolVersion

type acpResponse = acp.Response

// runACPMode connects to (or launches) the shared daemon, never an embedded server.
func runACPMode(cfg config.Config, in *bufio.Reader) {
	setupDispatchLogFile()
	fmt.Fprintf(os.Stderr, "Cercano ACP agent (v%s) on stdio\n", version)
	if in == nil {
		in = bufio.NewReader(os.Stdin)
	}
	input := struct {
		io.Reader
		io.Closer
	}{in, os.Stdin}
	output, err := acpOutput(os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ACP output: %v\n", err)
		return
	}
	defer output.Close()
	err = acp.Serve(context.Background(), input, output, func(ctx context.Context) (acp.Backend, error) {
		client, err := agentclient.Dial(ctx, net.JoinHostPort("localhost", cfg.Port))
		if err != nil {
			return nil, err
		}
		return acp.NewAgentBackend(ctx, client), nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ACP: %v\n", err)
	}
}
