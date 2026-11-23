package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/failurelog"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/usage"
	"cercano/source/server/internal/worker"
)

func TestPersistenceDiagnosticOriginalAdmission(t *testing.T) {
	for _, operation := range []string{"", "original-operation"} {
		t.Run(operation, func(t *testing.T) {
			s, _, owner, _ := preparedWakeFixture(t)
			path := filepath.Join(t.TempDir(), "failures.jsonl")
			writer, err := failurelog.NewWriter(path)
			if err != nil {
				t.Fatal(err)
			}
			originalWriter := s.failureLog
			s.failureLog = writer
			defer func() { s.failureLog = originalWriter; _ = writer.Close() }()
			ctx := t.Context()
			if operation != "" {
				ctx = usage.WithAttempts(ctx, func(usage.AttemptObservation) bool { return true }, usage.Attribution{OperationID: operation})
			}
			ctx, ticket, err := s.BeginRetainedWork(ctx, owner, broker.PersistenceWork)
			if err != nil {
				t.Fatal(err)
			}
			other, err := s.Retain(t.Context(), "other-epoch")
			if err != nil {
				t.Fatal(err)
			}
			if other.Epoch == owner.Epoch {
				t.Fatal("fixture epochs must differ")
			}
			a := &preparedWorkerAuthority{server: s, ctx: ctx, id: owner.Conversation, ticket: ticket}
			var normal bytes.Buffer
			old := log.Writer()
			log.SetOutput(&normal)
			defer log.SetOutput(old)
			secret := errors.New("SECRET credential /private/target prompt receipt")
			// A different capability/epoch is current at observation time. Holding mu verifies
			// that neither callback nor Close tries to resolve current lifecycle state.
			l := s.coexistence
			l.mu.Lock()
			cap := l.owners[owner.Conversation]
			l.owners[owner.Conversation] = l.owners[other.Conversation]
			persistsvc.ReportWriteFailure(ctx, secret)
			a.Close(&worker.CallbackCleanupError{Err: secret})
			l.owners[owner.Conversation] = cap
			l.mu.Unlock()
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			origins := map[string]bool{}
			for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
				var event map[string]any
				if err := json.Unmarshal(line, &event); err != nil {
					t.Fatal(err)
				}
				if event["event"] != "lifecycle.persistence_retained" {
					continue
				}
				origins[event["origin"].(string)] = true
				if event["conversation_id"] != owner.Conversation || event["host_id"] != owner.Host || event["ownership_epoch"] != owner.Epoch {
					t.Fatalf("wrong original identity: %s", line)
				}
				op, present := event["operation_id"]
				if (operation == "" && present) || (operation != "" && op != operation) {
					t.Fatalf("wrong operation: %s", line)
				}
				allowed := map[string]bool{"event": true, "ts": true, "scope": true, "origin": true, "work_kind": true, "conversation_id": true, "host_id": true, "ownership_epoch": true, "operation_id": true}
				for key := range event {
					if !allowed[key] {
						t.Fatalf("forbidden key %q: %s", key, line)
					}
				}
			}
			for _, origin := range []string{"retained_write", "worker_callback_cleanup", "worker_close"} {
				if !origins[origin] {
					t.Fatalf("missing %s: %s", origin, data)
				}
			}
			for _, bad := range []string{"SECRET", "credential", "/private/target", "prompt", "receipt"} {
				if strings.Contains(string(data)+normal.String(), bad) {
					t.Fatalf("unsafe diagnostics: %s %s", data, normal.String())
				}
			}
		})
	}
}

// The failing configurer must not leak a dependency error through normal logs.
type diagnosticConfigurer struct{ runner.TurnRunner }

func (diagnosticConfigurer) SetAccountingReceiver(worker.AccountingReceiver) error {
	return errors.New("SECRET credential target /private/path")
}

type configDiagnosticReceiver struct {
	worker.AccountingReceiver
	reason string
}

func (r *configDiagnosticReceiver) MarkAccountingIncomplete(reason string) { r.reason = reason }
func TestAccountingConfigurationDiagnosticPrivacy(t *testing.T) {
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(old)
	receiver := &configDiagnosticReceiver{}
	s := &Server{workerRunner: diagnosticConfigurer{}, accountingReceiver: receiver}
	s.configureWorkerAccounting()
	if !strings.Contains(output.String(), "reason=config_failed") {
		t.Fatalf("missing fixed reason: %s", output.String())
	}
	if receiver.reason != "worker accounting configuration failed" {
		t.Fatalf("classifier changed: %q", receiver.reason)
	}
	for _, bad := range []string{"SECRET", "credential", "target", "/private/path", "conversation_id", "ownership_epoch"} {
		if strings.Contains(output.String(), bad) {
			t.Fatalf("unsafe configuration log: %s", output.String())
		}
	}
}
