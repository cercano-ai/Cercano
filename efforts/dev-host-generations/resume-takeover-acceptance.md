# Resume/takeover remaining acceptance (2026-11-01)

> Current scope (2026-11-09): lifecycle Retry/Takeover RPC families are deleted.
> Terminal attachment conflict/replacement remains supported; the acceptance below
> is preserved historical evidence, not a lifecycle recovery API. See
> [safe takeover scope](../../docs/safe-takeover.md).

Base: `bc4887401574e8900fceed353294d073b61ceaf8`, branch `simplified-resume`.

## Expectations stated before execution

Reopening from B selects attached owner A without moving the durable epoch.
Enter, n, and Escape must not replace A's terminal; explicit y alone issues the
conditional replacement. Resuming must preserve B as the default selection.

## New evidence and bounded defect

Extended the existing opt-in ordinary-host subprocess bridge with UI confirmation
commands and authenticated real loopback RPCs. A is started through the ordinary
host fixture entrypoint; B is the existing lifecycle-only coexistence server.
The child environment is now allowlisted and HOME, XDG_CONFIG_HOME, TMPDIR,
database and tokens are disposable. The UI's HOME/config is also isolated.
No inference was requested; no external/local model tools were called.

The initial live probe **failed** with `expected structured terminal conflict,
got host did not acknowledge terminal attachment`. A trailers-only gRPC rejection
can return empty headers without a Header error. The client discarded its status
and structured takeover target. The bounded fix reads the rejection before
canceling when enrollment headers are missing (also on reconnect). Added a
normal, non-opt-in real-transport regression covering both initial attachment and
an obsolete explicit takeover target.

The live probe now proves that Enter/n/Escape retain the original terminal;
y changes the terminal observed through independent authenticated conflicts;
durable owner/epoch remain A; and the UI retains B as default selection. It
cancels and joins the event-wait command after observing replacement.

## Exact commands and outcomes

From repository root:

```
mkdir -p .tmp-resume-acceptance
```

From `source/server`:

```
go test -c -o ../../.tmp-resume-acceptance/server.test ./cmd/cercano
```

Passed (6.565s wall). From `source/clients/cli`, with
`CERCANO_RESUME_PROBE_BINARY=/Users/bryancostanich/git_repos/cercano-ai/Cercano-resume/.tmp-resume-acceptance/server.test`:

```
go test ./internal/ui -run '^TestReopenProductionReleasedAndDisconnected/live$' -count=1 -v
```

Before fix: FAIL (1.258s package). After fix: PASS (0.719s package).

```
go test ./internal/ui -run '^TestReopenProductionReleasedAndDisconnected$' -count=1 -v
```

PASS: released, disconnected, live (0.636s package).

From `source/server`:

```
go test ./pkg/agentclient -run 'TestAttachmentTrailersOnlyConflictPreservesTakeoverTarget|TestTakeoverMetadataOnlyOnInitialEnrollment' -count=1 -v
```

PASS, both tests (0.671s package). No baseline full suites rerun.

## Explicit limits / remaining acceptance

This is a launched **test binary running ordinary host lifecycle**, not a launched
CLI or PTY acceptance. The actual UI confirmation commands execute in-process;
no terminal rendering, keyboard decoding, or launcher publication is claimed.
The bridge publishes test selection files and B is lifecycle-only, not a second
ordinary inference host. The full A → B → resumed A journey still needs a
synthetic-inference, two-ordinary-host launcher/PTY fixture to establish title and
history hydration, a subsequent turn on A, and an actual new conversation/turn on
B. Default-selection preservation is not evidence of that final operation.
No production architecture changes or launcher publication were attempted.
No live private state, ambient processes, installation, restart, push or landing
was used. Temporary build artifact is removed after verification.
