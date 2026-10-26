# Regression and terminal acceptance follow-up

## Authorization — 2026-10-26

The user explicitly requested keeping the feature unlanded while fixing concrete compile/test regressions, establishing safe credential-test isolation, diagnosing remaining failures, and completing minimum terminal integration for the already-agreed A/B workflow. No new architecture or expanded lifecycle promises.

Required gates: rerun the full server and CLI regression suites and perform actual-terminal acceptance, including physical disconnection while work is active. Report automated fixture evidence separately from real-model evidence. Only after these gates pass: reconcile with current main, rerun affected checks, land locally, and smoke-test the landed build. This newer conditional authorization supersedes older effort language prohibiting local landing; it does not authorize push, live installation changes, or user credential access. Preserve unrelated edits.

The autonomous run brief was approved in this follow-up. Completion remains conditional on observed evidence; no new passing result is claimed here.

## Work separation

- Regression task `task_abb053fcb5768d4dd76c60002bf93c8b`: reproduce concrete failures, fix test mocks/fixtures, establish fake credential isolation, diagnose remaining failures. No terminal startup/UI edits.
- Terminal task `task_8c6d7432a9d576fc69bf02bdd3823dd1`: wire existing authenticated pinned/lifecycle APIs into actual terminal, narrowly expose required fixture RPCs, and prepare active-disconnect acceptance.
- Independent read-only isolation review `task_338b25f63adf3a8ebfece90297eb469c`: audit enabled credential-test semantics and backend/network safety.

Native Keychain exclusion cannot be inferred from a temporary HOME. Clearing credential disabling globally is not acceptable isolation. Passing synthetic provider/hold tests will not be described as real loaded-model evidence. Existing plan.md edits and retained PTY artifacts are preserved.
