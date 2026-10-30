package builtins

import (
	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/conversation"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitPatchBoundedSafe(t *testing.T) {
	dir := initTestRepo(t)
	marker := filepath.Join(dir, "executed")
	for _, args := range [][]string{{"config", "diff.external", "touch " + marker}, {"config", "diff.evil.textconv", "touch " + marker}, {"config", "core.fsmonitor", "touch " + marker}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatalf("%s %v", b, e)
		}
	}
	os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("*.txt diff=evil\n"), 0600)
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte(strings.Repeat("modified\n", 1000)), 0600)
	t.Setenv("GIT_EXTERNAL_DIFF", "touch "+marker)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "diff.external")
	t.Setenv("GIT_CONFIG_VALUE_0", "touch "+marker)
	call := func(raw string) (*capabilities.Result, error) {
		return GitDiff().Execute(context.Background(), &capabilities.Call{WorkDir: dir, Args: json.RawMessage(raw)})
	}
	r, e := call(`{"max_bytes":100}`)
	if e != nil || !strings.Contains(r.Text, "[patch truncated]") || len(r.Text) > 120 {
		t.Fatalf("%+v %v", r, e)
	}
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatalf("external program executed: %v", e)
	}
	r, e = call(`{"paths":[":(glob)*"]}`)
	if e != nil || r.Text != "" {
		t.Fatalf("pathspec magic interpreted: %+v %v", r, e)
	}
	for _, raw := range []string{`{"max_bytes":0}`, `{"max_bytes":65537}`, `{"paths":["../outside"]}`, `{"paths":["/etc/passwd"]}`, `{"options":["--output=owned"]}`, `{"path":"."} {}`, `{"paths":["x\u0000y"]}`} {
		if _, e := call(raw); e == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
func TestHistoryCapabilityScopeAndInputs(t *testing.T) {
	calls := 0
	svc := capabilities.Services{History: conversation.HistoryReaderFunc(func(_ context.Context, id string, q conversation.HistoryQuery) ([]conversation.HistoryEntry, error) {
		calls++
		if id != "active" {
			t.Fatalf("scope %s", id)
		}
		return []conversation.HistoryEntry{}, nil
	})}
	for _, cap := range []capabilities.Capability{ConversationSearch(), ConversationRetrieve()} {
		good := `{"query":"needle"}`
		if cap.Name() == "conversation_retrieve" {
			good = `{"turn_id":"turn"}`
		}
		if _, e := cap.Execute(context.Background(), &capabilities.Call{Args: json.RawMessage(good), Svc: svc, ConversationID: "active"}); e != nil {
			t.Fatal(e)
		}
		if _, e := cap.Execute(context.Background(), &capabilities.Call{Args: json.RawMessage(good), Svc: svc}); e == nil {
			t.Fatal("missing scope accepted")
		}
	}
	for _, raw := range []string{`{"query":"x","conversation_id":"victim"}`, `{"query":"x","sql":"SELECT * FROM turns"}`, `{"query":""}`, `{"query":"x","limit":51}`, `{"query":"x","after":-1}`, `{"query":"x","limit":0}`} {
		if _, e := ConversationSearch().Execute(context.Background(), &capabilities.Call{Args: json.RawMessage(raw), Svc: svc, ConversationID: "active"}); e == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if calls != 2 {
		t.Fatalf("unexpected persistence calls: %d", calls)
	}
}
