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
	// logOut receives the lines printed while migrating.
	logOut io.Writer = os.Stderr
	// stopWait bounds the wait for a stopped legacy agent's port to close.
	stopWait = 15 * time.Second
	// stopPoll is the interval between liveness checks while waiting.
	stopPoll = 200 * time.Millisecond
	// rename is os.Rename for a move between the legacy and the profile
	// location; tests replace it to simulate different filesystems.
	rename = os.Rename
	// removeAll deletes a moved source; tests replace it to simulate a crash
	// part way through.
	removeAll = os.RemoveAll
	// afterStep, when set, runs after each step of a cross-filesystem move;
	// tests use it to simulate a crash between steps.
	afterStep func(step string) error
)

// Steps of a cross-filesystem move, in order.
const (
	stepCopied        = "copied"         // src copied to dst+tempSuffix
	stepSourceTrashed = "source-trashed" // src renamed to src+trashSuffix
	stepInstalled     = "installed"      // dst+tempSuffix renamed to dst
)

const (
	tempSuffix  = ".migrating"
	trashSuffix = ".migrated-trash"
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
// when the marker exists. Otherwise it takes a cross-process lock and, when
// legacy data exists, stops a pre-profile agent if one is answering and moves
// every legacy file. It then splits the runtime sections into machine.yaml
// and writes the marker. Any failure returns an error without the marker, so
// the next start retries; files already moved stay where they are.
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

	plan := legacyPlan(l)
	pending, err := plan.pending()
	if err != nil {
		return fmt.Errorf("look for existing Cercano data: %w", err)
	}
	moved := false
	if pending {
		// Only an install with legacy data can have a legacy agent running.
		if err := stopLegacyAgent(ctx, opts); err != nil {
			return err
		}
		if moved, err = plan.run(); err != nil {
			return fmt.Errorf("move existing Cercano data into the default profile: %w", err)
		}
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
		if errors.Is(err, errAgentRunning) {
			return err
		}
		return fmt.Errorf("check for a running Cercano agent: %w", err)
	}
	if !alive {
		return nil
	}
	if opts.StopLegacyAgent == nil {
		return errAgentRunning
	}
	if err := opts.StopLegacyAgent(ctx); err != nil {
		if errors.Is(err, errAgentRunning) {
			return err
		}
		return fmt.Errorf("%w (%v)", errAgentRunning, err)
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

type moveKind int

const (
	moveFile       moveKind = iota // set aside anything at dst, then move
	moveDB                         // a SQLite database and its siblings
	moveIfNew                      // only when nothing is at dst
	moveBestEffort                 // like moveFile; failures are ignored
)

type moveSpec struct {
	src, dst string
	kind     moveKind
}

type movePlan []moveSpec

// legacyPlan lists every pre-profile location and where it goes, in order.
func legacyPlan(l layout) movePlan {
	inRoot := func(name string) string { return filepath.Join(l.configRoot, name) }
	inProfile := func(name string) string { return filepath.Join(l.defaultDir, name) }
	var plan movePlan
	for _, name := range []string{"config.yaml", "permissions.yaml", "catalog-cache.json"} {
		plan = append(plan, moveSpec{inRoot(name), inProfile(name), moveFile})
	}
	for _, db := range []string{"telemetry.db", "conversations.db", "watchdog-audit.db"} {
		plan = append(plan, moveSpec{inRoot(db), inProfile(db), moveDB})
	}
	for _, name := range []string{
		"crash.log", "mcp.yaml", "mcp.json",
		"failures.jsonl", "turn-routing.jsonl", "stream-anomalies.jsonl",
		"hook_state.json", "scripts",
	} {
		plan = append(plan, moveSpec{inRoot(name), inProfile(name), moveFile})
	}
	// Wizard state: the previous code read $XDG_CONFIG_HOME/cercano when
	// XDG_CONFIG_HOME was set, so that copy is preferred. Neither replaces a
	// wizard state already in the profile.
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		plan = append(plan, moveSpec{filepath.Join(xdg, "cercano", "wizard_state.yaml"), inProfile("wizard_state.yaml"), moveIfNew})
	}
	plan = append(plan,
		moveSpec{inRoot("wizard_state.yaml"), inProfile("wizard_state.yaml"), moveIfNew},
		moveSpec{filepath.Join(l.stateRoot, "enterprise"), inProfile("enterprise"), moveFile},
		moveSpec{filepath.Join(l.home, ".cercano-dispatch.log"), filepath.Join(l.defaultDir, "logs", "dispatch.log"), moveFile},
	)
	// CLI debug logs are diagnostics only.
	for _, name := range []string{"tui-perf.log", "search-perf.jsonl", "colorprobe.log"} {
		plan = append(plan, moveSpec{inRoot(name), filepath.Join(l.defaultDir, "logs", name), moveBestEffort})
	}
	return plan
}

// pending reports whether any legacy source, or an interrupted
// cross-filesystem move of one, is still present.
func (p movePlan) pending() (bool, error) {
	for _, m := range p {
		srcs := []string{m.src, m.src + trashSuffix}
		if m.kind == moveDB {
			srcs = srcs[:0]
			for _, suffix := range sqliteSiblings {
				srcs = append(srcs, m.src+suffix, m.src+suffix+trashSuffix)
			}
		}
		for _, src := range srcs {
			if _, err := os.Lstat(src); err == nil {
				return true, nil
			} else if !os.IsNotExist(err) {
				return false, err
			}
		}
	}
	return false, nil
}

// run performs the plan and reports whether anything moved.
func (p movePlan) run() (bool, error) {
	moved := false
	for _, m := range p {
		var did bool
		var err error
		switch m.kind {
		case moveDB:
			did, err = moveDatabase(m.src, m.dst)
		case moveIfNew:
			did, err = moveIfAbsent(m.src, m.dst)
		default:
			did, err = moveReplacing(m.src, m.dst)
		}
		moved = moved || did
		if err != nil && m.kind != moveBestEffort {
			return moved, err
		}
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
// An interrupted cross-filesystem move of src is finished first.
func moveReplacing(src, dst string) (bool, error) {
	if done, err := finishInterruptedMove(src, dst); done || err != nil {
		return done, err
	}
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
	if done, err := finishInterruptedMove(src, dst); done || err != nil {
		return done, err
	}
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
		_, err := os.Lstat(aside)
		if err == nil {
			continue
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("set aside %s: %w", path, err)
		}
		if err := os.Rename(path, aside); err != nil {
			return fmt.Errorf("set aside %s: %w", path, err)
		}
		return nil
	}
}

// move renames src to dst, creating dst's parent. When they are on
// different filesystems it moves in steps that a crash at any point leaves
// recoverable (see finishInterruptedMove):
//
//  1. copy src to dst.migrating (beside dst);
//  2. rename src to src.migrated-trash (same filesystem as src), so src
//     itself never exists alongside a complete copy;
//  3. rename dst.migrating to dst;
//  4. delete src.migrated-trash.
func move(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	err := rename(src, dst)
	if err == nil || !isCrossDevice(err) {
		return err
	}
	tmp, trash := dst+tempSuffix, src+trashSuffix
	if err := removeAll(tmp); err != nil {
		return err
	}
	if err := copyTree(src, tmp); err != nil {
		_ = removeAll(tmp)
		return fmt.Errorf("copy %s: %w", src, err)
	}
	if err := step(stepCopied); err != nil {
		return err
	}
	if err := os.Rename(src, trash); err != nil {
		return err
	}
	if err := step(stepSourceTrashed); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	if err := step(stepInstalled); err != nil {
		return err
	}
	return removeAll(trash)
}

func step(name string) error {
	if afterStep == nil {
		return nil
	}
	return afterStep(name)
}

// finishInterruptedMove completes a cross-filesystem move that a crash cut
// short. The state is unambiguous because src is renamed to the trash name
// only after the copy is complete:
//
//   - no trash: nothing to finish (src, if present, was never trashed; a
//     partial dst.migrating is discarded when the move is redone);
//   - trash and dst: the copy was installed; delete the rest of the trash;
//   - trash and dst.migrating: install the complete copy, delete the trash;
//   - trash only: restore it to src so the move is redone.
//
// done reports that src's move is now complete.
func finishInterruptedMove(src, dst string) (done bool, err error) {
	trash, tmp := src+trashSuffix, dst+tempSuffix
	if _, err := os.Lstat(trash); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if _, err := os.Lstat(dst); err == nil {
		return true, removeAll(trash)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if _, err := os.Lstat(tmp); err == nil {
		if err := os.Rename(tmp, dst); err != nil {
			return false, err
		}
		return true, removeAll(trash)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if _, err := os.Lstat(src); err == nil {
		return false, fmt.Errorf("both %s and %s exist; remove the one that is not current and retry", src, trash)
	}
	return false, os.Rename(trash, src)
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
// (config.Load reports that as before) or when it has neither section. A
// machine.yaml that already exists was not written from these settings (the
// marker is absent, so no migration wrote it: an earlier test run or build
// did), and it would otherwise replace them; it is set aside as
// machine.yaml.pre-migration rather than overwritten.
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
	if err := setAside(l.machinePath); err != nil {
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
