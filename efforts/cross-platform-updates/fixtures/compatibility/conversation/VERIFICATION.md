# Bounded conversation compatibility experiment — FAIL

TEST ONLY, executed 2026-10-22 on darwin/arm64 with Go 1.26.3. This is actual
conversation-package evidence, not the parent directory's synthetic gate and
not full compatibility or release/activation authorization.

## Exact inputs and approval

- User-approved latest published stable baseline: **v0.25.0**, published
  **2026-10-22T19:16:38Z**, https://github.com/cercano-ai/Cercano/releases/tag/v0.25.0.
- User-supplied tag API direct commit: **58befd2801dd6f3d53438e8c8bfc74d937a9e811**.
  The existing local Git commit object was used; no moving tag/checkout or
  additional network retrieval was needed. Release metadata is user-provided,
  not independently authenticated by this experiment.
- Candidate: **25969d70e8e834fa982978285577ce11e3088277** on
  `feat/cross-platform-updates` (initially clean, per task).
- Runner archives `source/server` from each exact commit into a private temporary
  directory. It does not compile the mutable working tree or run the normal agent.
  The same harness compiles against both APIs without version-specific adapters.
  Each copy lives at `source/server/cmd/compat-experiment/main.go`, inside its
  own module's internal import boundary. No production source is changed.

## Reproduce

From the repository root, with those Git objects, Python 3.9+, local Go 1.26,
 and already cached module dependencies:

```sh
COMPAT_GOMODCACHE=/Users/bryancostanich/go/pkg/mod \
  python3 efforts/cross-platform-updates/fixtures/compatibility/conversation/run.py \
  > efforts/cross-platform-updates/fixtures/compatibility/conversation/results.txt 2>&1
```

That is the executed command (exit **1**, expected evidence of incompatibility).
Set `COMPAT_GOMODCACHE` explicitly to an available cache on other machines.
The runner records every subprocess command and exit, toolchain and harness
binary hashes in `results.txt`. Hashes are per-run, not release artifact hashes;
Go build paths may change them. Inside each archived module it executes:

```sh
go build -mod=readonly -o <temporary-version-directory>/harness ./cmd/compat-experiment
go test -mod=readonly ./internal/conversation -count=1 -timeout=60s
```

Environment is explicitly constructed: temporary HOME/config/cache/TMPDIR,
`GOENV=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off`, temporary
GOCACHE, explicit existing GOMODCACHE, disabled global/system Git configuration.
No live HOME/config/keychain or normal startup is used. No network services,
signing, production format IDs, activation, push or merge. Databases and extracted
sources are cleaned on exit. Subprocess process groups are killed on timeout:
20 seconds per harness, 240 per build, 180 per package test (Go test itself 60),
120 for other commands. This is trusted-code isolation, not an OS sandbox.

## Actual assertions and sequence

For both prior-created empty and populated SQLite databases:
prior seed → prior seed-check → candidate seed-check → candidate write →
candidate verify → prior Open-only diagnostic → prior verify.
Failures remain failures and do not suppress later candidate writes/readback.

Populated seed has `untouched` and `updated` conversations. Assertions compare
exact title, recap, project, model and full turn fields (IDs, role, Unicode text,
blocks JSON, token/latency fields, timestamps). Autonomy assertions require one
run with exact run ID, brief JSON, decisions JSON and state. ContextUsage checks
existence and all struct fields. Empty seed requires zero conversations.
Candidate creates `candidate` with a turn, autonomy run and usage; populated
writes additionally append `updated-new` while preserving the old turn, and
replace `updated` usage (11 → 22). Both readers must see these values; the
untouched seed must remain semantically intact. Timestamps are normalized to
UTC for instant comparison because SQLite stores epoch seconds, not Go Location.
Autonomy coverage is intentionally limited to the stated fields, not every field.

## Observed results

Full raw output: `results.txt`.

- Both harness builds and both focused conversation package test suites **PASS**
  (prior 0.402s, candidate 0.335s in final run).
- Empty scenario: candidate and prior semantic assertions **PASS** after writes.
- Populated scenario, candidate seed-check **FAIL**: both seed context usages
  return `exists=false`, zero values instead of the prior values (`TokensUsed=11`,
  `MessageTokens=11`, `EstimatedRequest=21`, timestamp epoch 211).
- Candidate post-write verification **FAIL** for untouched usage (still missing).
  Candidate sees its own updated usage correctly (22, 22, 32, epoch 222).
- Prior post-write verification **FAIL**: `updated` usage still returns **11**
  tokens, **11** message tokens, estimated request **21**, epoch **211** instead
  of **22**, **22**, **32**, epoch **222**. All other recorded prior assertions,
  including seed turns/autonomy and candidate-created turn/autonomy, pass.
- **Prior Open is mutating, NOT read-only.** For both databases the logical dump
  and main-file hash changed. It inserted the candidate-created usage row into
  `conversation_request_usage`; no dump lines were removed. The prior verifier
  also calls production Open. Only the diagnostic observer uses SQLite
  `mode=ro&immutable=1` after all harnesses close, refusing nonempty WAL files.

This matches the pinned implementations: prior SaveContextUsage writes `turn`
usage to `conversation_request_usage`, prior GetContextUsage prefers that table,
and prior Open copies context turn rows with INSERT OR IGNORE. Candidate uses
`conversation_context_usage` only. The prior seed row therefore masks the newer
candidate write. Successful package unit tests do not establish cross-version
compatibility.

During fixture development a field-name compile correction was needed
(`ContextWindowKnown`); initial comparisons also exposed Go time.Location
representation differences, now compared by UTC instant. The initial Python
read-only SQLite observer could not open the WAL database; the closed-database
immutable observer with an explicit nonempty-WAL refusal resolved that diagnostic
obstacle. These were fixture setup issues, not production migration fixes. Final
semantic assertions remain strict and the compatibility failures are retained.

## Blockers and limits

The bounded experiment is complete with a negative result. Candidate preservation
of prior request usage and prior visibility of candidate updates are blockers;
any production remediation requires separate authorization and a rerun, not
weaker assertions or altered migration behavior in this fixture.

Only this source pair, one platform, two tiny prior-created databases and selected
conversation operations are covered. This does not cover all schemas, older
migration histories, compaction/retention/error/crash/concurrent writes, full
autonomy evolution, config, credentials, native Windows/Linux, packaged release
binaries or full startup. No compatibility declaration, production format ID,
release gate integration or full compatibility claim follows from this evidence.
