# Direct-child timeout investigation (2026-10-31)

## Result and boundary

This checkpoint is diagnostic/test-only, not a behavioral timeout fix. It keeps the existing shutdown authorization policy unchanged. The new shared classification helper is an equivalent extraction of the existing worker/UID/image/role checks.

The opt-in read-only snapshot observes PID 38883 as an agent owning TCP 50052. Its direct children 38888 and 38889 were revalidated with role `unrecognized`, category `unresolved`, reason `unrecognized executable identity`; 38890 had role `missing` and the same category/reason. Separate `ps -p ... -o pid=,ppid=,comm=` inspection identifies the first two as dotnet and the third as lunie-mcp. No argv, live config/DB contents, or credentials were inspected. Child 49097 was an accepted worker; transient child 49262 was also unresolved in the final snapshot and was gone by a subsequent ps observation. Host identity, role and listener ownership were revalidated. All observations are racy, not authority to signal.

The compiled disposable dotnet-named fixture with a non-worker argument reproduces `unresolved host children ... role=unrecognized reason=unrecognized executable identity: context deadline exceeded`. This verifies the mechanism that would block the observed host if these children persist into shutdown. It does not establish which PID caused the historical reported timeout: no ambient shutdown was attempted.

A new destructive-policy boundary is now concrete: resolving this host by merely accepting these direct children would expand shutdown authority to external runtime/MCP executables that the explicit unknown-image gate rejects. PPID, UID, lifetime and readable image identity alone do not establish their permitted role, private ownership, shared-service status, or absence of detached writers. `internal/mcp_host/manager.go:99` shows configured commands can be launched as stdio MCP servers, but this source fact is not proof that any particular ambient process belongs to that manager. No basename allowlist for dotnet/lunie, blanket direct-child acceptance, or ignoring surviving children was added. Further behavioral work needs a sound ownership/classification contract, not relaxation of this gate. No ambient signals, installed-launcher executions, migrations, permission changes, infrastructure operations or pushes were performed.

## Tests and stale expectations

- Explicit opt-in snapshot passed; bounded to 32 requested PIDs and 64 reported children per host. Uses existing Inspect, processRole, verifiedModelHost, safeUpgradeRole and the extracted production classifier. It revalidates identity/attachment/lifetime, role, and host listener ownership.
- Focused compiled-fixture direct-runtime reproduction, different-image workers and unknown-role regressions passed.
- Initial full brewrestart run failed the pre-existing zero-FD and surviving-alias discovery expectations.
- Source evidence: commit bf00a634 explicitly changed `inspectUpgradeCandidate` to ignore unidentified inventory candidates unless they concretely own the target listener. Thus expecting a zero-FD candidate or a non-target dangling alias named cercano to block discovery was stale. Updated only these expectations; the concrete target-listener case still must fail with the current identity-unresolved diagnostic, and the separate ambiguous-listener/confirmed-exit checks remain.
- Full `go test ./internal/brewrestart -count=1` passed after those test-only reconciliations (33.605s).
- No combined launcher acceptance rerun: this checkpoint changes no shutdown authorization, backup/migration ordering, launcher, persistence, history or reply behavior. It is not an end-to-end acceptance claim. The fixture is a copied compiled Go test image, not actual .NET/MCP behavior; no provider inference was exercised.

Artifacts under `source/server/artifacts/`: `ambient-child-snapshot-final.log`, `direct-helper-reproduction.log`, `brewrestart-snapshot-full.log` (before test reconciliation), and `brewrestart-snapshot-final.log`. Existing earlier artifacts were left untouched. No landing performed.
