// upgrade-discovery-preflight is deliberately separate from agent startup.
// It has no database/config flags and no shutdown path.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"time"

	"cercano/source/server/internal/brewrestart"
)

func preflight(args []string, out io.Writer, discover func(netip.AddrPort) error) int {
	fs := flag.NewFlagSet("upgrade-discovery-preflight", flag.ContinueOnError)
	fs.SetOutput(out)
	address := fs.String("address", "", "required numeric loopback endpoint; read-only discovery, never shutdown")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	endpoint, err := netip.ParseAddrPort(*address)
	if err != nil || fs.NArg() != 0 || !endpoint.Addr().IsLoopback() || endpoint.Port() == 0 || endpoint.Addr().Zone() != "" {
		fmt.Fprintln(out, "explicit numeric loopback --address required")
		return 2
	}
	started := time.Now()
	err = discover(endpoint)
	fmt.Fprintf(out, "discovery elapsed=%s\n", time.Since(started))
	if err != nil {
		fmt.Fprintf(out, "discovery refused: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, "Read-only discovery passed at this instant; no shutdown or database access performed. This is not permission to migrate.")
	return 0
}
func main() { os.Exit(preflight(os.Args[1:], os.Stderr, brewrestart.PreflightUpgradeHost)) }
