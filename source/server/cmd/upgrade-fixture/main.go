//go:build darwin && cgo && upgradefixture

// This disposable driver deliberately cannot select global process discovery.
package main

import (
	"cercano/source/server/internal/brewrestart"
	"context"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		panic("usage: upgrade-fixture loopback:port owned-host-pid")
	}
	endpoint, err := netip.ParseAddrPort(os.Args[1])
	if err != nil {
		panic(err)
	}
	pid, err := strconv.Atoi(os.Args[2])
	if err != nil || pid <= 0 {
		panic("invalid pid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := brewrestart.StopFixtureUpgradeHost(ctx, endpoint, pid); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
