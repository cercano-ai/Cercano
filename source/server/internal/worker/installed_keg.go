package worker

import (
	"fmt"
	"os"
	"path/filepath"
)

// installedKegRoot recognizes Homebrew's versioned layout, not opt or PATH.
// Resolve first so either a Cellar executable or the public alias can select it.
func installedKegRoot(binary string) (string, bool) {
	bin := filepath.Dir(binary)
	keg := filepath.Dir(bin)
	formula := filepath.Dir(keg)
	return keg, filepath.Base(binary) == cercanoBinaryName && filepath.Base(bin) == "bin" &&
		filepath.Base(formula) == "cercano" && filepath.Base(filepath.Dir(formula)) == "Cellar"
}

// SelectInstalledKegPolicy selects only Homebrew installations. Other layouts
// retain ordinary development discovery. A resolution error is never discovery.
func SelectInstalledKegPolicy(executable string) (SpawnPolicy, bool, error) {
	if !filepath.IsAbs(executable) {
		return SpawnPolicy{}, false, fmt.Errorf("installed worker: absolute executable required")
	}
	binary, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return SpawnPolicy{}, false, err
	}
	if _, ok := installedKegRoot(binary); !ok {
		return SpawnPolicy{}, false, nil
	}
	p, err := NewInstalledKegPolicy(binary)
	return p, true, err
}

// NewInstalledKegPolicy pins the actual installed executable, without inventing
// a development manifest or granting prepared/coexistence authority. Socket and
// lifecycle behavior remain ordinary-agent behavior.
func NewInstalledKegPolicy(executable string) (SpawnPolicy, error) {
	if !filepath.IsAbs(executable) {
		return SpawnPolicy{}, fmt.Errorf("installed worker: absolute executable required")
	}
	binary, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return SpawnPolicy{}, err
	}
	keg, ok := installedKegRoot(binary)
	if !ok {
		return SpawnPolicy{}, fmt.Errorf("installed worker: expected Cellar/cercano/<version>/bin/cercano")
	}
	p := SpawnPolicy{binary: binary, generation: keg, installed: true}
	if err := p.validateInstalled(); err != nil {
		return SpawnPolicy{}, err
	}
	p.installedKegInfo, err = os.Stat(keg)
	if err != nil {
		return SpawnPolicy{}, err
	}
	p.installedInfo, err = os.Stat(binary)
	if err != nil {
		return SpawnPolicy{}, err
	}
	p.binaryDigest, err = generationDigest(binary)
	if err != nil {
		return SpawnPolicy{}, err
	}
	return p, nil
}

// ValidateInstalled is separate from ValidatePrepared: selection is not admission
// to protected generation/session services.
func (p SpawnPolicy) ValidateInstalled() error {
	if !p.installed {
		return fmt.Errorf("installed worker: installed keg policy required")
	}
	_, err := p.workerBinary()
	return err
}

func (p SpawnPolicy) validateInstalled() error {
	resolved, err := filepath.EvalSymlinks(p.binary)
	if err != nil {
		return fmt.Errorf("installed worker: selected keg unavailable: %w", err)
	}
	if resolved != p.binary {
		return fmt.Errorf("installed worker: selected executable changed path")
	}
	kegInfo, err := os.Stat(p.generation)
	if err != nil {
		return err
	}
	if !kegInfo.IsDir() || (p.installedKegInfo != nil && !os.SameFile(p.installedKegInfo, kegInfo)) {
		return fmt.Errorf("installed worker: selected keg changed identity")
	}
	info, err := os.Stat(p.binary)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("installed worker: selected executable is not a regular executable")
	}
	if p.installedInfo != nil && !os.SameFile(p.installedInfo, info) {
		return fmt.Errorf("installed worker: selected executable changed identity")
	}
	return nil
}
