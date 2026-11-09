# Windows Phase 1 sanitized handoff

## Publication maintenance — 2026-11-09

**Branch:** `handoff/windows-replacement-sanitized`, based directly on the already-published shared base `a9ca2fb82831d707863b132ed650e591cded19cf`. This branch contains a sanitized net derivative, not the original private Windows feature commits. See `windows-sanitized-provenance.md` for privacy scope and evidence integrity.

**Status:** source audit and bounded synthetic managed/ARM64 results retained. Product implementation and Windows acceptance remain incomplete. No new experiment, VM/process/configuration access, live script execution or provider-blocked probe retry/rerouting occurred during sanitization. The historical observations below are not fresh VM observations.

### Evidence and remaining work

- Source audit: `windows-transaction-audit.md`. BeforeModify is not a reliable veto; manager recovery is not an application-state transaction.
- Stored synthetic managed observations: `windows-phase1-results.md`, `windows-phase1-checkpoint.md` and the phase1/extensions/smoke/consolidated JSON captures. Offline assertions cover 19 stored cases, not production acceptance.
- Native baseline/recovery/fix reports retain partial outcomes: serialization stall, incomplete recovery, then failed numeric-exit capture. They are not passes.
- `native-go-next-results.md` plus next/guard evidence retain corrected ARM64 observations: running A survived successful replacement; executable/resource locks failed followed by ordinary closed retries. Exactly one synthetic guard backstop refused with exit/retry guidance, followed by successful retry. No race-free exclusion is established.
- Historical per-package cleanup is not VM-wide cleanup or a current residual inventory. Historical run directories, feeds/logs and earlier failed-run packages may remain.
- The original local feature branch remains untouched and unpushed at `c9ee1b48104945d350891d16528eeff1d1987c18`, with raw captures and the three untracked diagnosis/race files preserved. Those files, private substitution maps and local blocker/state files are excluded from this derivative.

1. Leave the provider-rejected concurrent-launch probe **unexecuted**. No completion result exists; do not reroute, retry or rephrase it. Restriction resolution must use the supported process.
2. Resolve incomplete/inaccessible process identity, other-user/elevation permissions, concurrent launch/reconnect and upgrade/uninstall shim timing. No MSI or alternate architecture is approved.
3. Continue the existing phase 2–4 checklist only within authorized boundaries: independent agent/CLI packages, dependency-only convenience package, verified artifacts, command ownership, affected-component exit guidance, reconnect and native acceptance. ARM64 fixtures do not qualify x64 shipping artifacts; local llama delivery remains unqualified.
4. Actual product/configuration/conversation/credential preservation, account boundaries, native amd64, failures/recovery and real split-package lifecycle remain unqualified. Synthetic sentinels demonstrate only fixture preservation.

User-approved close-first scope supersedes rolling continuity. No custom updater/service, forced application kills or retained Windows generation architecture is authorized. This push publishes source/evidence documentation only, not packages/releases, signing, deployment or merge approval.

### Offline checks and publication boundary

Stored-evidence assertions, the bounded offline native audit, JSON/Python syntax, derivative checksum verification, whitespace, all-outgoing-blob privacy/text review and ancestry checks are the applicable checks. The native audit intentionally retains `BLOCKED_INCOMPLETE_NOT_ACCEPTANCE_PASS` and missing-transcript failure; these are historical technical outcomes, not sanitization failures. No guest/live tests are appropriate here.

The earlier ancestry privacy blocker is resolved by explicit scope clarification: already-published shared ancestry is out of scope and must not be rewritten. Privacy review covers all newly outgoing file versions and commit messages, not preexisting shared history or normal repository author attribution. The original unsanitized feature commits are excluded from this branch ancestry.

The branch is intended for a normal, branch-only origin push with upstream configuration. Remote SHA and ahead/behind are verified after publication; no shared history is rewritten.

## Historical handoff — 2026-11-06

The disposable clone was provisioned with Chocolatey 2.7.5. Process-only PowerShell execution-policy bypass was authorized; persistent policy changes were not needed. The original Windows VM remained stopped in the latest recorded inventory.

## Reviewed completed evidence

See `native-go-next-results.md` and its raw evidence for the corrected Windows ARM64 fixture runs. Independent read-only review confirmed held exits closed/running/locked-executable/locked-resource of 0/0/1/1; all ordinary closed retries returned 0 and fresh direct/public-shim launches reported exact B executable/resource identity. Running A survived replacement while Chocolatey reported success. Thus ordinary replacement alone is not a running-process refusal mechanism.

The one synthetic full-path process-check experiment returned -1, with actionable exit-and-retry guidance and final failure/manager rollback, followed by successful ordinary retry after cooperative fixture exit. BeforeModify alone was not a veto. All five IDs from these corrected runs were ordinarily uninstalled, fixture processes exited cooperatively, and synthetic sentinel hashes remained unchanged. Historical failed-run packages and evidence remain; this is not VM-wide cleanup or product acceptance.

Serializer and exit-code capture fixes have bounded regressions. Projection checks and worker joins are not a hard end-to-end bound on filesystem operations or the whole experiment. Only the earlier stuck test-owned PowerShell harness was terminated through an identity-verified retained handle; package scripts did not kill application processes.

## Remaining limitations

- Process inspection is a snapshot, not exclusion of concurrent launch. No race-free replacement boundary is established.
- The guard rejects incomplete identities for identified candidates, but inaccessible executable paths with nonmatching names can be skipped. Universal conservative inaccessible-process handling is not established.
- Upgrade/uninstall shim-phase races, cross-user and permission cases, native amd64, real product/component packaging, reconnect behavior, and local-runtime delivery remain unqualified.
- The Windows amendment requires actionable failure, truthful status and data safety, not atomic preservation of every package file after failure. These results do not establish that MSI or any alternate architecture is necessary.

## Execution blocker

The next concurrent-launch fixture task (`SANITIZED_BLOCKED_TASK`) failed at the model provider with an `invalid_request` safety-gate response flagging possible cybersecurity risk. It returned no completion result. No passing race outcome is claimed, and the task was not rerouted or retried to bypass that restriction. Any partial files from this attempt are not accepted experiment evidence.

Continue only after the provider restriction is resolved through its supported process, or the user chooses a different permitted scope. Production packaging is not ready to proceed on an assumed safe hook contract. Historically no push, merge, release or publication was performed; this sanitized handoff is separately published as described above.
