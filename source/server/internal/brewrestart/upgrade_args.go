package brewrestart

import (
	"flag"
	"io"
	"strings"
)

// upgradeArgvRole classifies only; kernel image, start identity and listener
// ownership must still be verified before any host can be signalled.
func upgradeArgvRole(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	args := argv[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0]
	}
	fs := flag.NewFlagSet("upgrade-classification", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	mcp := fs.Bool("mcp", false, "")
	external := fs.String("grpc-addr", "", "")
	version := fs.Bool("version", false, "")
	stats := fs.Bool("stats", false, "")
	fs.Bool("server", false, "")
	if fs.Parse(args) != nil || *version || *stats {
		return "unrecognized"
	}
	if *mcp {
		if *external != "" {
			return "mcp-client"
		}
		return "embedded-mcp"
	}
	return "agent"
}
