//go:build !ordinary_upgrade_acceptance

package main

import "cercano/source/server/internal/brewrestart"

var upgradeAgentCheck = knownUpgradeAgents

var upgradeHostStop = brewrestart.StopUpgradeHost
