// Package paths computes where per-profile data and machine-wide data live.
package paths

import (
	"errors"
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

func homeDir() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine the home directory (set HOME): %w", err)
	}
	if h == "" {
		return "", errors.New("cannot determine the home directory (set HOME): it is empty")
	}
	return h, nil
}

// CheckHome returns an error when the home directory is unknown or empty.
// Call it at process start so users get a readable error before any path is
// computed.
func CheckHome() error {
	_, err := homeDir()
	return err
}

func home() string {
	h, err := homeDir()
	if err != nil {
		panic("paths: " + err.Error())
	}
	return h
}

// ConfigRoot is $HOME/.config/cercano.
// It panics if the home directory is unknown; see CheckHome.
func ConfigRoot() string { return ConfigRootIn(home()) }

// ConfigRootIn is ConfigRoot for an explicit home directory.
func ConfigRootIn(home string) string { return filepath.Join(home, ".config", "cercano") }

// StateRoot is $HOME/.cercano.
// It panics if the home directory is unknown; see CheckHome.
func StateRoot() string { return StateRootIn(home()) }

// StateRootIn is StateRoot for an explicit home directory.
func StateRootIn(home string) string { return filepath.Join(home, ".cercano") }

// ProfilesRoot is the directory holding all profile directories.
// It panics if the home directory is unknown; see CheckHome.
func ProfilesRoot() string { return ProfilesRootIn(home()) }

// ProfilesRootIn is ProfilesRoot for an explicit home directory.
func ProfilesRootIn(home string) string { return filepath.Join(ConfigRootIn(home), "profiles") }

// ProfileDir returns the directory for the named profile. It panics on an
// invalid name or an unknown home directory; callers must validate first.
func ProfileDir(name string) string {
	if err := ValidateProfileName(name); err != nil {
		panic("paths: " + err.Error())
	}
	return filepath.Join(ProfilesRoot(), name)
}

// CurrentProfileDir returns the directory of the active profile. It panics if
// the home directory is unknown; see CheckHome.
func CurrentProfileDir() (string, error) {
	name, err := Profile()
	if err != nil {
		return "", err
	}
	return ProfileDir(name), nil
}

// ActiveProfileDir is CurrentProfileDir for callers that cannot return an
// error. When CERCANO_PROFILE is invalid it returns the default profile's
// directory; process entry points reject an invalid name before any profile
// file is opened, and config.Load reports it, so the fallback only keeps a
// helper from returning a relative path. It panics if the home directory is
// unknown; see CheckHome.
func ActiveProfileDir() string {
	if dir, err := CurrentProfileDir(); err == nil {
		return dir
	}
	return ProfileDir(DefaultProfile)
}

// MachineConfigPath is the machine-wide config file.
// It panics if the home directory is unknown; see CheckHome.
func MachineConfigPath() string { return MachineConfigPathIn(home()) }

// MachineConfigPathIn is MachineConfigPath for an explicit home directory.
func MachineConfigPathIn(home string) string {
	return filepath.Join(ConfigRootIn(home), "machine.yaml")
}

// ListProfiles returns the sorted valid profile directory names. A missing
// profiles root yields an empty list, not an error. It panics if the home
// directory is unknown; see CheckHome.
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
