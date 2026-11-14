package server

import (
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/failurelog"
)

// Keep the probe destination outside the fixture's TempDir so Unix unlink
// semantics cannot hide a still-open writer after fixture cleanup.
func TestPreparedAdminFixtureClosesFailureLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failures")
	w, err := failurelog.NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() }) // Also contain the pre-fix leak.
	size := func() int64 {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return info.Size()
	}
	t.Run("fixture", func(t *testing.T) {
		s, _, _, _ := preparedWakeFixture(t)
		if s.failureLog == nil {
			t.Fatal("fixture failure logger was not opened")
		}
		if err := s.failureLog.Close(); err != nil {
			t.Fatal(err)
		}
		s.failureLog = w
		s.toolSvc.SetFailureLog(w)
		w.Log("cleanup.probe", nil)
		if size() == 0 {
			t.Fatal("probe logger did not write before cleanup")
		}
	})
	before := size()
	w.Log("cleanup.after", nil)
	if after := size(); after != before {
		t.Fatalf("fixture cleanup left failure logger writable: size %d -> %d", before, after)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("failure log removal after cleanup: %v", err)
	}
}
