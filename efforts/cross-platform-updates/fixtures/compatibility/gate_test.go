// Test-only release-gate rehearsal. No production publisher or format support.
package compatibility

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/theupdateframework/go-tuf/v2/metadata"
)

const contract = "fixture-gate-matrix-v1"
const header = "SYNTHETIC-WRITER-v1\n"
const target = "synthetic/candidate.instructions"
const maxArtifact = 128

// Reviewed matrix: each row means prior-format seed -> candidate write ->
// strict prior reader checks preservation AND the expected newly written value.
// Changes to rows, interpreter or reader semantics require review and a new ID.
type scenario struct{ ID, Domain, Format, Seed string }

func matrix() []scenario {
	return []scenario{
		{"database-empty", "database", "fixture-db-v1", "empty"},
		{"database-populated", "database", "fixture-db-v1", "populated"},
		{"config-empty", "config", "fixture-config-v1", "empty"},
		{"config-populated", "config", "fixture-config-v1", "populated"},
		{"credentials-empty", "credentials", "fixture-credentials-v1", "empty"},
		{"credentials-populated", "credentials", "fixture-credentials-v1", "populated"},
	}
}

type policy struct{ Contract, Database, Config, Credentials string }

func reviewedPolicy() policy {
	return policy{contract, "fixture-db-v1", "fixture-config-v1", "fixture-credentials-v1"}
}
func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func matrixDigest() string   { b, _ := json.Marshal(matrix()); return digest(b) }

type binding struct {
	Length int
	SHA256 string
}

func bind(b []byte) binding { return binding{len(b), digest(b)} }

type result struct {
	Case   scenario
	Status string
}
type evidence struct {
	Version      int
	Policy       policy
	MatrixSHA256 string
	Artifact     binding
	Cases        []result
	Outcome      string
}
type emission struct {
	Evidence []byte
	Targets  []byte
	// Public verification material only. Private signer never escapes the gate.
	Root *metadata.Metadata[metadata.RootType]
}

