# Homebrew worker executable selection

Ordinary agent startup resolves its own executable before activating runtime or
store services. If that canonical path has Homebrew's installed layout
`<prefix>/Cellar/cercano/<version>/bin/cercano`, it selects that versioned keg
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
