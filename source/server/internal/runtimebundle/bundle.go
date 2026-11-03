// Package runtimebundle retains caller-inventoried installed runtime files.
// It does not discover dependencies, verify package authenticity, or select a
// session generation. The caller must supply the complete required inventory.
package runtimebundle

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Member is a required regular file. Path is a canonical slash-relative path;
// SHA256 describes independently expected bytes, not a hash inferred by Publish.
// Executable controls the private copy's execute bits (0700 versus 0600).
// Existing application manifests are opaque members, not changed by this API.
type Member struct {
	Path       string
	SHA256     [sha256.Size]byte
	Executable bool
}

func inventory(members []Member) error {
	if len(members) == 0 {
		return fmt.Errorf("runtime bundle: empty inventory")
	}
	seen := map[string]bool{}
	for _, m := range members {
		if !filepath.IsLocal(filepath.FromSlash(m.Path)) || !fs.ValidPath(m.Path) || m.Path == "." || strings.ContainsAny(m.Path, "\\:\x00") || seen[m.Path] {
			return fmt.Errorf("runtime bundle: unsafe or duplicate member %q", m.Path)
		}
		seen[m.Path] = true
	}
	for name := range seen {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if seen[parent] {
				return fmt.Errorf("runtime bundle: file/directory collision %q", parent)
			}
		}
	}
	return nil
}

// regular refuses symlink components. os.Root additionally confines opens even
// if the installed tree changes while copying. Source bytes are always hashed.
func regular(root *os.Root, name string) error {
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(filepath.FromSlash(strings.Join(parts[:i+1], "/")))
		if err != nil {
			return err
		}
		if i < len(parts)-1 {
			if !info.IsDir() {
				return fmt.Errorf("runtime bundle: non-directory component %q", name)
			}
		} else if !info.Mode().IsRegular() {
			return fmt.Errorf("runtime bundle: nonregular member %q", name)
		}
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func copyMember(ctx context.Context, source *os.Root, stage string, m Member) error {
	if err := regular(source, m.Path); err != nil {
		return err
	}
	in, err := source.Open(filepath.FromSlash(m.Path))
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("runtime bundle: nonregular opened member %q", m.Path)
	}
	dest := filepath.Join(stage, filepath.FromSlash(m.Path))
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	mode := os.FileMode(0600)
	if m.Executable {
		mode = 0700
	}
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), contextReader{ctx, in})
	if err == nil && string(h.Sum(nil)) != string(m.SHA256[:]) {
		err = fmt.Errorf("runtime bundle: digest mismatch %q", m.Path)
	}
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Validate rechecks every required member and digest. It does not infer missing
// inventory entries, establish authenticity, or validate application semantics.
func Validate(ctx context.Context, dir string, members []Member) error {
	if err := inventory(members); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, m := range members {
		if err := regular(root, m.Path); err != nil {
			return err
		}
		f, err := root.Open(filepath.FromSlash(m.Path))
		if err != nil {
			return err
		}
		info, statErr := f.Stat()
		if statErr != nil {
			f.Close()
			return statErr
		}
		if !info.Mode().IsRegular() || (m.Executable && runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0) {
			f.Close()
			return fmt.Errorf("runtime bundle: invalid member mode %q", m.Path)
		}
		h := sha256.New()
		_, err = io.Copy(h, contextReader{ctx, f})
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if string(h.Sum(nil)) != string(m.SHA256[:]) {
			return fmt.Errorf("runtime bundle: digest mismatch %q", m.Path)
		}
	}
	return ctx.Err()
}

// Publish copies into a fresh private slot under an existing caller-owned root.
// Only the returned slot/bundle directory is eligible: .staging is never eligible.
// Full staged validation precedes a same-filesystem directory rename. No current
// pointer or existing bundle is changed, and no failed stages are cleaned up.
// The root must be dedicated to runtime retention and not concurrently mutated
// by callers. Publication is process-interruption safe, not a power-loss durability
// promise. Callers must not admit a generation until this method succeeds.
func Publish(ctx context.Context, source, retainedRoot string, members []Member) (string, error) {
	members = append([]Member(nil), members...)
	if err := inventory(members); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(source) || !filepath.IsAbs(retainedRoot) {
		return "", fmt.Errorf("runtime bundle: absolute roots required")
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return "", err
	}
	defer root.Close()
	retainedRoot, err = filepath.EvalSymlinks(retainedRoot)
	if err != nil {
		return "", err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return "", err
	}
	for _, pair := range [][2]string{{source, retainedRoot}, {retainedRoot, source}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && (rel == "." || filepath.IsLocal(rel)) {
			return "", fmt.Errorf("runtime bundle: source and retention roots overlap")
		}
	}
	slot, err := os.MkdirTemp(retainedRoot, "generation-")
	if err != nil {
		return "", err
	}
	stage := filepath.Join(slot, ".staging")
	if err := os.Mkdir(stage, 0700); err != nil {
		return "", err
	}
	for _, m := range members {
		if err := copyMember(ctx, root, stage, m); err != nil {
			return "", fmt.Errorf("retain %s: %w", m.Path, err)
		}
	}
	if err := Validate(ctx, stage, members); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	bundle := filepath.Join(slot, "bundle")
	if err := os.Rename(stage, bundle); err != nil {
		return "", err
	}
	return bundle, nil
}
