package config_test

import (
	"path/filepath"
	"sync"
	"testing"

	cfgsvc "cercano/source/server/internal/hostsvc/config"
	cfg "cercano/source/server/pkg/config"
)

// TestServiceRaceGet races concurrent Get()+scalar-read against Set() to
// exercise the basic struct-copy path under the race detector.
func TestServiceRaceGet(t *testing.T) {
	svc := cfgsvc.New("", cfg.Config{LocusMode: "cloud_primary"}, nil)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c := svc.Get()
				_ = c.LocusMode
				svc.Set(cfg.Config{LocusMode: "open_primary"})
			}
		}()
	}
	wg.Wait()
}

// TestServiceRaceCloudProfiles is the characterization test for the snapshot
// data race: concurrent Get()+full-slice-iteration must not race against
// UpsertProfile/RemoveProfile that mutate the same backing array.
//
// Without Clone() in Get(), -race detects: concurrent write in UpsertProfile
// (s.current.CloudProfiles[i] = p) vs read in the caller iterating the
// returned snapshot slice (same backing array).
//
// With Clone() this test must pass -race cleanly.
func TestServiceRaceCloudProfiles(t *testing.T) {
	profiles := make([]cfg.CloudProfile, 5)
	for i := range profiles {
		profiles[i] = cfg.CloudProfile{Name: string(rune('a' + i)), Model: "m"}
	}
	svc := cfgsvc.New("", cfg.Config{
		CloudProfiles:      profiles,
		ActiveCloudProfile: "a",
	}, nil)

	var wg sync.WaitGroup

	// Writers: interleave upserts and removes.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			name := string(rune('a' + idx%5))
			for j := 0; j < 200; j++ {
				if j%2 == 0 {
					svc.UpsertProfile(cfg.CloudProfile{Name: name, Model: "updated"})
				} else {
					svc.RemoveProfile(name)
					svc.UpsertProfile(cfg.CloudProfile{Name: name, Model: "restored"})
				}
			}
		}()
	}

	// Readers: get snapshot and iterate the entire CloudProfiles slice.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				snap := svc.Get()
				// Iterate every element — this is what would race against
				// UpsertProfile's in-place write if the snapshot shared the
				// backing array.
				for k := range snap.CloudProfiles {
					_ = snap.CloudProfiles[k].Name
					_ = snap.CloudProfiles[k].Model
				}
				// Also iterate Watchdog.Checks and LlamaServer.ModelDirs to
				// cover the other cloned slices.
				for k := range snap.Watchdog.Checks {
					_ = snap.Watchdog.Checks[k]
				}
				for k := range snap.LlamaServer.ModelDirs {
					_ = snap.LlamaServer.ModelDirs[k]
				}
			}
		}()
	}

	wg.Wait()
}

func TestServiceSetActiveProfile(t *testing.T) {
	svc := cfgsvc.New("", cfg.Config{
		CloudProfiles: []cfg.CloudProfile{
			{Name: "a", Model: "m1"},
			{Name: "b", Model: "m2"},
		},
	}, nil)

	if svc.SetActiveProfile("nope") {
		t.Error("SetActiveProfile should return false for unknown profile")
	}
	if !svc.SetActiveProfile("a") {
		t.Error("SetActiveProfile should return true for known profile")
	}
	c := svc.Get()
	if c.ActiveCloudProfile != "a" {
		t.Errorf("got ActiveCloudProfile=%q, want %q", c.ActiveCloudProfile, "a")
	}
}

func TestServiceUpsertProfile(t *testing.T) {
	svc := cfgsvc.New("", cfg.Config{
		CloudProfiles:      []cfg.CloudProfile{{Name: "a", Model: "m1"}},
		ActiveCloudProfile: "a",
	}, nil)

	replaced, isActive := svc.UpsertProfile(cfg.CloudProfile{Name: "a", Model: "m2"})
	if !replaced {
		t.Error("UpsertProfile should report replaced=true for existing name")
	}
	if !isActive {
		t.Error("UpsertProfile should report isActive=true for active profile")
	}
	c := svc.Get()
	if c.CloudProfiles[0].Model != "m2" {
		t.Errorf("got model %q, want %q", c.CloudProfiles[0].Model, "m2")
	}

	replaced, _ = svc.UpsertProfile(cfg.CloudProfile{Name: "new", Model: "m3"})
	if replaced {
		t.Error("UpsertProfile should report replaced=false for new name")
	}
	c = svc.Get()
	if len(c.CloudProfiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(c.CloudProfiles))
	}
}

func TestServiceRemoveProfile(t *testing.T) {
	svc := cfgsvc.New("", cfg.Config{
		CloudProfiles:      []cfg.CloudProfile{{Name: "a", Model: "m1"}, {Name: "b", Model: "m2"}},
		ActiveCloudProfile: "a",
	}, nil)

	existed, wasActive := svc.RemoveProfile("a")
	if !existed {
		t.Error("RemoveProfile should report existed=true")
	}
	if !wasActive {
		t.Error("RemoveProfile should report wasActive=true")
	}
	c := svc.Get()
	if c.ActiveCloudProfile != "" {
		t.Errorf("expected empty ActiveCloudProfile after removing active, got %q", c.ActiveCloudProfile)
	}
	if len(c.CloudProfiles) != 1 {
		t.Errorf("expected 1 profile after remove, got %d", len(c.CloudProfiles))
	}

	existed, wasActive = svc.RemoveProfile("nope")
	if existed || wasActive {
		t.Error("RemoveProfile of unknown name should return false, false")
	}
}

