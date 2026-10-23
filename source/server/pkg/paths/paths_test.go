package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidateProfileName(t *testing.T) {
	for _, ok := range []string{"default", "work", "acme-2", strings.Repeat("a", 32)} {
		if err := ValidateProfileName(ok); err != nil {
			t.Errorf("ValidateProfileName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "Work", "../x", "a/b", "my profile", "-x", strings.Repeat("a", 33)} {
		err := ValidateProfileName(bad)
		if err == nil {
			t.Errorf("ValidateProfileName(%q) = nil, want error", bad)
			continue
		}
		if !strings.Contains(err.Error(), "lowercase letters, digits and hyphens") {
			t.Errorf("ValidateProfileName(%q) error %q lacks rule text", bad, err)
		}
	}
}

func TestProfileDefaultsWhenUnset(t *testing.T) {
	t.Setenv("CERCANO_PROFILE", "")
	got, err := Profile()
	if err != nil || got != DefaultProfile {
		t.Fatalf("Profile() = %q, %v; want %q", got, err, DefaultProfile)
	}
}

func TestProfileRejectsInvalidEnv(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	t.Setenv("CERCANO_PROFILE", "../x")
	if _, err := Profile(); err == nil {
		t.Fatal("Profile() error = nil, want error")
	}
	if _, err := CurrentProfileDir(); err == nil {
		t.Fatal("CurrentProfileDir() error = nil, want error")
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatalf("expected nothing created under HOME, found %v", entries)
	}
}

func TestPathsFollowHome(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	if got, want := ProfileDir("work"), filepath.Join(home, ".config", "cercano", "profiles", "work"); got != want {
		t.Errorf("ProfileDir = %q, want %q", got, want)
	}
	if got, want := StateRoot(), filepath.Join(home, ".cercano"); got != want {
		t.Errorf("StateRoot = %q, want %q", got, want)
	}
	if got, want := MachineConfigPath(), filepath.Join(home, ".config", "cercano", "machine.yaml"); got != want {
		t.Errorf("MachineConfigPath = %q, want %q", got, want)
	}
}

func TestProfileDirPanicsOnInvalid(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	ProfileDir("../x")
}

func TestListProfilesSkipsInvalidDirs(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	got, err := ListProfiles()
	if err != nil || len(got) != 0 {
		t.Fatalf("missing root: got %v, %v; want empty, nil", got, err)
	}
	root := filepath.Join(home, ".config", "cercano", "profiles")
	for _, d := range []string{"work", "default", "Bad"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"default", "work"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestCheckHomeErrorsWhenUnset(t *testing.T) {
	setHome(t, "")
	err := CheckHome()
	if err == nil {
		t.Fatal("CheckHome() = nil, want error")
	}
	if !strings.Contains(err.Error(), "cannot determine the home directory") {
		t.Errorf("unexpected message: %v", err)
	}
	setHome(t, t.TempDir())
	if err := CheckHome(); err != nil {
		t.Errorf("CheckHome() with home set = %v", err)
	}
}

func TestPathsPanicWhenHomeUnknown(t *testing.T) {
	setHome(t, "")
	for name, fn := range map[string]func(){
		"ConfigRoot": func() { ConfigRoot() },
		"StateRoot":  func() { StateRoot() },
		"ProfileDir": func() { ProfileDir("work") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()
			fn()
		}()
	}
}

func TestActiveProfileDirFallsBackToDefault(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	t.Setenv("CERCANO_PROFILE", "work")
	if got, want := ActiveProfileDir(), filepath.Join(home, ".config", "cercano", "profiles", "work"); got != want {
		t.Errorf("ActiveProfileDir() = %q, want %q", got, want)
	}
	t.Setenv("CERCANO_PROFILE", "../x")
	if got, want := ActiveProfileDir(), filepath.Join(home, ".config", "cercano", "profiles", "default"); got != want {
		t.Errorf("invalid profile: ActiveProfileDir() = %q, want %q", got, want)
	}
}

func TestExplicitHomeVariantsMatchCurrentHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	rows := []struct{ name, explicit, current string }{
		{"ConfigRoot", ConfigRootIn(home), ConfigRoot()},
		{"StateRoot", StateRootIn(home), StateRoot()},
		{"ProfilesRoot", ProfilesRootIn(home), ProfilesRoot()},
		{"MachineConfigPath", MachineConfigPathIn(home), MachineConfigPath()},
	}
	for _, r := range rows {
		if r.explicit != r.current {
			t.Errorf("%sIn(home) = %q, %s() = %q", r.name, r.explicit, r.name, r.current)
		}
	}
}
