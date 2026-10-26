// Package hostregistry records which agent host serves a profile. The running
// agent writes <profile dir>/agent.json once it listens and removes it when it
// exits; clients read it to find the agent's address.
package hostregistry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileName is the registry record's name inside a profile directory.
const FileName = "agent.json"

// Record describes one running agent host.
type Record struct {
	Profile    string    `json:"profile"`
	PID        int       `json:"pid"`
	Addr       string    `json:"addr"`
	InstanceID string    `json:"instance_id"`
	Version    string    `json:"version"`
	StartedAt  time.Time `json:"started_at"`
}

// Path returns the registry record's path for a profile directory.
func Path(profileDir string) string { return filepath.Join(profileDir, FileName) }

// Write stores r as the profile's record, mode 0600. It writes a temporary
// file and renames it into place, so a reader never sees a partial record.
func Write(profileDir string, r Record) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode agent record: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(profileDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", profileDir, err)
	}
	tmp, err := os.CreateTemp(profileDir, ".agent-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create agent record: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	// CreateTemp already uses 0600; Chmod keeps that true regardless.
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set agent record mode: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write agent record: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync agent record: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close agent record: %w", err)
	}
	if err := os.Rename(tmpName, Path(profileDir)); err != nil {
		return fmt.Errorf("install agent record: %w", err)
	}
	tmpName = ""
	return nil
}

// Read returns the profile's record. A missing record yields an error that
// matches os.ErrNotExist.
func Read(profileDir string) (Record, error) {
	data, err := os.ReadFile(Path(profileDir))
	if err != nil {
		return Record{}, err
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return Record{}, fmt.Errorf("parse %s: %w", Path(profileDir), err)
	}
	return r, nil
}

// Remove deletes the profile's record only while it still names instanceID,
// so an exiting agent never removes a successor's record. A missing record,
// or one that cannot be read or belongs to another instance, is left as is
// and is not an error.
func Remove(profileDir, instanceID string) error {
	r, err := Read(profileDir)
	if err != nil || r.InstanceID != instanceID {
		return nil
	}
	if err := os.Remove(Path(profileDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove agent record: %w", err)
	}
	return nil
}
