package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"cercano/source/server/internal/setupresetcmd"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/profilemigrate"
)

// profileFile returns name beside the active profile's config.yaml, where
// every profile-owned file (crash.log, telemetry.db, permissions.yaml,
// catalog-cache.json, mcp.yaml) lives.
func profileFile(name string) string {
	return filepath.Join(filepath.Dir(config.DefaultPath()), name)
}

// dispatchLogPath is the dispatch diagnostics log: logs/dispatch.log in the
// active profile's directory.
func dispatchLogPath() string {
	return filepath.Join(paths.ActiveProfileDir(), "logs", "dispatch.log")
}

// updateCacheDir holds update_check.json. A release check is about the
// installed binary, not a profile, so the cache is machine-wide.
func updateCacheDir() string { return paths.ConfigRoot() }

// checkStartupEnvironment fails when profile and machine paths cannot be
// resolved: no home directory, or an invalid CERCANO_PROFILE. Running anyway
// would scatter data into the working directory or quietly use the default
// profile's data.
func checkStartupEnvironment() error {
	if err := paths.CheckHome(); err != nil {
		return err
	}
	if _, err := paths.Profile(); err != nil {
		return fmt.Errorf("CERCANO_PROFILE: %w", err)
	}
	return nil
}

// needsStartupCheck reports whether the command in args reads or writes
// profile or machine state. Only commands that print static information or
// write into an explicit directory skip the check.
func needsStartupCheck(args []string) bool {
	if len(args) < 2 {
		return true
	}
	switch args[1] {
	case "version", "-version", "--version", "protocols", "skills":
		return false
	}
	return true
}

// requireStartupEnvironment exits with a readable error when
// checkStartupEnvironment fails.
func requireStartupEnvironment(args []string) {
	if !needsStartupCheck(args) {
		return
	}
	if err := checkStartupEnvironment(); err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		os.Exit(1)
	}
}

// migrationOptions builds the options for moving pre-profile data; tests
// replace it so they never probe or stop a real agent.
var migrationOptions = profilemigrate.DefaultOptions

// needsProfileMigration reports whether the command in args must move
// pre-profile data into the default profile before it starts. It does not
// for:
//   - worker: spawned by an agent that has already migrated;
//   - model-host: machine-wide;
//   - restart-after-upgrade: the Homebrew coordinator stops the pre-profile
//     agent and relaunches the new binary, which migrates on startup;
//   - a help request that the command handles before touching any data,
//     recognized with the command's own argument parsing;
//   - commands that skip the startup check, which touch no profile data.
//
// agent and stats ignore help flags, so they always migrate.
func needsProfileMigration(args []string) bool {
	if !needsStartupCheck(args) {
		return false
	}
	if len(args) < 2 {
		return true
	}
	rest := args[2:]
	switch args[1] {
	case "worker", "model-host", "restart-after-upgrade":
		return false
	case "agent", "stats":
		return true
	case "setup":
		_, help := parseSetupArgs(rest)
		return !help
	case "run":
		return !parseHeadlessArgs(rest).help
	case "reset":
		return !setupresetcmd.HelpRequested(rest)
	case "logs":
		fs, _, _ := newLogsFlags(flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		return !errors.Is(fs.Parse(rest), flag.ErrHelp)
	case "export":
		if len(rest) == 0 || rest[0] != "trajectory" {
			return true
		}
		fs, f := newExportTrajectoryFlags(io.Discard)
		err := fs.Parse(rest[1:])
		return !(errors.Is(err, flag.ErrHelp) || err == nil && *f.help)
	case "enterprise":
		return len(rest) > 0 && rest[0] != "--help" && rest[0] != "help"
	}
	// No subcommand: the top-level flags. Parsing stops at the first
	// non-flag argument, so an unknown subcommand followed by --help is not
	// a help request.
	fs := flag.NewFlagSet("cercano", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	defineTopLevelFlags(fs)
	return !errors.Is(fs.Parse(args[1:]), flag.ErrHelp)
}

// parseSetupArgs reads `cercano setup` arguments: --install-engine, and -h
// or --help, which prints usage and changes nothing.
func parseSetupArgs(args []string) (installEngine, help bool) {
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			return installEngine, true
		case "--install-engine":
			installEngine = true
		}
	}
	return installEngine, false
}

// topLevelFlags are the flags of a bare `cercano` invocation.
type topLevelFlags struct {
	mcp, version, stats, server *bool
	grpcAddr                    *string
}

func defineTopLevelFlags(fs *flag.FlagSet) topLevelFlags {
	return topLevelFlags{
		mcp:      fs.Bool("mcp", false, "Run in MCP mode (embedded gRPC server + MCP on stdio)"),
		grpcAddr: fs.String("grpc-addr", "", "Address of an external gRPC server (MCP-only, no embedded server)"),
		version:  fs.Bool("version", false, "Print version and exit"),
		stats:    fs.Bool("stats", false, "Print usage statistics and exit"),
		server:   fs.Bool("server", false, "Force server mode (equivalent to `cercano agent`)"),
	}
}

// migrateProfileData moves pre-profile data into the default profile when
// the command needs it. It returns at once after the first successful run.
func migrateProfileData(args []string) error {
	if !needsProfileMigration(args) {
		return nil
	}
	// Covers waiting for another process's migration and a pre-profile
	// agent's shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return profilemigrate.Run(ctx, migrationOptions())
}

// requireProfileMigration exits with a readable error when the migration
// fails; running on would open an empty profile beside the user's data.
func requireProfileMigration(args []string) {
	if err := migrateProfileData(args); err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		os.Exit(1)
	}
}
