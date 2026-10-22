# Offline synthetic compatibility evidence gate

Test-only rehearsal of the approved RELEASE-PIPELINE gate; **not real Cercano
compatibility evidence**, a production publisher, or activation authorization.
Everything is `_test.go`. No command, workflow, network fetch/server, live agent,
user state, migration, production key, or publication is involved. The nested
module reuses the pinned dependencies and ephemeral Ed25519/go-tuf signer pattern
from `../../tuf-proof`; the parent fixture module remains dependency-free.

## Fixed reviewed synthetic matrix

Contract `fixture-gate-matrix-v1` identifies the six rows below and the owned
interpreter/prior-reader semantics in `gate_test.go`. These are synthetic IDs,
not assertions about production format versions. Matrix changes, reader changes,
or interpreter changes require review and a new contract identity.

| Domain | Prior format | Required seeds/cases |
|---|---|---|
| database | fixture-db-v1 | database-empty, database-populated |
| config | fixture-config-v1 | config-empty, config-populated |
| credentials | fixture-credentials-v1 | credentials-empty, credentials-populated |

Each row allocates fresh in-memory prior JSON state, reads it with the prior
reader, executes the exact candidate instruction for that domain, serializes the
candidate write, then uses the strict prior reader to verify version, preservation
and the expected newly written value. All three toy domains intentionally share
one tiny JSON representation. They do **not** simulate SQLite, actual config,
keychains or encryption. Credential values are public synthetic markers only.

The candidate artifact is a bounded interpreted program, not an executable or
an arbitrary all-pass results file: `SYNTHETIC-WRITER-v1\n` followed by exactly
three instructions (database/config/credentials). `P` performs the compatible
write; `B` changes the version; `N` does no write; other instructions fail the
runner. The gate itself runs all six cases. No external runner, subprocess,
results-file input or caller-selected matrix is accepted. Private fault hooks
exist only for negative tests after owned execution.

## Exact evidence chain

1. Read a regular, at-most-128-byte artifact in a test-owned temporary root;
   bind its length and SHA-256 before execution.
2. Execute all fixed rows on those exact bytes. Require exactly one passing
   result per row, exact matrix digest/contract/format tuple and artifact binding.
3. Re-read/hash the candidate before declaration construction; refuse substitution.
4. Construct the acquisition v1 custom declaration with the fixed contract and
   all three prior formats, each with explicit backward-readability `true`.
5. Place that declaration in the **same TUF target entry** as candidate length
   and SHA-256. Sign using a newly generated in-memory Ed25519 targets key.
6. Recheck the on-disk artifact immediately before returning the signed bytes.
   The success test verifies the targets signature with ephemeral root delegation,
   checks target/evidence binding and exact acquisition declaration schema, and
   proves target mutation invalidates the signature. No full TUF repository is
   published; root is in-memory verification material, not a production bootstrap.

For the passing `PPP` fixture:

- Candidate length: **23** bytes.
- Candidate SHA-256: `738863885baca893c870bd7132b9c1f4385ee758cd4afc7e4857b2559f6367e3`.
- Matrix SHA-256: `24554ee6e93ac4741c20538f5e0f2ce654e66ab2984f70990128b3e0240055f6`
  (ordered JSON encoding of the six fixed rows, pinned by a regression test).

Evidence includes the matrix identity, policy, candidate length/hash, fixed case
identities/statuses and a gate-generated outcome code. Tests enforce an 8 KiB
bound, log the evidence for test-log retention and write it in a temporary root
(cleaned by Go). No state contents, raw error text, paths or private keys are
retained. Refusals return diagnostic evidence but **no signed target or root**.
A signer is not a separate evidence authority: evidence is not separately signed
or consumed by clients. The contract ID identifies the reviewed matrix in the
existing declaration schema; the diagnostic matrix hash is not a new custom field.

Negative tests cover missing/skipped/failed/duplicate/unknown/extra results,
wrong matrix/contract/format/seed/version, wrong artifact length/hash, candidate
substitution/removal, invalid/oversized inputs, runner failure, incompatible writes
and no-op writers. This is a trusted test process with private owned roots, not a
sandbox for hostile native executables or protection against a concurrent hostile
filesystem writer. Final rechecking is not filesystem/signing atomicity.

## Offline verification

From this directory:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./... -count=1 -timeout=60s
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -race ./... -count=1 -timeout=60s
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go vet ./...
```

Cached pinned modules are required; no module download is permitted. `go test -v`
retains the diagnostic JSON in captured test output. No production evidence store
or retention policy is introduced.

## Still required before any real release declaration

Inventory **all** actual database schemas/stores and migration/write paths,
config representations/defaults/unknown-field handling, and credential stores,
encodings/encryption/keychain behavior on supported platforms. Establish reviewed
production format IDs, supported prior releases/readers and complete scenario
coverage (including populated/edge/error state, candidate initialization and
writes, recovery and prior-reader behavior). Run those prior-release readers
against state actually written by the exact candidate package in isolated native
roots with bounded owned processes. Cover the complete agent/client artifact and
platform-specific storage behavior; a toy interpreter cannot establish any of it.

Production artifact freezing, complete-matrix review/enforcement, durable evidence
retention, real signer integration and failure-safe publication remain unimplemented.
The existing acquisition parser tests separately cover client fail-closed parsing;
this rehearsal verifies the emitted schema and signed binding, not a new client
integration or normal-startup gate. No release is marked compatible by this work.
