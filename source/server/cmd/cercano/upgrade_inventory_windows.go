package main

import (
	"cercano/source/server/internal/conversation"
	"context"
	"fmt"
	"os"
	"strings"
)

func openUpgradeInventory(ctx context.Context, state string) (func(), func(string, any) error, func(string) ([]os.DirEntry, error), error) {
	inventory, err := conversation.OpenUpgradeInventory(ctx, state)
	if err != nil {
		return nil, nil, nil, err
	}
	return inventory.Close, inventory.Read, func(string) ([]os.DirEntry, error) { return inventory.Entries() }, nil
}
func upgradeRecordedHostAbsent(context.Context, string) error {
	return fmt.Errorf("Windows cannot prove absence of a retained host, including renamed executables; retained inventory must remain intact; use a platform with verified host-identity discovery before upgrading")
}

// Windows manual upgrade supports no retained hosts. Only canonical record
// names may reach the shared semantic/absence checks (which still refuse all
// retained Windows hosts). Ancillary .token files, even without a record, and
// all unknown entries/directories are refused, never ignored or removed.
func validateUpgradeInventoryEntry(entry os.DirEntry) error {
	name := entry.Name()
	canonical := (strings.HasSuffix(name, ".json") && len(name) > len(".json")) ||
		(strings.HasSuffix(name, ".starting") && len(name) > len(".starting"))
	if entry.IsDir() || !canonical {
		return fmt.Errorf("unrecognized or noncanonical Windows inventory entry %q; retained inventory must remain intact", name)
	}
	return nil
}
