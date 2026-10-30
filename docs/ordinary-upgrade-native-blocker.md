# Native upgrade shutdown: bounded regression and acceptance blocker

Status: **not complete; not qualified for migration acceptance** (2026-10-30).

## Implemented diagnostic unit

Darwin shutdown now verifies identities and listener ownership, freezes the host,
then enumerates direct children with `proc_listchildpids`. This API returns a PID
count, although its buffer argument is bytes. Workers are individually signalled,
not signalled through the host process group. The host remains stopped through
TERM / twenty 100ms polls / KILL, preventing it from launching more workers during
that interval. Shared model-host children are excluded. Unknown children refuse;
error cleanup attempts to resume only the original host identity.

This regression establishes exit of one disposable host and one active worker,
without the fixture's historical RPC cancellation marker. It does **not** prove
racing worker coverage, production writer inventory, or migration acceptance.

## Commands and outcomes

Working directory for Go commands: `source/server`.
Exact test argv for all three variants:

```
/opt/homebrew/bin/go test ./internal/brewrestart -run '^TestUpgradeStopsRealHostAndActiveWorker$' -count=1
```

The command ran through Python `subprocess.run` with environment loaded from the
retained disposable `upgrade-check-y4xz4a69/env.json` under the Darwin temporary
directory. Explicit settings were:

```
GOMODCACHE=/Users/bryancostanich/go/pkg/mod
GOPATH=/Users/bryancostanich/go
GOCACHE=/Users/bryancostanich/Library/Caches/go-build
GOROOT=/opt/homebrew/Cellar/go/1.26.3/libexec
GOPROXY=off
GOSUMDB=off
GOTOOLCHAIN=local
CERCANO_DISABLE_CREDENTIALS=1
```

HOME and TMPDIR were that disposable root; XDG_CONFIG_HOME and XDG_RUNTIME_DIR
were its `config` and `runtime` children. Test processes additionally use `t.TempDir`.
No database migration was run in this diagnostic.

Expected before baseline/verification: one selected test fails against the old
RPC implementation, one selected test passes with native external shutdown.
Observed:

- Baseline: temporarily replace `upgrade_darwin.go` with
  `git show HEAD:source/server/internal/brewrestart/upgrade_darwin.go`, preserving
  and restoring edited bytes in `finally`. Exit 1,
  `unexpected historical RPC shutdown`.
- Patched scoped-discovery regression: exit 0, `ok ... 2.656s` (final run).
- Production-discovery diagnostic: temporarily replace the final fixture call
  `stopUpgradeHost(ctx, endpoint, []int{host.Process.Pid})` with
  `StopUpgradeHost(ctx, endpoint)`, restoring test bytes in `finally`.
  Expected refusal on unowned ambient processes; observed exit 1,
  `inspect PID 283: no such file or directory`, before termination.

A separate read-only process-name snapshot used:

```python
s = subprocess.check_output(['ps', '-U', str(os.getuid()), '-o', 'pid=,comm='], text=True)
# Print only PID and basename for exact cercano / cercano-agent basenames.
```

It found numerous ambient same-user Cercano processes, including PID 283. Names
are discovery evidence only, not ownership evidence. None were signalled.

## Decision required for production acceptance

Production discovery scans same-user candidates. It fails closed on the ambient
PID 283 inspection error before it can terminate the owned disposable fixture.
Even if this inspection error were bypassed, unrelated known agents remain
blockers under the preserved global detector. Disposable HOME/XDG settings do
not isolate a UID process namespace.

Safe alternatives:

1. Run the unmodified production-discovery acceptance on an isolated macOS
   account or VM with no ambient Cercano processes (preferred).
2. Explicitly authorize scoped candidate discovery for acceptance while retaining
   native identity, listener, signal and exit checks. This would be a narrower
   acceptance claim than unmodified production discovery.

Stopping ambient processes, ignoring inspection errors, or weakening production
process detection is not authorized. No such action was taken. No combined
confirmation/termination/backup/migration/startup result, decline invariance, or
compatible A/B result is claimed here. Launcher/command integration edits remain
uncommitted and unqualified; the native diagnostic checkpoint is not release
approval.
