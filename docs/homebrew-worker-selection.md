# Homebrew worker executable selection

Ordinary agent startup resolves its own executable before activating runtime or
store services. If that canonical path has Homebrew's installed layout
`<prefix>/Cellar/cercano/<version>/bin/cercano` (legacy monolith) or
`<prefix>/Cellar/cercano-agent/<version>/bin/cercano` (split agent), it selects that versioned keg
for worker launches. This supports arbitrary Homebrew prefixes and version
strings (including formula revisions). A public `bin` or `opt` symlink can be
used to start the agent; it is resolved once, not consulted again per turn.
Non-Homebrew development executables retain existing worker discovery.

The ordinary production execution policy installs this selection into the
server's normal turn runner and worker pool. With worker execution enabled,
new workers use the exact absolute executable path in the selected keg; they
do not resolve `PATH`, `opt`, or the public alias again. Before each new launch,
the policy rechecks the canonical path, keg and executable filesystem identities,
regular/executable mode, and executable SHA-256. If the selected keg is missing,
redirected, replaced, or its executable contents have changed, launch returns an
error instead of discovering another worker. An already warm worker continues
under normal pool lifecycle rules; selection validation applies to new launches,
not to an already running process. `in_process` execution remains in-process.

Thus if an agent selected A and Homebrew retargets its public alias to B, that
agent's subsequent worker launches still select A, **provided A remains intact**.
This is selection/pinning, not a retained installation or a package-manager lock:

- It **does not prevent Homebrew from deleting old kegs**. Cleanup of A causes
  subsequent launches from that agent to fail closed, not to migrate to B.
- It **does not establish B-session coexistence**. Ordinary host resources,
  singleton/session behavior, and credential authority are unchanged.
- It **does not retain external llama installations**, models, libraries, or any
  other dependency. Only the Cercano executable is hash/identity checked; the
  whole keg and dependencies are not content-pinned.
- Path/stat/hash checking followed by OS process launch is not an atomic
  executable lease or a defense against adversarial concurrent filesystem
  mutation between validation and exec.

This installed-keg policy does not require copied bundles, a manifest, a new
inventory, local inference, or a llama installation. Agent-only and remote-only
installations use the same selection. Protected generation startup is a separate
policy and continues to require its existing manifest, admission checks, resource
namespace, and prepared-session authority. Installed selection grants none of
that authority and cannot satisfy `ValidatePrepared`.

## Fixture verification

No Homebrew command, installation, live agent, model, or credential is needed:

```sh
# Run from source/server
go test ./internal/worker ./internal/server ./cmd/cercano -run 'TestInstalled|TestProductionCompositionPolicy|TestSelectExecutionMode' -count=1
go test -race ./internal/worker ./internal/server ./cmd/cercano -run 'TestInstalled|TestProductionCompositionPolicy|TestSelectExecutionMode' -count=1
```

The worker fixture drives the normal runner's `RunTurn` through its real pool
spawn closure. A shell probe records the selected executable and deliberately
exits before worker handshake (it is not an agent). Fixtures cover alias A→B,
in-place content changes, missing executables, identical-byte inode replacement,
non-keg selection, rejection as prepared authority, and server dispatch-mode
installation. All filesystem changes are confined to isolated fixture directories.

## Follow-up validation (2026-11-05)

The split formula `release/homebrew/cercano-agent.rb.in` owns `bin/cercano`;
the new `cercano` convenience formula owns only a README and dependencies.
Selection now recognizes both agent-owning formula layouts, but rejects
`cercano-cli`, unrelated formulas, and formula-name prefixes. Both layouts run
through the real worker `RunTurn`/pool fixture; server dispatch uses the split
layout. No installed packages, live agents, configuration, or VMs were modified.

Tool discovery `/bin/sh -c 'command -v go; command -v gofmt; go version'`
succeeded: `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`,
`go version go1.26.3 darwin/arm64`. The earlier checkpoint's inability to execute
these tools does not describe this validation run.

Before the implementation fix, this regression failed with
`selection: false, <nil>` (exit 1):

```sh
go test ./internal/worker -run '^TestInstalledKegPoolPin/cercano-agent$' -count=1
```

Initial broader runs exposed fixture instability: nested temporary paths exceed
Darwin's socket budget, and a one-second startup deadline was insufficient under
race-test load. The fixture now uses a private short temporary directory, cleans
it up, and allows five seconds for the deliberately rejected handshake.

Final commands from `source/server`, all successful (exit 0):

```sh
gofmt -w internal/worker/installed_keg.go internal/worker/installed_keg_test.go internal/server/installed_worker_test.go
go test ./internal/worker ./internal/server ./cmd/cercano -run 'TestInstalled|TestProductionComposition|TestSelectExecutionMode|TestOrdinaryGenerationStartup|TestDevelopmentProductionMissingAdmissionBeforeOpener|TestDevelopmentValidatedStartupBinding' -count=1
go test -race ./internal/worker ./internal/server ./cmd/cercano -run 'TestInstalled|TestProductionComposition|TestSelectExecutionMode|TestOrdinaryGenerationStartup|TestDevelopmentProductionMissingAdmissionBeforeOpener|TestDevelopmentValidatedStartupBinding' -count=1
```

Normal package results: worker 12.987s, server 0.780s, command 1.269s.
Race package results: worker 25.662s, server 2.188s, command 3.024s.
These compile all three packages, but run only the named focused tests, not the
full suite or native package acceptance. Other Go files in checkpoint 3ebc235f3438
were checked with `gofmt -l` and produced no output. Review included startup
admission ordering, composition validation, runner selection, pool acquisition,
and installed/prepared authority separation.

An initial parallel tool-wrapper invocation lost its parameters and returned
`run_command: cmd is required; no action performed`,
`read_file: path is required; no action performed`, and
`glob: pattern is required; no action performed`. Direct tool calls worked.
An exploratory grep also returned missing-file errors for unmatched
`startup*test.go`, `*admission*test.go`, and nonexistent `internal/worker/runner.go`;
subsequent inspection used the actual `host.go` and ordinary/composition tests.
