package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Migration must account for every retained instance, not just current discovery.
// Hold the existing launch exclusion until migration completes. An uncertain or
// still-live development host blocks offline upgrade; upgrade-wide shutdown is
// not authority to discard its record or assume a startup intent never spawned.
func developmentUpgradeGuard(ctx context.Context, database string) (func(), error) {
	return developmentUpgradeGuardWith(ctx, database, upgradeRecordedHostAbsent)
}
func developmentUpgradeGuardWith(ctx context.Context, database string, absent func(context.Context, string) error) (func(), error) {
	state := filepath.Dir(database)
	hosts := filepath.Join(state, "hosts")
	release, read, list, err := openUpgradeInventory(ctx, state)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (func(), error) {
		release()
		return nil, fmt.Errorf("offline upgrade blocked by development host inventory: %w", err)
	}
	// Lock even before the first hosts directory exists: otherwise an initial
	// development launch could race an apparently empty migration inventory.
	entries, err := list(hosts)
	if os.IsNotExist(err) {
		return release, nil
	}
	if err != nil {
		return fail(err)
	}
	sources := map[string]string{}
	for _, entry := range entries {
		if err := validateUpgradeInventoryEntry(entry); err != nil {
			return fail(err)
		}
		name := entry.Name()
		suffix := ""
		if strings.HasSuffix(name, ".starting") {
			suffix = ".starting"
		} else if strings.HasSuffix(name, ".json") {
			suffix = ".json"
		} else {
			continue
		}
		base := filepath.Join(hosts, strings.TrimSuffix(name, suffix))
		host := ""
		if suffix == ".starting" {
			var intent map[string]string
			if err := read(base+suffix, &intent); err != nil {
				return fail(err)
			}
			if !developmentInstanceMatches(base, intent) {
				return fail(fmt.Errorf("invalid start intent %s", name))
			}
			host = intent["host"]
		} else {
			var record launchSelection
			if err := read(base+suffix, &record); err != nil {
				return fail(err)
			}
			host = record.Identity.GetHost()
			if host == "" || record.Identity.GetStore() != database || record.TokenFile != base+".token" {
				return fail(fmt.Errorf("invalid host record %s", name))
			}
		}
		if old, exists := sources[base]; exists && old != host {
			return fail(fmt.Errorf("host record/start intent mismatch: %s", base))
		}
		sources[base] = host
	}
	seen := map[string]bool{}
	for base, host := range sources {
		if seen[host] {
			return fail(fmt.Errorf("ambiguous host identity %s", host))
		}
		seen[host] = true
		if err := absent(ctx, host); err != nil {
			return fail(fmt.Errorf("%s (%s) is live or uncertain; stop all development writers before upgrading: %w", host, base, err))
		}
	}
	return release, nil
}
