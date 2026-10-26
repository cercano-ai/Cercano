package llm

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"time"

	"cercano/source/server/pkg/paths"
)

// RecordAnomaly appends a structured turn/stream anomaly record — a resume-replay
// or fabricated-turn fingerprint — to the anomaly log, so occurrences across every
// conversation are captured with their content and conversation id: a reviewable
// trail instead of ad-hoc, per-thread incident hunting. The log path is
// stream-anomalies.jsonl in the active profile, overridable via CERCANO_ANOMALY_LOG
// (tests, ops). Best-effort; every failure is swallowed so it never affects the
// caller or the stream.
func RecordAnomaly(conversationID, reason, content string) {
	path := os.Getenv("CERCANO_ANOMALY_LOG")
	if path == "" {
		// A unit test with no explicit override must never write to the real
		// user anomaly log (test.v is registered only in test binaries).
		if flag.Lookup("test.v") != nil {
			return
		}
		path = AnomalyLogPath()
		if path == "" {
			return
		}
	}
	rec := map[string]any{
		"ts":           time.Now().Unix(),
		"conversation": conversationID,
		"reason":       reason,
		"bytes":        len(content),
		"content":      content,
	}
	blob, err := json.Marshal(rec)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer fh.Close()
	_, _ = fh.Write(append(blob, '\n'))
}

// AnomalyLogPath returns the anomaly log location: $CERCANO_ANOMALY_LOG if
// set, otherwise stream-anomalies.jsonl in the active profile's directory.
// Empty when the home directory is unknown.
func AnomalyLogPath() string {
	if p := os.Getenv("CERCANO_ANOMALY_LOG"); p != "" {
		return p
	}
	if paths.CheckHome() != nil {
		return ""
	}
	return filepath.Join(paths.ActiveProfileDir(), "stream-anomalies.jsonl")
}
