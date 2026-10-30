//go:build developmentfixture && cercano_development_acceptance

package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"cercano/source/server/pkg/proto"
)

// ObserveAcceptanceStartup is called only at child entry, never by StartTurn.
func ObserveAcceptanceStartup() { observeAcceptanceEvent("worker-startup", nil) }

func observeAcceptanceTurn(start *proto.StartTurn) { observeAcceptanceEvent("worker-turn", start) }

// Test-only child evidence. No credentials, prompt text, or native state.
func observeAcceptanceEvent(kind string, start *proto.StartTurn) {
	root := os.Getenv("CERCANO_FIXTURE_BARRIER")
	if root == "" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		panic(err)
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(binary)
	manifest, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "manifest.json"))
	if err != nil {
		panic(err)
	}
	manifestSum := sha256.Sum256(manifest)
	info, ok := debug.ReadBuildInfo()
	if !ok {
		panic("missing worker build info")
	}
	event := map[string]any{"event": kind, "pid": os.Getpid(), "parent_pid": os.Getppid(), "executable": exe, "sha256": hex.EncodeToString(sum[:]), "manifest_sha256": hex.EncodeToString(manifestSum[:]), "build_info": info.String(), "unix_nano": time.Now().UnixNano()}
	if start != nil {
		event["conversation"] = start.ConversationId
		event["turn_generation"] = start.Gen
	}
	b, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	f, err := os.OpenFile(filepath.Join(root, "worker.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if _, err = f.Write(append(b, '\n')); err != nil {
		panic(err)
	}
}
