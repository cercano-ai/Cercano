package compatibility

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/theupdateframework/go-tuf/v2/metadata"
)

func artifact(t *testing.T, ops string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "candidate.instructions")
	if e := os.WriteFile(p, []byte(header+ops), 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func retain(t *testing.T, out emission) evidence {
	t.Helper()
	if len(out.Evidence) > 8192 {
		t.Fatal("unbounded evidence")
	}
	var ev evidence
	if e := json.Unmarshal(out.Evidence, &ev); e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"public-synthetic-marker", "public-candidate-write", header} {
		if bytes.Contains(out.Evidence, []byte(secret)) {
			t.Fatal("diagnostic leaked state")
		}
	}
	// Retained in an isolated test root, not a published release evidence store.
	if e := os.WriteFile(filepath.Join(t.TempDir(), "evidence.json"), out.Evidence, 0600); e != nil {
		t.Fatal(e)
	}
	t.Log(string(out.Evidence))
	return ev
}
func TestEvidenceToEphemeralTUFTarget(t *testing.T) {
	if matrixDigest() != "24554ee6e93ac4741c20538f5e0f2ce654e66ab2984f70990128b3e0240055f6" {
		t.Fatal("reviewed matrix changed: review semantics and bump contract before repinning")
	}
	p := artifact(t, "PPP")
	out, e := rehearse(p, reviewedPolicy())
	if e != nil {
		t.Fatal(e)
	}
	ev := retain(t, out)
	if !complete(ev, bind([]byte(header+"PPP"))) || ev.Outcome != "synthetic-only-passed" {
		t.Fatal("wrong evidence")
	}
	signed, e := metadata.Targets().FromBytes(out.Targets)
	if e != nil {
		t.Fatal(e)
	}
	if e = out.Root.VerifyDelegate("targets", signed); e != nil {
		t.Fatal(e)
	}
	tf := signed.Signed.Targets[target]
	if tf.Length != int64(ev.Artifact.Length) || hex.EncodeToString(tf.Hashes["sha256"]) != ev.Artifact.SHA256 {
		t.Fatal("target/evidence mismatch")
	}
	// Exact v1 schema, with no evidence extension or extra client authority.
	var actual any
	if e = json.Unmarshal(*tf.Custom, &actual); e != nil {
		t.Fatal(e)
	}
	expected := `{"version":1,"contract":"fixture-gate-matrix-v1","domains":{"database":{"prior_format":"fixture-db-v1","prior_reads_candidate_writes":true},"config":{"prior_format":"fixture-config-v1","prior_reads_candidate_writes":true},"credentials":{"prior_format":"fixture-credentials-v1","prior_reads_candidate_writes":true}}}`
	var want any
	_ = json.Unmarshal([]byte(expected), &want)
	a, _ := json.Marshal(actual)
	b, _ := json.Marshal(want)
	if !bytes.Equal(a, b) {
		t.Fatal("wrong declaration")
	}
	// Signature authenticates both declaration and artifact binding.
	tf.Length++
	if e = out.Root.VerifyDelegate("targets", signed); e == nil {
		t.Fatal("substituted signed target accepted")
	}
}
func TestRefuseIncompleteOrForeignEvidence(t *testing.T) {
	mutations := map[string]func(*evidence){
		"missing":           func(e *evidence) { e.Cases = e.Cases[:5] },
		"skipped":           func(e *evidence) { e.Cases[0].Status = "skipped" },
		"failed":            func(e *evidence) { e.Cases[0].Status = "failed" },
		"duplicate":         func(e *evidence) { e.Cases[0] = e.Cases[1] },
		"unknown":           func(e *evidence) { e.Cases[0].Case.ID = "unknown" },
		"extra":             func(e *evidence) { e.Cases = append(e.Cases, e.Cases[0]) },
		"wrong-matrix":      func(e *evidence) { e.MatrixSHA256 = "other" },
		"wrong-contract":    func(e *evidence) { e.Policy.Contract = "other" },
		"wrong-tuple":       func(e *evidence) { e.Policy.Credentials = "other" },
		"wrong-case-format": func(e *evidence) { e.Cases[0].Case.Format = "other" },
		"wrong-case-seed":   func(e *evidence) { e.Cases[0].Case.Seed = "other" },
		"wrong-length":      func(e *evidence) { e.Artifact.Length++ },
		"wrong-digest":      func(e *evidence) { e.Artifact.SHA256 = "other" },
		"wrong-version":     func(e *evidence) { e.Version++ },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			out, e := runGate(artifact(t, "PPP"), reviewedPolicy(), faults{afterRun: mutate})
			if e == nil || len(out.Targets) != 0 || out.Root != nil {
				t.Fatal("unsafe emission")
			}
			retain(t, out)
		})
	}
}
func TestOwnedRunnerMustActuallyWriteReadableState(t *testing.T) {
	for _, ops := range []string{"BPP", "PBP", "PPB", "NPP", "PNP", "PPN", "EPP", "PEP", "PPE", "", "PPPP"} {
		t.Run(ops, func(t *testing.T) {
			out, e := rehearse(artifact(t, ops), reviewedPolicy())
			if e == nil || len(out.Targets) != 0 {
				t.Fatal("unsafe emission")
			}
			ev := retain(t, out)
			if len(ev.Cases) != 6 {
				t.Fatal("missing diagnostic cases")
			}
		})
	}
}
func TestRefuseArtifactSubstitution(t *testing.T) {
	for _, replacement := range []string{header + "BPP", header + "PPPP", ""} {
		t.Run(replacement, func(t *testing.T) {
			p := artifact(t, "PPP")
			out, e := runGate(p, reviewedPolicy(), faults{beforeEmit: func() {
				if replacement == "" {
					if e := os.Remove(p); e != nil {
						t.Fatal(e)
					}
				} else if e := os.WriteFile(p, []byte(replacement), 0600); e != nil {
					t.Fatal(e)
				}
			}})
			if e == nil || len(out.Targets) != 0 {
				t.Fatal("substitution emitted")
			}
			retain(t, out)
		})
	}
}
func TestRefusePolicyAndInvalidArtifact(t *testing.T) {
	for _, p := range []policy{{}, {contract, "fixture-db-v1", "fixture-config-v1", "wrong"}, {"other", "fixture-db-v1", "fixture-config-v1", "fixture-credentials-v1"}} {
		out, e := rehearse(artifact(t, "PPP"), p)
		if e == nil || len(out.Targets) != 0 {
			t.Fatal("policy emitted")
		}
		retain(t, out)
	}
	for _, path := range []string{filepath.Join(t.TempDir(), "missing"), t.TempDir(), artifact(t, string(bytes.Repeat([]byte{'P'}, 1024)))} {
		out, e := rehearse(path, reviewedPolicy())
		if e == nil || len(out.Targets) != 0 {
			t.Fatal("invalid artifact emitted")
		}
		retain(t, out)
	}
}
