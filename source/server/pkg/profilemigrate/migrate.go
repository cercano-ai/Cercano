// Package profilemigrate moves data written by Cercano before profiles
// existed into the default profile, once per home directory.
//
// Before profiles, every profile-owned file lived directly in
// ~/.config/cercano (plus ~/.cercano/enterprise and ~/.cercano-dispatch.log).
// Run moves those files into ~/.config/cercano/profiles/default, moves the
// machine-wide local runtime sections of config.yaml into machine.yaml, and
// writes a marker so later starts return immediately. It must run before any
// profile file is opened.
package profilemigrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"cercano/source/server/internal/filelock"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"

	"gopkg.in/yaml.v3"
)

// MarkerName is the file in the profiles root whose presence means the
// migration has completed. Nothing else counts: a profiles/default directory
// can exist before migration (tests and older builds create one).
const MarkerName = ".migrated-v1"

// Options configures Run.
type Options struct {
	// Home is the home directory. Empty means the current user's.
	Home string
	// LegacyAgentAlive reports whether an agent started before profiles is
	// answering on its port. Nil means none is checked for.
	LegacyAgentAlive func(ctx context.Context) (bool, error)
	// StopLegacyAgent asks that agent to shut down.
	StopLegacyAgent func(ctx context.Context) error
}

var (
	// logOut receives the one line printed when files are moved.
	logOut io.Writer = os.Stderr
	// stopWait bounds the wait for a stopped legacy agent to go away.
	stopWait = 15 * time.Second
	// stopPoll is the interval between liveness checks while waiting.
	stopPoll = 200 * time.Millisecond
	// rename is os.Rename; tests replace it to simulate other filesystems.
	rename = os.Rename
)

// errAgentRunning is returned when a pre-profile agent would not stop.
var errAgentRunning = errors.New("a Cercano agent started before this upgrade is still running and did not stop; quit Cercano (and any editor using it) and retry")

type layout struct {
	home, configRoot, stateRoot, profilesRoot, defaultDir, machinePath string
}

func resolveLayout(home string) (layout, error) {
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return layout{}, fmt.Errorf("cannot determine the home directory (set HOME): %w", err)
		}
		if h == "" {
			return layout{}, errors.New("cannot determine the home directory (set HOME): it is empty")
		}
		home = h
	}
	return layout{
		home:         home,
		configRoot:   paths.ConfigRootIn(home),
		stateRoot:    paths.StateRootIn(home),
		profilesRoot: paths.ProfilesRootIn(home),
		defaultDir:   filepath.Join(paths.ProfilesRootIn(home), paths.DefaultProfile),
		machinePath:  paths.MachineConfigPathIn(home),
	}, nil
}

func (l layout) marker() string { return filepath.Join(l.profilesRoot, MarkerName) }

func (l layout) migrated() bool {
	_, err := os.Stat(l.marker())
	return err == nil
}

// Run moves pre-profile data into the default profile. It returns at once
// when the marker exists. Otherwise it takes a cross-process lock, stops a
// pre-profile agent if one is answering, moves every legacy file that exists,
// splits the runtime sections into machine.yaml and writes the marker. Any
// move failure returns an error without the marker, so the next start
// retries; files already moved stay where they are.
func Run(ctx context.Context, opts Options) error {
	l, err := resolveLayout(opts.Home)
	if err != nil {
		return err
	}
	if l.migrated() {
		return nil
	}
	if err := os.MkdirAll(l.configRoot, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", l.configRoot, err)
	}
	release, err := filelock.Lock(ctx, filepath.Join(l.configRoot, "migrate.lock"))
	if err != nil {
		return fmt.Errorf("wait for another Cercano process to finish moving data into the default profile: %w", err)
	}
	defer release()
	if l.migrated() {
		return nil
	}

	if err := stopLegacyAgent(ctx, opts); err != nil {
		return err
	}
	moved, err := moveLegacyData(l)
	if err != nil {
		return fmt.Errorf("move existing Cercano data into the default profile: %w", err)
	}
	if err := splitRuntimeSettings(l); err != nil {
		return fmt.Errorf("move local runtime settings into %s: %w", l.machinePath, err)
	}
	if err := os.MkdirAll(l.profilesRoot, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", l.profilesRoot, err)
	}
	if err := os.WriteFile(l.marker(), []byte("pre-profile data moved into profiles/default\n"), 0o600); err != nil {
		return fmt.Errorf("write migration marker: %w", err)
	}
	if moved {
		fmt.Fprintln(logOut, "Moved existing Cercano data into the default profile")
	}
	return nil
}