func readArtifact(path string) ([]byte, error) {
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Size() > maxArtifact {
		return nil, errors.New("artifact-invalid")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, errors.New("artifact-unavailable")
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Size() > maxArtifact {
		return nil, errors.New("artifact-invalid")
	}
	b, e := io.ReadAll(io.LimitReader(f, maxArtifact+1))
	if e != nil || len(b) > maxArtifact {
		return nil, errors.New("artifact-invalid")
	}
	return b, nil
}

// These are deliberately toy, in-memory formats, not SQLite, real config or a
// credential store. Every row owns fresh state. All values are public constants.
// Prior readers reject different versions and check all fields, not just parse.
type priorState struct {
	Version            int
	Preserved, Written string
}

func priorRead(raw []byte) (priorState, error) {
	var s priorState
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(&s); e != nil {
		return s, e
	}
	if s.Version != 1 {
		return s, errors.New("prior-format")
	}
	if d.Decode(new(any)) != io.EOF {
		return s, errors.New("trailing")
	}
	return s, nil
}
func execute(candidate []byte, c scenario) string {
	if len(candidate) != len(header)+3 || !bytes.HasPrefix(candidate, []byte(header)) {
		return "runner-failed"
	}
	index := map[string]int{"database": 0, "config": 1, "credentials": 2}[c.Domain]
	preserved := ""
	if c.Seed == "populated" {
		preserved = "public-synthetic-marker"
	}
	seed, _ := json.Marshal(priorState{Version: 1, Preserved: preserved})
	state, e := priorRead(seed)
	if e != nil {
		return "runner-failed"
	}
	// Interpret the EXACT hashed artifact's bounded three-instruction program.
	// P writes backward-readable state; B breaks prior reading; E fails the runner;
	// N is a no-op, which must fail the expected-write assertion.
	switch candidate[len(header)+index] {
	case 'P':
		state.Written = "public-candidate-write"
	case 'B':
		state.Version = 2
		state.Written = "public-candidate-write"
	case 'N':
	default:
		return "runner-failed"
	}
	written, _ := json.Marshal(state)
	got, e := priorRead(written)
	if e != nil || got.Preserved != preserved || got.Written != "public-candidate-write" {
		return "failed"
	}
	return "passed"
}
func complete(e evidence, original binding) bool {
	if e.Version != 1 || e.Policy != reviewedPolicy() || e.MatrixSHA256 != matrixDigest() || e.Artifact != original || len(e.Cases) != len(matrix()) {
		return false
	}
	seen := map[string]bool{}
	expected := map[string]scenario{}
	for _, c := range matrix() {
		expected[c.ID] = c
	}
	for _, r := range e.Cases {
		c, ok := expected[r.Case.ID]
		if !ok || seen[c.ID] || c != r.Case || r.Status != "passed" {
			return false
		}
		seen[c.ID] = true
	}
	return true
}

// No evidence-file input and no configurable external runner. Hooks are private
// test fault injection AFTER owned execution, never a supported gate interface.
type faults struct {
	afterRun   func(*evidence)
	beforeEmit func()
}

func rehearse(path string, p policy) (emission, error) { return runGate(path, p, faults{}) }
func runGate(path string, p policy, f faults) (out emission, err error) {
	ev := evidence{Version: 1, Policy: reviewedPolicy(), MatrixSHA256: matrixDigest(), Outcome: "refused"}
	// Only fixed case identities/status codes enter retained evidence. Never raw
	// runner errors, artifact contents, state, credentials or filesystem paths.
	defer func() {
		if err != nil {
			ev.Outcome = err.Error() // Gate-generated fixed codes only.
		}
		out.Evidence, _ = json.Marshal(ev)
	}()
	if p != reviewedPolicy() {
		return out, errors.New("policy-refused")
	}
	candidate, e := readArtifact(path)
	if e != nil {
		return out, e
	}
	original := bind(candidate)
	ev.Artifact = original
	for _, c := range matrix() {
		ev.Cases = append(ev.Cases, result{c, execute(candidate, c)})
	}
	checked := ev
	checked.Cases = append([]result(nil), ev.Cases...)
	if f.afterRun != nil {
		f.afterRun(&checked)
	}
	if !complete(ev, original) || !complete(checked, original) {
		return out, errors.New("matrix-refused")
	}
	if f.beforeEmit != nil {
		f.beforeEmit()
	}
	again, e := readArtifact(path)
	if e != nil || bind(again) != original {
		return out, errors.New("artifact-changed")
	}
	// Build the schema already consumed by acquisition. Matrix identity is the
	// fixed contract ID; target length/hash bind that declaration to the artifact.
	domains := map[string]any{}
	for _, c := range matrix() {
		domains[c.Domain] = map[string]any{"prior_format": c.Format, "prior_reads_candidate_writes": true}
	}
	custom, _ := json.Marshal(map[string]any{"version": 1, "contract": contract, "domains": domains})
	tf, e := metadata.TargetFile().FromBytes(target, again, "sha256")
	if e != nil {
		return out, errors.New("target-failed")
	}
	raw := json.RawMessage(custom)
	tf.Custom = &raw
	targets := metadata.Targets(time.Now().UTC().Add(time.Hour))
	targets.Signed.Targets[target] = tf
	// Same ephemeral Ed25519/go-tuf signing pattern as tuf-proof/proof_test.go.
	_, private, e := ed25519.GenerateKey(nil)
	if e != nil {
		return out, errors.New("signer-failed")
	}
	signer, e := signature.LoadSigner(private, crypto.Hash(0))
	if e != nil {
		return out, errors.New("signer-failed")
	}
	key, e := metadata.KeyFromPublicKey(private.Public())
	if e != nil {
		return out, errors.New("signer-failed")
	}
	root := metadata.Root(time.Now().UTC().Add(time.Hour))
	if e = root.Signed.AddKey(key, "targets"); e != nil {
		return out, errors.New("signer-failed")
	}
	if _, e = targets.Sign(signer); e != nil {
		return out, errors.New("signer-failed")
	}
	signed, e := targets.ToBytes(false)
	if e != nil {
		return out, errors.New("signer-failed")
	}
	// Final recheck immediately before returning signed output (nothing published).
	again, e = readArtifact(path)
	if e != nil || bind(again) != original {
		return out, errors.New("artifact-changed")
	}
	ev.Outcome = "synthetic-only-passed"
	out.Targets = signed
	out.Root = root
	return out, nil
}
