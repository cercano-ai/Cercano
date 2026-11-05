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

## Opted-in Homebrew keg retention

The approved operational policy uses normal (unversioned) formulas and a
persistent, formula-specific Homebrew setting:

```text
HOMEBREW_NO_CLEANUP_FORMULAE=cercano-agent
```

Place this assignment in the appropriate supported `brew.env`, preserving all
other settings and any existing comma-separated exclusions. This is an external
Homebrew invocation policy, **not configuration installed or enforced by Cercano**.
For user scope, Homebrew reads `$XDG_CONFIG_HOME/homebrew/brew.env` when
`XDG_CONFIG_HOME` is set, otherwise `$HOMEBREW_XDG_CONFIG_HOME/homebrew/brew.env`
when set, otherwise `$HOME/.homebrew/brew.env`. Different users, service
invocations, HOME/XDG overrides, prefix/system settings and precedence can change
which policy is effective; verify with `brew config` in each invoking context.
Do not assume a setting in a synthetic test user applies to a normal shell.

With the exclusion effective, ordinary A→B upgrades remain allowed and the public
command selects B. Homebrew automatic cleanup skips old `cercano-agent` kegs;
explicit targeted cleanup also refuses to remove them. On tested Homebrew 7.0.12,
that explicit refusal printed `Error: Refusing to clean ...` but returned **0**;
check the diagnostic and retained filesystem state, not just exit status.
An intact canonical A path remains launchable, allowing the selection policy
above to keep choosing A rather than following the updated public alias.

Limitations:

- This is **not explicit uninstall protection**, a package-manager version pin,
  or an immutable executable lease. Removal/replacement still makes new pinned
  launches fail closed.
- External llama installations and models are not retained by this setting;
  it does not guarantee old dependency/library versions. Homebrew also excludes
  runtime formula dependencies from autoremove, but that is not a whole-runtime
  version lease. Neither the convenience formula nor `cercano-cli` receives the
  agent exclusion.
- There is **no automatic retirement** or safe-to-delete detection. Inactive agent
  kegs are retained for now; disk usage grows across upgrades. Any retirement
  requires separately authorized lifecycle work after users/processes drain.
- Keg retention alone proves neither session coexistence/continuity nor actual
  A-agent dispatch. The native rehearsal below used pre-worker-policy synthetic
  binaries; their direct worker probes establish launchability only.

### Disposable native retention rehearsal (2026-11-05)

Only guest `{d5633dbc-c8a1-49fc-a420-df6d5d60d952}` was changed. The setting remains
in `/tmp/cnp.h8sjTc/config/homebrew/brew.env`, scoped to the established synthetic
fixture user's `XDG_CONFIG_HOME=/tmp/cnp.h8sjTc/config`; it is **not guest-wide**.
The file was absent before the run; existing Homebrew trust/settings were left
unchanged. No host Homebrew configuration or live Cercano was changed.

Ordinary native install/upgrade of preserved synthetic A `0.0.901` and B `0.0.902`
succeeded with automatic cleanup enabled. Cleanup removed old convenience/CLI
kegs but kept agent A's inode and SHA-256. Explicit agent cleanup refused removal;
a fresh canonical A worker and public-command B worker both returned HTTP/2
SETTINGS. All three owned probe processes exited on targeted SIGTERM. Agent A/B,
convenience B and CLI B remain installed, with the exclusion, for reproducible
inspection; no uninstall was attempted. Exact commands, hashes, the initial
harness exit-code assertion failure and successful continuation are recorded in
`efforts/cross-platform-updates/verification.md` and
`/tmp/cnp-macos-update-retention/policy-evidence.tar.gz` on host and guest.

## Accepted local-runtime rehearsal boundary (2026-11-05)

The user explicitly accepted ("that's fine") the closeout/native rehearsal scope:
**Cercano upgrades only**, while the external llama executable, libraries,
backends/resources and models remain unchanged for all active dependent sessions,
including idle sessions and later work/restarts. Defer external runtime/dependency/
model upgrades or cleanup affecting those paths until those sessions finish.
This is an accepted scope limitation, not a guarantee of external cleanup safety
or completed native qualification. No new manager or live policy edit is involved;
automatic retirement remains deferred.

The four Phase 3 modelhost tests already passed normally and under the race
detector (not rerun for this record). Fake-runtime hold tests are not real
inference. The human checklist must verify actual local inference after idle
across the Cercano upgrade, plus later work/restart usability with those external
paths unchanged. Results remain outstanding; see
`efforts/macos-native-validation-closeout/verification.md`.

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
