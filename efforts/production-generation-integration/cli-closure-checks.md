# CLI closure bounded checks — 2026-10-29

Branch: `feat/production-generation-integration`.

## Scope and readiness gate

Read root AGENTS.md; no deeper AGENTS.md found under source or efforts.
Task inspection of review `task_21bd1e0bf32102a7270dd0cac8da9f8a` returned
`task not found in this conversation`. Both inspection and wait for server task
`task_87ba200ce8b050ee75eefcd701ef5350` returned the same error. Consequently
no claim of review retrieval or server readiness is made. Server/broker files
and plan.md were not edited by this bounded client task.

The compiled rehearsal was NOT executed or extended to same-conversation
release: the required readiness gate could not be verified. Its current
ordinary-session-close path still closes the sibling conversation, not the
original conversation owning the task. The evidence label now says so explicitly.
Unexpected-disconnect, busy reopen, history reopen and B persistence checks
remain in the harness. No nested worker delegation was run or claimed.

## Analytical expectations (before any future compiled run)

For same-conversation acceptance, A must own a running independent task when
that exact conversation intentionally quits. Its terminal ownership should be
released without cancellation; task/execution accounting should keep A alive
until durable completion and final callback accounting drain. Once all other
A sessions are intentionally closed, A should retire automatically without
ownership-management release/transfer/retire calls. B must remain the current
target and persist a fresh user turn and assistant reply after A exits.
Unexpected disconnect must instead retain ownership; explicit detach followed
by quit must also retain it. Existing reopen checks must remain separately
observable rather than being misreported as same-conversation drain evidence.

## Reproduced findings and checks

Before fixes, `python3 -m unittest discover -s efforts/dev-host-generations
-p test_compiled_closure.py` produced three failures: intentional quit did not
close its PTY, obsolete ordinary-closure-probe retention expectation remained,
and help omitted the detach exception. After fixes: 3 tests passed (0.004s).
The probe now follows ordinary quit through the existing automatic-retirement
and B-persistence assertions instead of returning a stale blocked result.
`python3 -m py_compile efforts/dev-host-generations/everyday-launcher-compiled.py`
passed. `git diff --check` passed.

Added loopback-only synthetic RPC regression for final model client replacement,
detached no-op and release-error propagation. First run failed because the fake
peer lacked credential-isolation attestation. Added the fake's disabled-policy
response, without weakening production isolation. Final checks:

- CLI `go test ./internal/ui -run 'Test(Intentional|CloseSession)' -count=1`:
  PASS, 0.411s.
- CLI `go test . -run '^$'`: PASS, compile-only (no tests run).
- Server `go test ./pkg/agentclient -run 'TestReleaseSession' -count=1`:
  PASS, 0.392s.

Go checks used private temporary HOME/XDG paths, CERCANO_DISABLE_CREDENTIALS=1,
GOPROXY=off and GOSUMDB=off, with existing Go module/build caches. No actual
credentials, user history or production runtime were used. RPC regression uses
127.0.0.1 ephemeral listeners only. Full suites and compiled PTY process exit
failure injection were not run: error propagation is tested through CloseSession,
not the final operating-system exit code.
