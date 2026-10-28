# Bounded Load/reset publication follow-up — 2026-10-28

Scope: pkg/config admission and internal/setupreset publication/tests only. No internal/server or hostsvc/config edits in this slice.

## Reproductions and fixes

Before fixes, the focused Load test demonstrated that multi-document, comment-only and empty existing files loaded successfully but subsequent SaveSnapshot failed. The reset stale fixture overwrote the competing edit and applied/deleted credentials; replacing the target with a directory failed publication only after live apply and two deletions. These red tests were run, not inferred. An initial diagnostic assertion accidentally included its secret marker in the test-directory name; corrected to numeric subtest names.

Load now runs configDocument against the same bytes decoded and captured as its baseline, before environment/default activation. Invalid authority is not replaced. Errors name the path and remediation without including YAML values. Existing full Save struct serialization (including legacy unknown-field behavior) is unchanged.

Prepare captures a coordinator snapshot. Explicit ResetSetup publication compares the entire expected revision/absence under the existing lock and preserves reset's unrelated-field semantics. Reset publishes before any credential deletion or live callback. The callback and Result.Config receive the returned snapshot. No stateful writer, automatic retry/repair or new authority was introduced. Prepare may create the existing coordinator's lock file and parent directory, but does not publish YAML.

Tests use temp directories and memory credential stores only. Coverage includes stale reset, nonregular targets, injected publication failure with no effects, malformed/multi-document/comment-only authorities, genuinely absent creation, competing creation, successful reset and callback snapshot propagation. Credential-disabled policy was not weakened.

## Verification actually run

All Go commands used CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off, from source/server:

- Before fix: `go test ./pkg/config ./internal/setupreset -run 'TestLoadAdmissionRejects|TestResetPublicationRejects' -count=1` — failed with the reproductions above.
- `go test ./pkg/config ./internal/setupreset -count=1` — passed after correction of field-specific validation diagnostics.
- `go test -race ./pkg/config ./internal/setupreset -count=1` — passed.
- `go test ./pkg/config ./internal/setupreset ./internal/setupresetcmd ./internal/server ./internal/localruntime/llamaserver ./cmd/cercano -run '^$'` — compilation passed; no caller tests or CLI startup executed.

No native credentials, live data, real reset, installers or runtime activation were exercised.

## Limits / remaining caller work

Publication, credential deletion, live application and wizard-state publication are NOT a cross-store transaction. After successful YAML publication, deletion, cancellation or live apply may fail; ConfigWritten remains true and partial deletion counts are reported. No rollback or automatic repair is attempted. Active sessions are not stopped; nonparticipating writers can still race. Atomic rename does not establish power-loss durability.

setupreset's private writeAtomic remains only for the separate wizard-state document, not configuration YAML. Existing reset callers in internal/server/setup_reset.go, internal/setupresetcmd/execute.go and internal/localruntime/llamaserver tests were compile-checked but not modified; the server callback gets the advanced snapshot through its existing Config argument. Profile/login/logout/watcher migrations and same-host runtime apply serialization remain outside this bounded task and are being changed independently; prior follow-up caller inventory is not asserted to describe their current working-tree state. This is not all-production-writer or Phase 3 completion evidence.