func TestServiceActiveProfile(t *testing.T) {
	svc := cfgsvc.New("", cfg.Config{
		CloudProfiles:      []cfg.CloudProfile{{Name: "a", Model: "m1"}},
		ActiveCloudProfile: "a",
	}, nil)

	p, ok := svc.ActiveProfile()
	if !ok {
		t.Error("ActiveProfile should return ok=true")
	}
	if p.Name != "a" || p.Model != "m1" {
		t.Errorf("unexpected profile %+v", p)
	}

	svc2 := cfgsvc.New("", cfg.Config{}, nil)
	_, ok = svc2.ActiveProfile()
	if ok {
		t.Error("ActiveProfile on empty config should return ok=false")
	}
}

func TestServiceMutate(t *testing.T) {
	svc := cfgsvc.New("", cfg.Config{LocusMode: "cloud_only"}, nil)
	svc.Mutate(func(c *cfg.Config) {
		c.LocusMode = "open_only"
	})
	c := svc.Get()
	if c.LocusMode != "open_only" {
		t.Errorf("Mutate did not update LocusMode: got %q", c.LocusMode)
	}
}

func TestServiceSetCloudModel(t *testing.T) {
	svc := cfgsvc.New("", cfg.Config{CloudModel: "old"}, nil)
	svc.SetCloudModel("new")
	c := svc.Get()
	if c.CloudModel != "new" {
		t.Errorf("SetCloudModel: got %q, want %q", c.CloudModel, "new")
	}
}

func TestServiceSetActiveProfileRejectsBackupCollision(t *testing.T) {
	svc := cfgsvc.New("", cfg.Config{ActiveCloudProfile: "a", BackupCloudProfile: "b", CloudProfiles: []cfg.CloudProfile{{Name: "a"}, {Name: "b"}}}, nil)
	if svc.SetActiveProfile("b") {
		t.Fatal("activation silently rewired the backup")
	}
	c := svc.Get()
	if c.ActiveCloudProfile != "a" || c.BackupCloudProfile != "b" {
		t.Fatal("rejected activation mutated bindings")
	}
}

func TestServiceSetActiveProfileRejectsSelfBackup(t *testing.T) {
	svc := cfgsvc.New("", cfg.Config{BackupCloudProfile: "b", CloudProfiles: []cfg.CloudProfile{{Name: "a"}, {Name: "b"}}}, nil)
	if svc.SetActiveProfile("b") {
		t.Fatal("activation accepted a self-backup")
	}
	c := svc.Get()
	if c.ActiveCloudProfile != "" || c.BackupCloudProfile != "b" {
		t.Fatal("rejection mutated bindings")
	}
}

func TestServiceSetBackupProfile(t *testing.T) {
	svc := cfgsvc.New("", cfg.Config{
		CloudProfiles: []cfg.CloudProfile{{Name: "a"}, {Name: "b"}},
	}, nil)
	if !svc.SetBackupProfile("b") {
		t.Error("SetBackupProfile should return true for existing profile")
	}
	c := svc.Get()
	if c.BackupCloudProfile != "b" {
		t.Errorf("got BackupCloudProfile=%q, want %q", c.BackupCloudProfile, "b")
	}
	if svc.SetBackupProfile("nope") {
		t.Error("SetBackupProfile should return false for unknown profile")
	}
	// Clearing (empty string) always ok.
	if !svc.SetBackupProfile("") {
		t.Error("SetBackupProfile with empty string should return true")
	}
}

func persistTestPair(t *testing.T) (a, b cfgsvc.Service) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	var svcs [2]cfgsvc.Service
	for i, name := range []string{"a.yaml", "b.yaml"} {
		path := filepath.Join(t.TempDir(), name)
		if err := cfg.Save(cfg.Defaults(), path); err != nil {
			t.Fatal(err)
		}
		c, err := cfg.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		svcs[i] = cfgsvc.New(path, c, nil)
	}
	return svcs[0], svcs[1]
}

func TestPersistDoesNotResendSameFieldOverNewerValue(t *testing.T) {
	a, b := persistTestPair(t)
	setISQ := func(s cfgsvc.Service, v string) {
		if err := s.Mutate(func(c *cfg.Config) { c.MistralRS.ISQ = v }); err != nil {
			t.Fatal(err)
		}
		s.Persist()
	}
	setISQ(a, "Q4K")
	setISQ(b, "Q8_0")
	// a changes only an unrelated field.
	if err := a.Mutate(func(c *cfg.Config) { c.Port = "50111" }); err != nil {
		t.Fatal(err)
	}
	a.Persist()
	mc, _ := cfg.LoadMachine()
	if mc.MistralRS.ISQ != "Q8_0" {
		t.Errorf("ISQ = %q, want Q8_0 (a re-sent its old edit)", mc.MistralRS.ISQ)
	}
}

func TestMarkMachineSavedSkippedWhenLiveSectionsChanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := cfg.Defaults()
	saved := c.MachineSections()
	saved.MistralRS.ISQ = "Q4K" // what a stale copy wrote
	c.MistralRS.ISQ = "Q2K"     // live config edited again since
	if c.MarkMachineSaved(saved) {
		t.Fatal("baseline advanced although live sections changed")
	}
	// The newer edit is still unsaved relative to the baseline, so it is sent.
	path := filepath.Join(t.TempDir(), "c.yaml")
	if err := cfg.Save(c, path); err != nil {
		t.Fatal(err)
	}
	mc, _ := cfg.LoadMachine()
	if mc.MistralRS.ISQ != "Q2K" {
		t.Errorf("ISQ = %q, want Q2K (edit lost)", mc.MistralRS.ISQ)
	}
}
