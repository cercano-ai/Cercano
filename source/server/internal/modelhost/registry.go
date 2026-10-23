package modelhost

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const recordFile = "modelhost.json"

// Record describes a running model host so other processes can find it.
type Record struct {
	PID        int       `json:"pid"`
	Addr       string    `json:"addr"`
	Token      string    `json:"token"`
	APIVersion int       `json:"api_version"`
	Version    string    `json:"version"`
	StartedAt  time.Time `json:"started_at"`
}

// RunDir is the directory holding the host record, launch lock and log.
func RunDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating home directory: %w", err)
	}
	if home == "" {
		return "", errors.New("home directory is unknown")
	}
	return filepath.Join(home, ".cercano", "run", "modelhost"), nil
}

// ensureRunDir creates dir (0700) and verifies it is safe to trust: a real
// directory owned by the current user that nobody else can access.
func ensureRunDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return checkRunDir(dir)
}

// WriteRecord atomically writes the record with mode 0600.
func WriteRecord(dir string, r Record) error {
	if err := ensureRunDir(dir); err != nil {
		return err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, recordFile+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, recordFile)); err != nil {
		cleanup()
		return err
	}
	return nil
}

// ReadRecord reads the host record; the error wraps os.ErrNotExist when absent.
func ReadRecord(dir string) (Record, error) {
	var r Record
	if err := ensureRunDir(dir); err != nil {
		return r, err
	}
	path := filepath.Join(dir, recordFile)
	if err := checkRecordFile(path); err != nil {
		return r, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return Record{}, err
	}
	return r, nil
}

// RemoveRecord removes the record only if it still names pid.
func RemoveRecord(dir string, pid int) error {
	r, err := ReadRecord(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if r.PID != pid {
		return nil
	}
	if err := os.Remove(filepath.Join(dir, recordFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
