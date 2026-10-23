package profilemigrate

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"

	"gopkg.in/yaml.v3"
)

// DefaultOptions returns Options for the current user that find a
// pre-profile agent on 127.0.0.1 at its port and stop it with the existing
// ShutdownAgent RPC. The port is CERCANO_PORT when set, else the port in the
// legacy ~/.config/cercano/config.yaml, else the default. It is resolved on
// first use, which Run does before moving anything. Nothing is ever launched.
func DefaultOptions() Options {
	var once sync.Once
	var addr string
	resolve := func() string {
		once.Do(func() { addr = legacyAgentAddr() })
		return addr
	}
	return Options{
		LegacyAgentAlive: func(ctx context.Context) (bool, error) {
			return agentAnswering(ctx, resolve()), nil
		},
		StopLegacyAgent: func(ctx context.Context) error {
			return shutdownAgent(ctx, resolve())
		},
	}
}

// legacyAgentAddr is the loopback address a pre-profile agent listens on.
func legacyAgentAddr() string {
	port := os.Getenv("CERCANO_PORT")
	if port == "" {
		port = legacyConfigPort()
	}
	if port == "" {
		port = config.Defaults().Port
	}
	return net.JoinHostPort("127.0.0.1", port)
}

// legacyConfigPort reads the port key from the pre-profile config.yaml.
func legacyConfigPort() string {
	data, err := os.ReadFile(filepath.Join(paths.ConfigRoot(), "config.yaml"))
	if err != nil {
		return ""
	}
	var raw struct {
		Port any `yaml:"port"`
	}
	if yaml.Unmarshal(data, &raw) != nil || raw.Port == nil {
		return ""
	}
	return fmt.Sprint(raw.Port)
}

// agentAnswering reports whether a Cercano agent completes a gRPC connection
// at addr. A closed port, or a listener that does not speak gRPC, is not one.
func agentAnswering(ctx context.Context, addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	c, err := agentclient.DialExisting(ctx, addr)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func shutdownAgent(ctx context.Context, addr string) error {
	c, err := agentclient.DialExisting(ctx, addr)
	if err != nil {
		return err
	}
	defer c.Close()
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.ShutdownAgent(rctx, "Cercano is moving its data into the default profile")
}
