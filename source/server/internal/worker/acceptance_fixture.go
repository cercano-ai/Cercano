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

// Test-only evidence emitted by the actual worker handling StartTurn, not by
// its host/provider proxy. No credentials, prompt text, or native state.
func observeAcceptanceTurn(start *proto.StartTurn) {
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
	event := map[string]any{"event": "worker-turn", "pid": os.Getpid(), "parent_pid": os.Getppid(), "executable": exe, "sha256": hex.EncodeToString(sum[:]), "manifest_sha256": hex.EncodeToString(manifestSum[:]), "build_info": info.String(), "conversation": start.ConversationId, "turn_generation": start.Gen, "unix_nano": time.Now().UnixNano()}
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
