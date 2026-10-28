package gitflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cercano/source/server/internal/procx"
)

// Patch reads only working-tree or index changes. No revisions, options,
// pathspec magic, external diff drivers, textconv or submodule commands.
func (r *Repo) Patch(ctx context.Context, paths []string, staged bool, maxBytes int) (string, bool, error) {
	if maxBytes < 1 || maxBytes > 65536 || len(paths) > 100 {
		return "", false, fmt.Errorf("invalid patch bounds")
	}
	for _, p := range paths {
		if p == "" || len(p) > 4096 || strings.ContainsAny(p, "\x00\r\n") || filepath.IsAbs(p) || p == ".." || strings.HasPrefix(filepath.Clean(p), ".."+string(filepath.Separator)) {
			return "", false, fmt.Errorf("invalid relative patch path")
		}
	}
	args := []string{"git", "--no-pager", "--literal-pathspecs", "-c", "core.fsmonitor=false", "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--ignore-submodules=all", "--submodule=short", "--unified=3"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--")
	args = append(args, paths...)
	// Inherited GIT_* variables can redirect the repository or inject config.
	env := []string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_") {
			env = append(env, e)
		}
	}
	env = append(env, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	res, err := procx.Run(ctx, procx.Options{Args: args, Dir: r.Dir, Env: env, Timeout: gitQueryTimeout, MaxOutputBytes: maxBytes})
	if err != nil {
		return "", false, err
	}
	if res.ExitCode != 0 {
		return "", false, fmt.Errorf("git patch failed (exit %d): %s", res.ExitCode, res.Stderr)
	}
	return string(res.Stdout), res.OutputTruncated, nil
}
