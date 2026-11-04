package main

import (
	"cercano/source/server/pkg/proto"
	"context"
	"flag"
	"google.golang.org/grpc"
	"net"

	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRemoteStartupProcess(t *testing.T) {
	if os.Getenv("CERCANO_REMOTE_TEST_CHILD") == "1" {
		flag.CommandLine = flag.NewFlagSet("remote-probe", flag.ExitOnError)
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"remote-probe"}, os.Args[i+1:]...)
				break
			}
		}
		main()
		return
	}
	for _, tc := range []struct {
		name    string
		args    []string
		want    string
		success bool
	}{
		{"unavailable", []string{"--address", "127.0.0.1:1"}, "connect-only", false},
		{"help", []string{"--help"}, "-address", true},
		{"empty", []string{"--address="}, "--address requires", false},
		{"conflict", []string{"--address=127.0.0.1:1", "--dev-host=missing.json"}, "incompatible", false},
		{"management conflict", []string{"--address=127.0.0.1:1", "--dev-host-receipt=x"}, "incompatible", false},
		{"empty pinned conflict", []string{"--address=127.0.0.1:1", "--dev-host="}, "incompatible", false},
		{"action conflict", []string{"--address=127.0.0.1:1", "--dev-host-action=list"}, "incompatible", false},
		{"destination conflict", []string{"--address=127.0.0.1:1", "--dev-host-destination=x"}, "incompatible", false},
		{"handover conflict", []string{"--address=127.0.0.1:1", "--dev-host-handover=x"}, "incompatible", false},
		{"invalid port", []string{"--address=localhost:0"}, "--address requires", false},
		{"version", []string{"--version"}, "cercano-cli", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestRemoteStartupProcess$", "--"}, tc.args...)...)
			home := t.TempDir()
			cmd.Env = append(os.Environ(), "CERCANO_REMOTE_TEST_CHILD=1", "HOME="+home, "XDG_CONFIG_HOME="+home, "PATH="+t.TempDir(), "CERCANO_LAUNCHED_DEVELOPMENT=", "CERCANO_DISABLE_CREDENTIALS=0")
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.success || !strings.Contains(string(out), tc.want) {
				t.Fatalf("err=%v output=%s; want %q", err, out, tc.want)
			}
			if strings.Contains(string(out), "auto-launch failed") || strings.Contains(string(out), "Failed to load config") {
				t.Fatalf("local startup leaked: %s", out)
			}
		})
	}
}

func TestRemoteAddressValidation(t *testing.T) {
	for _, addr := range []string{"localhost:53780", "127.0.0.1:1234", "[::1]:1234", "trusted.internal:1234"} {
		if err := validateRemoteAddress(addr); err != nil {
			t.Errorf("%s: %v", addr, err)
		}
	}
	for _, addr := range []string{"", ":1234", "host", "host:0", "host:65536", "host:http", "http://host:1234", "::1:1234"} {
		if err := validateRemoteAddress(addr); err == nil {
			t.Errorf("accepted %q", addr)
		}
	}
}

func TestRemoteCLIDialWithoutLocalAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	proto.RegisterAgentServer(s, &remoteCLIProbe{})
	go s.Serve(l)
	defer s.Stop()
	c, err := dialCLI(t.Context(), l.Addr().String(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.AutoLaunched {
		t.Fatal("explicit loopback endpoint launched a local agent")
	}
	// A real fixture RPC verifies the selected endpoint rather than only TCP dial.
	if _, err := c.ListConversations(context.Background(), "", 10); err != nil {
		t.Fatal(err)
	}
}

type remoteCLIProbe struct{ proto.UnimplementedAgentServer }

func (*remoteCLIProbe) ListConversations(context.Context, *proto.ListConversationsRequest) (*proto.ListConversationsResponse, error) {
	return &proto.ListConversationsResponse{}, nil
}
