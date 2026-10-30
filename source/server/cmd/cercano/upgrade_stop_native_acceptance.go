//go:build ordinary_upgrade_acceptance && upgradefixture && darwin && cgo

package main

import (
	"cercano/source/server/internal/brewrestart"
	"context"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func acceptanceHostStop(ctx context.Context, endpoint netip.AddrPort) error {
	pid, err := strconv.Atoi(os.Getenv("CERCANO_FIXTURE_OWNED_HOST"))
	if err != nil || pid <= 0 || os.Getenv("CERCANO_DISABLE_CREDENTIALS") != "1" {
		return fmt.Errorf("explicit owned fixture host required")
	}
	state := os.Getenv("CERCANO_DEVELOPMENT_STATE")
	backups, err := filepath.Glob(filepath.Join(state, "conversations.before-upgrade-*.db"))
	if err != nil || len(backups) != 0 {
		return fmt.Errorf("backup preceded shutdown")
	}
	f, err := os.OpenFile(os.Getenv("CERCANO_FIXTURE_STOP_RECORD"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, "stop-start")
	if err := brewrestart.StopFixtureUpgradeHost(ctx, endpoint, pid); err != nil {
		return err
	}
	for _, raw := range strings.Split(os.Getenv("CERCANO_FIXTURE_OWNED_DESCENDANTS"), ",") {
		child, err := strconv.Atoi(raw)
		if err != nil || child <= 0 {
			return fmt.Errorf("explicit owned descendant required")
		}
		if output, _ := exec.Command("/bin/ps", "-p", raw, "-o", "stat=").Output(); len(strings.TrimSpace(string(output))) != 0 && !strings.HasPrefix(strings.TrimSpace(string(output)), "Z") {
			return fmt.Errorf("owned descendant %d survives before backup", child)
		}
	}
	fmt.Fprintln(f, "verified-stop-before-backup")
	return nil
}
