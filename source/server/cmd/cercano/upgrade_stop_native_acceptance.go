//go:build ordinary_upgrade_acceptance && upgradefixture && darwin && cgo

package main

import (
	"cercano/source/server/internal/brewrestart"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func acceptanceHostStop(ctx context.Context, endpoint netip.AddrPort) error {
	pid, err := strconv.Atoi(os.Getenv("CERCANO_FIXTURE_OWNED_HOST"))
	if err != nil || pid <= 0 || os.Getenv("CERCANO_DISABLE_CREDENTIALS") != "1" {
		return fmt.Errorf("explicit owned fixture host required")
	}
	pids := []int{pid}
	for _, raw := range strings.Fields(os.Getenv("CERCANO_FIXTURE_ADDITIONAL_HOSTS")) {
		extra, err := strconv.Atoi(raw)
		if err != nil || extra <= 0 {
			return fmt.Errorf("invalid additional fixture host")
		}
		pids = append(pids, extra)
	}
	worker, err := strconv.Atoi(os.Getenv("CERCANO_FIXTURE_SIBLING_WORKER"))
	if err != nil || worker <= 0 {
		return fmt.Errorf("owned sibling worker required")
	}
	pids = append(pids, worker) // candidate only, never a host/descendant
	if raw := os.Getenv("CERCANO_FIXTURE_UNIDENTIFIED"); raw != "" {
		unrelated, err := strconv.Atoi(raw)
		if err != nil || unrelated <= 0 {
			return fmt.Errorf("explicit unrelated fixture PID required")
		}
		if _, err := brewrestart.Inspect(unrelated); !errors.Is(err, syscall.ENOENT) {
			return fmt.Errorf("fixture image must be unavailable: %v", err)
		}
		pids = append(pids, unrelated)
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
	if err := brewrestart.StopFixtureUpgradeHost(ctx, endpoint, pids...); err != nil {
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
