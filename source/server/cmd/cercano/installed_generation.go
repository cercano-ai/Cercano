package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"cercano/source/server/internal/worker"
)

// Installed selection supplies executable identity only. Callers must still
// preflight the exact store and mint the existing validated startup binding.
// There is intentionally no dependency on the separately installed CLI.
func installedGeneration() (generation, binary, build string, err error) {
	binary, err = os.Executable()
	if err != nil {
		return
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return
	}
	policy, selected, e := worker.SelectInstalledKegPolicy(binary)
	if e != nil {
		err = e
		return
	}
	if !selected {
		err = fmt.Errorf("installed agent requires a canonical Homebrew agent keg")
		return
	}
	if err = policy.ValidateInstalled(); err != nil {
		return
	}
	var data []byte
	data, err = os.ReadFile(binary)
	if err != nil {
		return
	}
	generation = filepath.Dir(filepath.Dir(binary))
	build = fmt.Sprintf("installed-%x", sha256.Sum256(data))
	return
}
