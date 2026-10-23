// Package paths computes where per-profile data and machine-wide data live.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// DefaultProfile is the profile used when CERCANO_PROFILE is unset.
const DefaultProfile = "default"

var profileNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// ValidateProfileName reports whether name is a legal profile name.
func ValidateProfileName(name string) error {
	if !profileNameRE.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: use 1-32 lowercase letters, digits and hyphens, starting with a letter or digit", name)
	}
	return nil
}

// Profile returns the active profile name from CERCANO_PROFILE (default if empty).
func Profile() (string, error) {
	name := os.Getenv("CERCANO_PROFILE")
	if name == "" {
		name = DefaultProfile
	}
	if err := ValidateProfileName(name); err != nil {
		return "", err
	}
	return name, nil
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// ConfigRoot is $HOME/.config/cercano.
func ConfigRoot() string { return filepath.Join(home(), ".config", "cercano") }

// StateRoot is $HOME/.cercano.
func StateRoot() string { return filepath.Join(home(), ".cercano") }

// ProfilesRoot is the directory holding all profile directories.
func ProfilesRoot() string { return filepath.Join(ConfigRoot(), "profiles") }

// ProfileDir returns the directory for the named profile. It panics on an
// invalid name; callers must validate first.
func ProfileDir(name string) string {
	if err := ValidateProfileName(name); err != nil {
		panic("paths: " + err.Error())
	}
	return filepath.Join(ProfilesRoot(), name)
}

// CurrentProfileDir returns the directory of the active profile.
func CurrentProfileDir() (string, error) {
	name, err := Profile()
	if err != nil {
		return "", err
	}
	return ProfileDir(name), nil
}

// MachineConfigPath is the machine-wide config file.
func MachineConfigPath() string { return filepath.Join(ConfigRoot(), "machine.yaml") }

// ListProfiles returns the sorted valid profile directory names. A missing
// profiles root yields an empty list, not an error.
func ListProfiles() ([]string, error) {
	entries, err := os.ReadDir(ProfilesRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() && ValidateProfileName(e.Name()) == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