// stopLegacyAgent shuts down a pre-profile agent so no file is moved while it
// has it open. Any agent answering here is pre-profile: newer agents migrate
// before they listen.
func stopLegacyAgent(ctx context.Context, opts Options) error {
	if opts.LegacyAgentAlive == nil {
		return nil
	}
	alive, err := opts.LegacyAgentAlive(ctx)
	if err != nil {
		return fmt.Errorf("check for a running Cercano agent: %w", err)
	}
	if !alive {
		return nil
	}
	if opts.StopLegacyAgent == nil {
		return errAgentRunning
	}
	if err := opts.StopLegacyAgent(ctx); err != nil {
		return fmt.Errorf("%w (shutdown request failed: %v)", errAgentRunning, err)
	}
	deadline := time.Now().Add(stopWait)
	for {
		alive, err := opts.LegacyAgentAlive(ctx)
		if err == nil && !alive {
			return nil
		}
		if !time.Now().Before(deadline) {
			return errAgentRunning
		}
		timer := time.NewTimer(stopPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// sqliteSiblings are the files SQLite keeps beside a database. They travel
// with it; a stale one left beside a moved database could be replayed into it.
var sqliteSiblings = []string{"", "-wal", "-shm", "-journal"}

// moveLegacyData moves every legacy file that exists and reports whether
// anything moved.
func moveLegacyData(l layout) (bool, error) {
	moved := false
	note := func(did bool, err error) error {
		moved = moved || did
		return err
	}
	inRoot := func(name string) string { return filepath.Join(l.configRoot, name) }
	inProfile := func(name string) string { return filepath.Join(l.defaultDir, name) }

	for _, name := range []string{"config.yaml", "permissions.yaml", "catalog-cache.json"} {
		if err := note(moveReplacing(inRoot(name), inProfile(name))); err != nil {
			return moved, err
		}
	}
	for _, db := range []string{"telemetry.db", "conversations.db", "watchdog-audit.db"} {
		if err := note(moveDatabase(inRoot(db), inProfile(db))); err != nil {
			return moved, err
		}
	}
	for _, name := range []string{
		"crash.log", "mcp.yaml", "mcp.json",
		"failures.jsonl", "turn-routing.jsonl", "stream-anomalies.jsonl",
		"hook_state.json", "scripts",
	} {
		if err := note(moveReplacing(inRoot(name), inProfile(name))); err != nil {
			return moved, err
		}
	}

	// Wizard state: the previous code read $XDG_CONFIG_HOME/cercano when
	// XDG_CONFIG_HOME was set, so that copy is preferred. Neither replaces a
	// wizard state already in the profile.
	wizardDst := inProfile("wizard_state.yaml")
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		if err := note(moveIfAbsent(filepath.Join(xdg, "cercano", "wizard_state.yaml"), wizardDst)); err != nil {
			return moved, err
		}
	}
	if err := note(moveIfAbsent(inRoot("wizard_state.yaml"), wizardDst)); err != nil {
		return moved, err
	}

	if err := note(moveReplacing(filepath.Join(l.stateRoot, "enterprise"), inProfile("enterprise"))); err != nil {
		return moved, err
	}
	if err := note(moveReplacing(filepath.Join(l.home, ".cercano-dispatch.log"), filepath.Join(l.defaultDir, "logs", "dispatch.log"))); err != nil {
		return moved, err
	}

	// CLI debug logs are diagnostics only: a failure does not block the
	// migration.
	for _, name := range []string{"tui-perf.log", "search-perf.jsonl", "colorprobe.log"} {
		did, _ := moveReplacing(inRoot(name), filepath.Join(l.defaultDir, "logs", name))
		moved = moved || did
	}
	return moved, nil
}

// moveDatabase moves a SQLite database and its sibling files. The database
// moves first; on a retry after a partial move only the remaining siblings
// are left at the legacy path, and those replace nothing of the database's.
func moveDatabase(src, dst string) (bool, error) {
	if _, err := os.Lstat(src); err == nil {
		// Leftovers in the profile from another source must not pair with
		// the moved database.
		for _, suffix := range sqliteSiblings {
			if err := setAside(dst + suffix); err != nil {
				return false, err
			}
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	moved := false
	for _, suffix := range sqliteSiblings {
		did, err := moveReplacing(src+suffix, dst+suffix)
		moved = moved || did
		if err != nil {
			return moved, err
		}
	}
	return moved, nil
}

// moveReplacing moves src to dst when src exists. Something already at dst
// was not put there by this migration (the marker is absent), so it is set
// aside as dst.pre-migration rather than overwritten. A destination of the
// other kind (a directory where a file goes, or the reverse) is an error.
func moveReplacing(src, dst string) (bool, error) {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if dstInfo, err := os.Lstat(dst); err == nil {
		if dstInfo.IsDir() != srcInfo.IsDir() {
			return false, fmt.Errorf("cannot move %s: %s is in the way", src, dst)
		}
		if err := setAside(dst); err != nil {
			return false, err
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := move(src, dst); err != nil {
		return false, err
	}
	return true, nil
}

// moveIfAbsent moves src to dst only when src exists and dst does not.
func moveIfAbsent(src, dst string) (bool, error) {
	if _, err := os.Lstat(dst); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return moveReplacing(src, dst)
}

// setAside renames an existing path to the first free name of the form
// path.pre-migration, path.pre-migration-2, ...
func setAside(path string) error {
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for i := 1; ; i++ {
		aside := path + ".pre-migration"
		if i > 1 {
			aside += "-" + strconv.Itoa(i)
		}
		if _, err := os.Lstat(aside); os.IsNotExist(err) {
			if err := os.Rename(path, aside); err != nil {
				return fmt.Errorf("set aside %s: %w", path, err)
			}
			return nil
		}
	}
}

// move renames src to dst, creating dst's parent. When src is on another
// filesystem it copies into a temporary name beside dst, renames that into
// place and then removes src.
func move(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	err := rename(src, dst)
	if err == nil || !isCrossDevice(err) {
		return err
	}
	tmp := dst + ".migrating"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := copyTree(src, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("copy %s: %w", src, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	return os.RemoveAll(src)
}

// copyTree copies a file, symlink or directory tree, keeping permission bits.
// Other file types (sockets, devices) are runtime artifacts and are skipped.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		}
		return nil
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// The umask may have narrowed the mode at create time.
	return os.Chmod(dst, mode)
}

// machineSections are the config.yaml keys that belong to machine.yaml.
var machineSections = map[string]bool{"llama_server": true, "mistralrs": true}

// splitRuntimeSettings moves the llama_server and mistralrs sections of the
// default profile's config.yaml into machine.yaml, leaving every other key,
// and the comments, as they were.
//
// It changes nothing when the config is missing or cannot be parsed
// (config.Load reports that as before), when it has neither section, or when
// machine.yaml already exists: machine.yaml is never overwritten, and since
// it then takes precedence the profile's copies are ignored on load and
// dropped by the next save.
func splitRuntimeSettings(l layout) error {
	path := filepath.Join(l.defaultDir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	root := doc.Content[0]
	machine := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	kept := make([]*yaml.Node, 0, len(root.Content))
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if machineSections[key.Value] {
			machine.Content = append(machine.Content, key, value)
			continue
		}
		kept = append(kept, key, value)
	}
	if len(machine.Content) == 0 {
		return nil
	}
	if _, err := os.Lstat(l.machinePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	machineData, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{machine}})
	if err != nil {
		return err
	}
	// Leave the profile file alone if the sections would not load as machine
	// settings; config.Load reports such a file the same way it did before.
	var check config.MachineConfig
	if err := yaml.Unmarshal(machineData, &check); err != nil {
		return nil
	}
	root.Content = kept
	profileData, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	if err := writeAtomic(l.machinePath, machineData, 0o600); err != nil {
		return err
	}
	mode := fs.FileMode(0o600)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	return writeAtomic(path, profileData, mode)
}

// writeAtomic writes data to a temporary file beside path and renames it
// into place.
func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, mode); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
