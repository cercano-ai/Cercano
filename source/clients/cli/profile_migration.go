package main

import (
	"context"
	"time"

	"cercano/source/server/pkg/profilemigrate"
)

// migrationOptions builds the options for moving pre-profile data; tests
// replace it so they never probe or stop a real agent.
var migrationOptions = profilemigrate.DefaultOptions

// migrateProfileData moves data written before profiles existed into the
// default profile. It returns at once after the first successful run.
func migrateProfileData() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return profilemigrate.Run(ctx, migrationOptions())
}
