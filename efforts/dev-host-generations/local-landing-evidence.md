# Local landing evidence — 2026-10-26

User-authorized conditional local landing completed after gates passed.

- Previous local main: `361e38879209b84703cd56e331a0bd80100589a4`.
- Landed feature/main: `0143e439f664ed0d2e9aaa1573cec97fd57c20b9`.
- Tested code: `9004eddd850e93159c8ef45693dd1cd6ecfbb258`; the only following commit before landing added followup-verification.md.
- Current local main remained an ancestor. No rebase or conflict resolution was necessary. No fetch or push.

Delegated landing task `task_c1375a44055e6ddf747a28540d1afad5` rechecked ancestry, status, path collisions, and preserved-file hashes. git_land refused the intentionally dirty feature worktree; an explicit authorized `merge --ff-only` with hooks disabled advanced main without a merge commit or modifying the preserved feature dirt.

## Fresh checks before landing

Committed-source snapshots avoided unrelated untracked source files, notably main's `source/server/internal/server/probe_wake_persist_test.go`. Full checks used a replacement environment, private HOME/XDG/short TMP, offline Go dependencies and outer disabled credentials.

- Injected-opener probes: PASS.
- Full server suite: PASS, 117 packages, 35 built-in skips.
- Full CLI suite: PASS, 10 packages, one built-in skip.
- Expanded actual-terminal PTY gate: PASS twice.

## Landed-source smoke

- Server and CLI built to temporary outputs with the exact landed SHA embedded: PASS.
- Both binaries --version and --help: PASS.
- Compiled-startup and actual-terminal PTY smoke from landed-source snapshot: PASS. The harness itself builds fixture-labeled binaries from that source.

Artifacts: `/tmp/cl-zdok2y99/` retains command manifests, environment, results, transcripts/logs, binaries, revisions, preservation hashes and SHA256SUMS. This document and the skip-list wording correction are documentation-only changes following the tested landing; they do not change code.

## Preservation and limits

Retained feature worktree `/Users/bryancostanich/git_repos/cercano-ai/Cercano-dev-host-generations`: contains preserved plan.md edits and `.terminal-ab-audit.5OklQb/` artifacts. Containment and unchanged statuses/hashes were verified by the landing task. No force removal, branch deletion, stash/reset/clean, or nested-worktree cleanup. Main's unrelated untracked content remains untouched.

No live installation changes or existing-host restarts. No real-model inference or loaded-model retirement qualification; PTY evidence uses restricted synthetic fixtures. The earlier credential-test isolation incident remains disclosed in credential-test-isolation-incident.md; passing later gates do not establish that earlier native access did not occur. Unix socket path-length sensitivity and built-in test skips remain documented in followup-verification.md.
