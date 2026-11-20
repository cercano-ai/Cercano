//go:build !windows

package main

import (
	"cercano/source/server/internal/brewrestart"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

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
