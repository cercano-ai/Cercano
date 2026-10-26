package hostregistry

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func sampleRecord(id string) Record {
	return Record{
		Profile:    "work",
		PID:        4242,
		Addr:       "127.0.0.1:53111",
		InstanceID: id,
		Version:    "1.2.3",
		StartedAt:  time.Date(2026, 10, 22, 9, 30, 0, 0, time.UTC),
	}
}

func TestRegistryWriteReadRemove(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profiles", "work")
	want := sampleRecord("abc123")
	if err := Write(dir, want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, want.StartedAt)
	}
	got.StartedAt = want.StartedAt
	if got != want {
		t.Errorf("Read = %+v, want %+v", got, want)
	}

	// Another instance's ID must leave this instance's record alone.
	if err := Remove(dir, "someone-else"); err != nil {
		t.Fatalf("Remove(other): %v", err)
	}
	if _, err := Read(dir); err != nil {
		t.Fatalf("record gone after Remove with another instance ID: %v", err)
	}

	if err := Remove(dir, want.InstanceID); err != nil {
		t.Fatalf("Remove(own): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("agent.json still present after Remove: %v", err)
	}
	if _, err := Read(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Read after Remove: err = %v, want ErrNotExist", err)
	}
	// Removing an absent record is not an error.
	if err := Remove(dir, want.InstanceID); err != nil {
		t.Fatalf("Remove(absent): %v", err)
	}
}

func TestRegistryWriteReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, sampleRecord("first")); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, sampleRecord("second")); err != nil {
		t.Fatal(err)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.InstanceID != "second" {
		t.Fatalf("InstanceID = %q, want second", got.InstanceID)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != FileName {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory holds %v, want only %s (no temp files left behind)", names, FileName)
	}
}

func TestRegistryFileIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions only")
	}
	dir := filepath.Join(t.TempDir(), "p")
	if err := Write(dir, sampleRecord("x")); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("agent.json mode = %v, want 0600", fi.Mode().Perm())
	}
}

func TestRegistryReadRejectsCorruptRecord(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil {
		t.Fatal("Read of a corrupt record returned no error")
	}
	// A corrupt record cannot be proven to belong to anyone; Remove keeps it.
	if err := Remove(dir, "x"); err != nil {
		t.Fatalf("Remove(corrupt): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Fatalf("corrupt record removed: %v", err)
	}
}
