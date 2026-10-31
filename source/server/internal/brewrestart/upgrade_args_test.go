package brewrestart

import "testing"

func TestUpgradeArgvRole(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "agent"}, {[]string{"agent"}, "agent"}, {[]string{"development-host"}, "development-host"},
		{[]string{"--server"}, "agent"}, {[]string{"--mcp"}, "embedded-mcp"},
		{[]string{"--mcp", "--grpc-addr", "localhost:1234"}, "mcp-client"},
		{[]string{"--grpc-addr=localhost:1234", "--mcp=true"}, "mcp-client"},
		{[]string{"--mcp", "--grpc-addr="}, "embedded-mcp"},
		{[]string{"--mcp=false", "--grpc-addr=localhost:1234"}, "agent"},
		{[]string{"--mcp", "--unknown"}, "unrecognized"},
		{[]string{"worker", "--socket", "/tmp/x"}, "worker"},
	} {
		if got := upgradeArgvRole(append([]string{"cercano"}, tc.args...)); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.args, got, tc.want)
		}
	}
}
