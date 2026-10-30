//go:build ordinary_upgrade_acceptance && !upgradefixture

package main

import (
	"context"
	"net/netip"
)

func acceptanceHostStop(context.Context, netip.AddrPort) error { return nil }
