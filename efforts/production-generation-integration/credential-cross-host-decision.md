# Cross-host credential logout ordering — approval required

2026-10-28. Approved Phase 3 already requires preventing late refresh from resurrecting logout. This note does not reopen that requirement; it records a narrower security/availability choice about an in-flight external operation. No credential coordination implementation was performed.

## Observed evidence

Source audit task task_a0433766d4c7f1464f5a208d6e005a6b found per-Service profile maps/mutexes/generations, plain credential CRUD, and no shared durable generation/tombstone. It did not locate the repo-root spec from its server-scoped working directory; its product-contract uncertainty is superseded by the next probe's reading of the actual approved spec/plan.

Diagnosis task task_9356ad0f1e33d8683f62191b080bfda7 read the approved spec/plan and reported a temporary memory-only barrier probe reproduced resurrection 10 times under -race. Two Service objects shared secrets.NewMemory. A staged a refresh and waited; B.Delete succeeded and backing state was absent; releasing A allowed its unconditional Set to restore the credential. Passing diagnostic tests assert the defect, not security correctness. The test was removed. Outer credentials were disabled; a local nonparallel enabled scope used only explicitly injected memory store/package-local fake transaction resolver. No auth HTTP/native backend/live user data was used. This is two-service evidence, not multi-process native qualification.

Relevant source: hostsvc/credentials/service.go profile generation and flight (24–69), Delete local invalidation (138–155), snapshot acquisition (214–230), local-generation commit check and Set (257–271). Auth envelopes contain tokens/expiry/account identity, not durable refresh uncertainty or generations. secrets.Store exposes ordinary Get/Set/Delete/List, not conditional commits.

## Decision needing approval

Should logout wait behind an admitted cross-host refresh, or supersede it without waiting for the issuer?

- **Serialize logout behind refresh:** shared identity operation exclusion spans external refresh and credential commit. Logout waits or returns explicit busy/pending; it does not claim success early. Lower coordination complexity, but an uncooperative issuer can delay logout. Ambiguous token rotation still needs explicit handling; a lock is not a crash-recovery solution.
- **Prompt logout with durable stale-operation fencing (recommended):** invalidate the identity's operation generation before deletion, reject delayed refresh/login commits, and serialize competing refresh operations separately. More complex persistent coordination and compatibility/crash tests, but logout need not wait for an external issuer. This cannot guarantee immediate revocation of already-issued remote tokens, and native backend calls may themselves block.

Both require participating hosts, fresh reads under coordination, conservative external-rotation ambiguity handling, and no secret-bearing notifications. Neither option authorizes a new service, second credential authority, changing existing credential profile identities, or live native access. Specific durable representation/location and compatibility implications require source-grounded review before implementation; none was selected by the audit.

## Concurrent completed work

Watcher slice c0e4e4d7c31c adds two-Server callback convergence and saved/not-applied outcome tests. Parent reran (offline, CERCANO_DISABLE_CREDENTIALS=1): go test -race ./internal/server ./internal/hostsvc/providers -run 'TestCrossHostConfig|TestConfigApplyCounterexample|TestReloadConfigFromDisk|TestStartConfigWatcher|TestUpdateConfig|TestRemoveCloudProfile' -count=2. Server passed; providers had no matching tests. Full suites were not repeated after this slice. Failed application can still retain old provider graph, and identical baseline reload skips retry; applied-revision/admission reconciliation remains incomplete. Canonical activation remains disabled.
