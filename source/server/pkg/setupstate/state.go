// Package setupstate owns the persisted onboarding contract shared by the terminal reset command and CLI.
package setupstate

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"

	"cercano/source/server/pkg/paths"
)

// Step names one wizard screen. Values are persisted in the state file, so
// they are stable identifiers, not display strings.
type Step string

const (
	StepResources  Step = "resources"
	StepPreference Step = "preference"
	StepLocus      Step = "locus" // legacy locus-first screen; migrated on resume
	StepCloud      Step = "cloud" // provider + auth; only when the locus uses cloud
	StepOpen       Step = "open"  // curated open-model set; only when the locus uses open
	StepDone       Step = "done"  // confirmation screen; apply and Clear complete the run
)

// State is every answer collected so far plus the current step. It is
// persisted after every transition so quitting mid-wizard resumes in place;
// a completed run clears the file.
type State struct {
	ReplaceTaskOverrides bool     `yaml:"replace_task_overrides,omitempty"`
	ExcludedProfiles     []string `yaml:"excluded_profiles,omitempty"`
	FlowVersion          int      `yaml:"flow_version,omitempty"`
	ResourcePage         int      `yaml:"resource_page,omitempty"` // legacy resource subpage: 1 migrates to providers, 2 to preferences
	Preference           string   `yaml:"preference,omitempty"`
	SecondaryProfile     string   `yaml:"secondary_profile,omitempty"`
	UseLocal             bool     `yaml:"use_local,omitempty"`

	// Explicit edits are separate from automatically displayed recommendations.
	CloudPicksEdited map[string]bool   `yaml:"cloud_picks_edited,omitempty" json:"cloud_picks_edited,omitempty"`
	Step             Step              `yaml:"step"`
	LocusMode        string            `yaml:"locus_mode,omitempty"`     // locus.Mode string value (the organizing answer)
	CloudProfile     string            `yaml:"cloud_profile,omitempty"`  // pending default account; applied only on confirmation
	CloudProvider    string            `yaml:"cloud_provider,omitempty"` // cloud preset ID
	AuthMethod       string            `yaml:"auth_method,omitempty"`    // meridian | chatgpt | device_code | api_key
	TierPicks        map[string]string `yaml:"tier_picks,omitempty"`     // "<tier>.<side>" → model id
	// Baseline is retained only to decode older progress. Cancellation never
	// restores it or deletes accounts; completed connections persist.
	Baseline *Baseline `yaml:"baseline,omitempty"`
}

// Baseline is legacy non-secret snapshot data; it is not used for rollback.
type Baseline struct {
	ActiveProfile string            `yaml:"active_profile"`
	Profiles      []ProfileSnapshot `yaml:"profiles,omitempty"`
}

// ProfileSnapshot is one cloud profile's metadata as it stood at wizard
// start. Field set mirrors agentclient.CloudProfileInfo minus HasKey (keys
// are not snapshottable, by design).
type ProfileSnapshot struct {
	DetailsCaptured bool              `yaml:"details_captured,omitempty"`
	Provider        string            `yaml:"provider,omitempty"`
	Region          string            `yaml:"region,omitempty"`
	AWSProfile      string            `yaml:"aws_profile,omitempty"`
	TierOverrides   map[string]string `yaml:"tier_overrides,omitempty"`
	ImageModel      string            `yaml:"image_model,omitempty"`
	Name            string            `yaml:"name"`
	Flavor          string            `yaml:"flavor"`
	Backend         string            `yaml:"backend,omitempty"`
	BaseURL         string            `yaml:"base_url,omitempty"`
	Model           string            `yaml:"model,omitempty"`
	Route           string            `yaml:"route,omitempty"`
}

// Fresh returns a new first-step run with no prior answers or rollback baseline.
func Fresh() State { return State{Step: StepResources, FlowVersion: 2} }

// StatePath resolves the resume file: wizard_state.yaml in the active
// profile's directory, beside the config.yaml the wizard writes.
// CERCANO_WIZARD_STATE overrides for tests, mirroring the uiconfig
// env-override convention. XDG_CONFIG_HOME no longer applies: the run belongs
// to the profile, and profile directories follow HOME like config.yaml does.
func StatePath() string {
	if p := os.Getenv("CERCANO_WIZARD_STATE"); p != "" {
		return p
	}
	return filepath.Join(paths.ActiveProfileDir(), "wizard_state.yaml")
}

// Load reads a persisted in-progress run. ok is false when there is
// nothing to resume (no file or unreadable). A file at StepDone still
// resumes — the run is only complete once the answers were applied, and
// Clear removes the file at that point; quitting on the summary screen
// must come back to it.
func Load() (s State, ok bool) {
	data, err := os.ReadFile(StatePath())
	if err != nil {
		return State{}, false
	}
	if yaml.Unmarshal(data, &s) != nil || s.Step == "" {
		return State{}, false
	}
	return s, true
}

// Save persists the run for resume. Called after every transition.
func Save(s State) error {
	path := StatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Clear removes the resume file; a missing file is not an error.
func Clear() error {
	err := os.Remove(StatePath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
