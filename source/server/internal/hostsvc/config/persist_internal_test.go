package config

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cfg "cercano/source/server/pkg/config"
)

// Two Persists can overlap (an RPC and the watcher, say). The later one must
// not finish first and then be overwritten by the earlier, older snapshot,
// in either the profile file or machine.yaml.
func TestOverlappingPersistsKeepNewestConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "config.yaml")
	s := New(path, cfg.Defaults(), nil)

	firstSaving := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	prev := saveConfig
	saveConfig = func(c cfg.Config, p string) error {
		if calls.Add(1) == 1 {
			close(firstSaving)
			<-release
		}
		return prev(c, p)
	}
	t.Cleanup(func() { saveConfig = prev })

	edit := func(locus, isq string) {
		if err := s.Mutate(func(c *cfg.Config) { c.LocusMode = locus; c.MistralRS.ISQ = isq }); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	edit("cloud_only", "Q4K")
	wg.Add(1)
	go func() { defer wg.Done(); s.Persist() }()
	<-firstSaving
	edit("open_only", "Q8_0")
	wg.Add(1)
	go func() { defer wg.Done(); s.Persist() }()
	time.Sleep(100 * time.Millisecond) // let an unserialized second Persist finish first
	close(release)
	wg.Wait()

	got, err := cfg.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.LocusMode != "open_only" {
		t.Errorf("profile locus_mode = %q, want the newer open_only", got.LocusMode)
	}
	mc, err := cfg.LoadMachine()
	if err != nil {
		t.Fatal(err)
	}
	if mc.MistralRS.ISQ != "Q8_0" {
		t.Errorf("machine ISQ = %q, want the newer Q8_0", mc.MistralRS.ISQ)
	}
}
