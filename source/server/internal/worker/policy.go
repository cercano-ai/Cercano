package worker

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// SpawnOptions selects an immutable published generation for a coexistence host.
// All fields are required. GenerationDir must be the exact directory, not a
// mutable current-generation alias. ManifestPath names a nonempty JSON manifest
// inside it; publication/contract compatibility remains the startup caller's job.
// ResourceRoot must be dedicated to coexistence, outside the legacy worker dir.
// HostInstance must be unique for each host lifetime and must never be reused.
type SpawnOptions struct {
	GenerationDir string
	ManifestPath  string
	HostInstance  string
	ResourceRoot  string
}

// SpawnPolicy is immutable after construction. Its zero value preserves legacy
// binary lookup and resource behavior. Copying a policy does not change identity.
type SpawnPolicy struct{ binary, manifest, generation, dir string }

func NewSpawnPolicy(o SpawnOptions) (SpawnPolicy, error) {
	if o.HostInstance == "" || !filepath.IsAbs(o.GenerationDir) || !filepath.IsAbs(o.ManifestPath) || !filepath.IsAbs(o.ResourceRoot) {
		return SpawnPolicy{}, fmt.Errorf("worker policy: host instance and absolute generation, manifest, resource paths required")
	}
	generation, err := filepath.EvalSymlinks(o.GenerationDir)
	if err != nil {
		return SpawnPolicy{}, err
	}
	if filepath.Clean(o.GenerationDir) != generation {
		return SpawnPolicy{}, fmt.Errorf("worker policy: generation must not be a symlink alias")
	}
	root, err := filepath.EvalSymlinks(o.ResourceRoot)
	if err != nil {
		return SpawnPolicy{}, fmt.Errorf("worker policy: resource root must already exist: %w", err)
	}
	legacy, _ := filepath.Abs(workerDir())
	if resolved, err := filepath.EvalSymlinks(legacy); err == nil {
		legacy = resolved
	}
	if within(legacy, root) {
		return SpawnPolicy{}, fmt.Errorf("worker policy: resource root overlaps legacy sweep")
	}
	sum := sha256.Sum256([]byte(generation + "\x00" + o.HostInstance))
	p := SpawnPolicy{binary: filepath.Join(generation, cercanoBinaryName), manifest: filepath.Clean(o.ManifestPath), generation: generation, dir: filepath.Join(root, fmt.Sprintf("w-%x", sum[:12]))}
	if err := p.validate(); err != nil {
		return SpawnPolicy{}, err
	}
	// Refuse reuse, including symlinks. Never adopt resources from an old host.
	if err := os.Mkdir(p.dir, 0700); err != nil {
		return SpawnPolicy{}, fmt.Errorf("worker policy: reserve host namespace: %w", err)
	}
	return p, nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && (len(rel) < 3 || rel[:3] != ".."+string(filepath.Separator))
}

func (p SpawnPolicy) validate() error {
	for _, path := range []string{p.binary, p.manifest} {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("worker policy: %w", err)
		}
		if !within(p.generation, resolved) || resolved != path {
			return fmt.Errorf("worker policy: generation member must not escape or alias: %s", path)
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("worker policy: not a regular file: %s", path)
		}
		if path == p.binary && runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
			return fmt.Errorf("worker policy: binary is not executable")
		}
	}
	b, err := os.ReadFile(p.manifest)
	if err != nil {
		return err
	}
	var manifest map[string]json.RawMessage
	if json.Unmarshal(b, &manifest) != nil || len(manifest) == 0 {
		return fmt.Errorf("worker policy: missing or invalid generation manifest")
	}
	return nil
}

func (p SpawnPolicy) workerBinary() (string, error) {
	if p.binary == "" {
		return findWorkerBinary()
	}
	if err := p.validate(); err != nil {
		return "", err
	}
	return p.binary, nil
}
