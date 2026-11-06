package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cercano/source/server/internal/brewrestart"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/config"
)

// Detection is deliberately bounded: same-user processes with the known
// cercano/cercano-agent executable names. It is not a writer fence. Renamed,
// custom, other-user, or subsequently launched processes are not covered.
// Conservatively refuse known processes even when their compatibility is unknown.
func knownUpgradeAgents(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-U", strconv.Itoa(os.Getuid()), "-o", "pid=,comm=").Output()
	if err != nil {
		return fmt.Errorf("cannot check known Cercano agents: %w", err)
	}
	var filtered strings.Builder
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			pid, err := strconv.Atoi(fields[0])
			if err == nil && pid > 0 && brewrestart.IsUpgradeUnidentifiedOrNonWriter(pid) {
				continue
			}
		}
		filtered.WriteString(line + "\n")
	}
	return refuseKnownUpgradeAgents(filtered.String(), os.Getpid())
}

func refuseKnownUpgradeAgents(snapshot string, self int) error {
	for _, line := range strings.Split(snapshot, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			return fmt.Errorf("invalid process snapshot")
		}
		if pid == self {
			continue
		}
		name := filepath.Base(strings.Join(f[1:], " "))
		if name == "cercano" || name == "cercano-agent" {
			return fmt.Errorf("offline upgrade refused before opening database: known Cercano process pid=%d executable=%s is active (compatibility unknown); stop ALL legacy Cercano agents and retry", pid, name)
		}
	}
	return nil
}

func runUpgradeStore(ctx context.Context, args []string, out io.Writer) error {
	return runUpgradeStoreChecked(ctx, args, out, upgradeAgentCheck)
}
func runUpgradeStoreChecked(ctx context.Context, args []string, out io.Writer, check func(context.Context) error) error {
	fs := flag.NewFlagSet("upgrade-store", flag.ContinueOnError)
	fs.SetOutput(out)
	db := fs.String("database", "", "existing conversation database (absolute path)")
	checkOnly := fs.Bool("check-only", false, "check known processes without opening the store")
	stopConfig := fs.String("stop-hosts-config", "", "after consent, stop verified DB-writing hosts (not descendants)")
	interrupted := fs.Bool("interrupt-tasks", false, "recover interrupted tasks after verified writers exit")
	stopped := fs.Bool("all-legacy-agents-stopped", false, "confirm ALL legacy Cercano agents are stopped and will remain stopped")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || !*stopped || !filepath.IsAbs(*db) {
		return fmt.Errorf("usage: cercano upgrade-store --database /absolute/path/conversations.db --all-legacy-agents-stopped; prerequisite: stop ALL legacy Cercano agents")
	}
	// Include all generation instances and intent-only attempts before any stop,
	// backup authorization, or database open. Discovery alone is not inventory.
	release, err := developmentUpgradeGuard(ctx, *db)
	if err != nil {
		return err
	}
	defer release()
	if *stopConfig != "" {
		// Match ordinary configuration port selection, but never treat a missing
		// authority as permission to stop the default listener.
		info, err := os.Stat(*stopConfig)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("upgrade shutdown requires an existing regular config file")
		}
		cfg, err := config.Load(*stopConfig)
		if err != nil {
			return fmt.Errorf("load upgrade shutdown config: %w", err)
		}
		endpoint, err := netip.ParseAddrPort("127.0.0.1:" + cfg.Port)
		if err != nil || !endpoint.Addr().IsLoopback() || endpoint.Port() == 0 || endpoint.Addr().Zone() != "" {
			return fmt.Errorf("upgrade shutdown requires a numeric loopback endpoint with nonzero configured port")
		}
		stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = upgradeHostStop(stopCtx, endpoint)
		cancel()
		if err != nil {
			return fmt.Errorf("upgrade host shutdown failed before opening database: %w", err)
		}
	}
	// Do not open, initialize, or change the database before detection succeeds.
	if err := check(ctx); err != nil {
		return err
	}
	if *checkOnly {
		return nil
	}
	upgrade := conversation.UpgradeOffline
	if *interrupted {
		upgrade = conversation.UpgradeInterruptedOffline
	}
	if err := upgrade(ctx, *db); err != nil {
		return err
	}
	if *interrupted {
		fmt.Fprintln(out, "Store upgraded. History and task rows preserved; unfinished tasks interrupted without replay. Never restart legacy writers against this store.")
	} else {
		fmt.Fprintln(out, "Store upgraded (or already compatible). History and task statuses preserved; no reconciliation or replay. Compatible ordinary launches now need no repeated upgrade. Never restart legacy writers against this store.")
	}
	return nil
}
