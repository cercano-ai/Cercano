package server

import (
	"cercano/source/server/internal/watchdog"
	"errors"
	"sync/atomic"
	"testing"
)

type failingShutdownAudit struct {
	watchdog.AuditLogger
	calls atomic.Int32
	err   error
}

func (a *failingShutdownAudit) Close() error { a.calls.Add(1); return a.err }

func TestServerShutdownRetainsCloseFailure(t *testing.T) {
	sentinel := errors.New("audit close sentinel")
	audit := &failingShutdownAudit{err: sentinel}
	s := &Server{watchdogAudits: []watchdog.AuditLogger{audit}}
	for i := 0; i < 3; i++ {
		if err := s.Shutdown(); !errors.Is(err, sentinel) {
			t.Fatalf("close error lost: %v", err)
		}
	}
	if audit.calls.Load() != 1 {
		t.Fatal("retried close", audit.calls.Load())
	}
}
