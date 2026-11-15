# Homebrew split — non-publishing definitions

**Not release-ready.** These definitions prepare Phase 4 in parallel with runtime
retention/activation work; they are not native acceptance or a support claim.

| Formula | Commands installed | Package dependencies |
| --- | --- | --- |
| `cercano-agent` | `bin/cercano` | none (no CLI dependency) |
| `cercano-cli` | `bin/cercano-cli` | none (remote agent remains possible) |
| `cercano` | none | `cercano-agent`, `cercano-cli` |

All three use the same coordinated, verified darwin-arm64 archive and version.
The component installs select only their binary. The archive currently contains
those two executables, LICENSE, README.txt and runtime-inventory.json; it has
**no bundled llama/full model runtime**. No additional runtime resources are
invented or installed. The inventory describes the combined archive, not a
complete installed agent runtime, so it is not copied into either component.
Homebrew manages acquisition, checksums, Cellar, links, upgrades and removal.
There are no service/post-install/restart hooks, custom updater or user-state
operations. Bare dependencies deliberately do not promise exact-version or
atomic upgrades: partial upgrades and repository lag require native testing.

## Offline tests and rendering

From the repository root:

```sh
python3 -m unittest discover -s release/homebrew -p 'test_*.py'
ruby release/homebrew/test_formula.rb
```

Use a local coordinated archive with the exact release layout/name and its
independently trusted lowercase SHA-256:

```sh
mkdir -p /tmp/cercano-formulas
python3 release/homebrew/render_formula.py --version 1.2.3 \
  --archive /path/to/cercano-1.2.3-darwin-arm64.tar.gz \
  --sha256 "$TRUSTED_SHA256" --output /tmp/cercano-formulas/cercano.rb
```

The existing `--output .../cercano.rb` entry point and returned path are retained;
it now also creates sibling `cercano-agent.rb` and `cercano-cli.rb`. A custom
`--template` must have matching sibling component templates. All inputs/templates
are validated before writing. Existing outputs (including dangling symlinks)
are refused; exclusive creation does not clobber files arriving after preflight.
Use a private output directory controlled by the caller, with no untrusted
concurrent writers. On write failure, rollback compares device/inode identity
against retained open descriptors, skips missing or replaced paths, and attempts
all independent owned outputs even if one cleanup fails. Cleanup errors are
reported alongside the original write failure. This bounded check is not hostile
concurrency safety: checking identity and unlinking are not atomic, and in-place
modification of the same inode is not detected. This is not a crash-atomic
multi-file transaction: after a hard interruption discard the entire private
output directory and rerender into a fresh directory. Never treat an incomplete
directory as a releasable set.
Hash, strict tar layout and inventory checks remain mandatory; signatures,
notarization, architecture and binary-reported version are not verified here.

## Coordinated offline promotion preparation

After rendering, use a **private local snapshot** of the tap (with a `Formula/`
directory), not the installed Homebrew tap. No fetch or clone is performed:

```sh
python3 release/homebrew/prepare_promotion.py --version 1.2.3 \
  --archive /path/to/cercano-1.2.3-darwin-arm64.tar.gz \
  --sha256 "$TRUSTED_SHA256" --formulas /tmp/cercano-formulas \
  --tap /path/to/private-tap-snapshot --tap-base-sha "$REVIEWED_TAP_COMMIT"
```

Stdout is a review-only JSON plan containing **exactly three** paths, trusted
canonical formula contents, before/after text hashes and update/no-op decisions.
Capture it in a new private evidence file if wanted; the tool itself writes
nothing. All siblings must match the repository templates exactly (no legacy-owner
exception); the archive must pass checksum, layout and inventory validation.
Every existing formula is checked for downgrade and same-version checksum/text
drift. A missing component is an addition, not permission to omit it. Symlink
formula inputs/targets are refused. Use caller-controlled directories without
concurrent writers; this is not a hostile-filesystem snapshot or a Git base lock.
The optional full lowercase `--tap-base-sha` records the caller-supplied snapshot
commit in the plan (null when omitted); it does not authenticate the snapshot.
Keep the pinned read-only API tree/blob evidence alongside it. The before hashes
and base SHA are review evidence, not authorization to apply a stale plan.
Unrelated tap files are never changed. Ruby DSL tests remain a separate check.

This deliberately stops **before tap mutation**. A future separately authorized
publisher must stage the whole set in one commit and compare-and-swap the reviewed
tap base, revalidating all inputs and dependencies. It must not revive sequential
single-file Contents API writes or force updates. Even all-no-op plans do not
prove published assets, native behavior or distribution trust. Existing helper
transport tests do not implement or authorize a coordinated live transaction.

## Publication is blocked

The macOS review artifact explicitly includes all three rendered formulas.
Both `.github/workflows/release-macos.yml` and `update-homebrew.yml` still call
`update_tap.py`, whose `update()` fails closed **before network access**, even
with credentials. `promote_to_tap.py` also refuses local promotion, including
`--skip-download`. Offline preparation does not bypass either guard. Do not
dispatch release workflows: the macOS workflow has other release actions outside
this bounded guard. No publication, network release action, credentials, signing
or release-tag change is authorized here.

The one-time manual migration policy is approved; native evidence for it is not.
Keep installed-user applicability, migration/failure recovery, human acceptance and
final signing/notarization gates pending. See the [closeout handoff](../../efforts/macos-native-validation-closeout/human-acceptance.md).
Its same-source synthetic native rehearsal is real bounded evidence, **not**
changed-source or real-runtime acceptance. The accepted support boundary is
Cercano-only upgrades with external llama executable, libraries, backends,
resources and models unchanged for all dependent sessions, including idle time
and later launches/restarts. No broader external-runtime cleanup guarantee is
claimed, and this preparation does not drop existing supported platforms.

## Native fixture recipe — instructions only, not executed

Use a **fresh disposable macOS arm64 machine with an isolated Homebrew prefix and
HOME**, never the developer's host brew/agent/state. No VM activity is part of
this slice. Prepare a local tap and local artifact; do not fetch production assets.

1. Render as above into a fresh directory. For install smoke testing, use binaries
   built locally for that disposable system. Keep the original archive/hash and
   rendered definitions as evidence. A tiny inert archive suffices only for layout
   and ownership checks, not `brew test` or runtime acceptance.
2. On that disposable machine create a local tap:
   `mkdir -p /tmp/cercano-native-tap/Formula; git -C /tmp/cercano-native-tap init`.
   Copy **all three** rendered `.rb` files into its `Formula/` directory. In this
   fixture copy only, change each `url` to the same absolute
   `file:///path/to/cercano-1.2.3-darwin-arm64.tar.gz`; retain its exact checksum.
3. Set `HOMEBREW_NO_AUTO_UPDATE=1` and `HOMEBREW_NO_INSTALL_FROM_API=1`.
   Run `brew tap fixture/cercano /tmp/cercano-native-tap`, then
   `brew install --formula fixture/cercano/cercano-cli`.
   Check `brew list --formula` has no agent and
   `brew list fixture/cercano/cercano-cli` owns only `bin/cercano-cli`
   (apart from Homebrew metadata/license bookkeeping). Run `brew test
   fixture/cercano/cercano-cli`; remote-only connectivity needs a separately
   isolated fixture agent, not the developer's agent.
4. Uninstall CLI, install `fixture/cercano/cercano-agent`, confirm no CLI was
   pulled in and only `bin/cercano` is owned; run its version-only `brew test`.
   Install `fixture/cercano/cercano`, verify both dependencies and confirm its
   own keg has no commands. Inspect `command -v cercano` and `cercano-cli` and
   `brew list` for exclusive ownership. No installation should start an agent.
5. Prepare locally built A/B sets in the same tap. Test `brew upgrade` of each
   component independently and the convenience package, including a tap missing
   B for one dependency. Record installed versions separately from running
   versions; dependency resolution is not activation evidence. Verify uninstall
   preserves fixture configuration/conversations/credentials.

## Explicit one-time old-monolith migration — rehearsal instructions only

**Approved policy, not executed here and not yet a supported migration claim.**
No formula hook performs migration. Never automatically unlink, overwrite,
remove a keg, kill a process, or mutate configuration/credentials. Do not use
`brew link --overwrite`, `brew uninstall --force` or destructive cleanup.
These steps are for a separately authorized **disposable macOS arm64 fixture**
with isolated Homebrew prefix/HOME/state, not commands to run on the host.

1. Historical distribution is established by the prior read-only API audit:
   the official `cercano-ai/homebrew-cercano` tap distributes the v0.25.2
   monolith, and the personal tap distributes v0.20.3. This is not evidence
   that a particular user installed either version. Record installed-user
   applicability, exact formula/archive/source identity, architecture and restart
   hooks before rehearsal. An invented monolith tests ownership only; it cannot
   substitute for the applicable historical installation.
2. Install the historical local fixture monolith in the disposable tap before
   staging the new definitions. Record `brew list --versions`,
   `brew list fixture/cercano/cercano`, canonical command targets and hashes,
   synthetic state/credential markers and any owned running processes. Save
   original formula/archive and an isolated machine snapshot for rollback.
   Establish the effective persistent `HOMEBREW_NO_CLEANUP_FORMULAE` policy
   retaining **both `cercano` and `cercano-agent`**, preserving other exclusions,
   and capture `brew config` in the actual invocation context. Disable automatic
   cleanup during the migration rehearsal (`HOMEBREW_NO_INSTALL_CLEANUP=1`).
   Do not extend any policy to other users or the developer host.
3. Drain/finish fixture conversations through supported lifecycle operations
   before manual migration. Do not kill hosts/workers or assume an old monolith
   has prepared/admitted generation state. Unprepared state must explicitly
   refuse/defer where required; never fabricate admission or silently migrate
   user data. Snapshot again after the drain. No active-session continuity claim
   is made for this one-time topology transition.
4. Stage all three locally rendered fixture formulas together in the local tap,
   using local artifact URLs as above. Confirm old ownership is actually both
   `bin/cercano` and `bin/cercano-cli`, no unrelated owner exists, and both new
   components are absent. If any precondition differs, **stop and diagnose**.
   Operator explicitly runs `brew unlink fixture/cercano/cercano` once. This
   retains the old keg; record its canonical executable/hash after unlink.
5. Operator runs `brew install --formula fixture/cercano/cercano-agent`, then
   `brew install --formula fixture/cercano/cercano-cli`, stopping on **any**
   failure. Inspect exclusive ownership and resolved paths after each step.
   Only after both succeed, run `brew upgrade --formula fixture/cercano/cercano`
   to replace the monolith definition with the dependency-only convenience keg.
   Run both component `brew test` checks and inspect versions/ownership. The
   convenience keg must own no command; no installation should start an agent.
   Verify synthetic configuration/conversation/credential markers are unchanged.
6. Do not delete the saved monolith keg or snapshot as cleanup. Verify the
   retention policy and record residual kegs/processes for separately authorized
   teardown. Steady-state agent retention still requires the persistent
   `cercano-agent` exclusion in **every** supported upgrade/cleanup context;
   this is not uninstall protection or external-runtime retention.

### Failure/recovery acceptance matrix — all pending native execution

| Scenario | Required observation and operator recovery |
| --- | --- |
| Ordinary dependency-first upgrade without manual migration (separate snapshot) | Capture actual link-conflict/failure behavior. Never overwrite links to make this pass. Restore fixture snapshot before the approved path. |
| Missing CLI artifact/dependency or failed install after unlink/agent install | Stop before convenience upgrade. Record partial kegs/links and unchanged markers. On this pre-convenience path only, explicitly `brew unlink fixture/cercano/cercano-agent` and/or `brew unlink fixture/cercano/cercano-cli` for components actually installed; then `brew link fixture/cercano/cercano` with no overwrite. Verify canonical old paths/hashes and old command smoke. If link fails, stop; restore the disposable snapshot, not arbitrary files. |
| Failure during/after convenience upgrade | Capture state without cleanup. Restore the saved **disposable machine snapshot** (including tap, Cellar, links and isolated state); do not assume `brew link cercano` can select a saved old keg after the convenience replacement. Verify old ownership/hashes and synthetic state after restore. |
| Successful migration then independent component/convenience A/B upgrades | Verify partial upgrade/repository-lag behavior, no double command owner, no automatic agent launch, and preserved state. Installed and session-serving versions are distinct evidence. |
| Cleanup/idle/new-worker/reconnect and incompatible activation | Follow the closeout checklist with prepared/admitted state and unchanged external runtime paths. Retention diagnostics plus canonical hashes and fresh worker identities are required; exit code or one surviving process alone is insufficient. |

Record each scenario as pending/pass/fail/blocked with versions, revisions,
archive hashes, Homebrew/macOS versions, sanitized logs, before/after ownership,
state markers and recovery outcome. Documentation and inert offline tests cannot
mark this matrix passed. Final signed-package and real local inference tests
remain separately authorized release gates.

## Remaining acceptance gates

* Installed-user applicability of the established historical monoliths and the
  manual migration/failure-recovery matrix above; approval of policy is not
  execution evidence.
* Native package install/upgrade/uninstall, partial sets and repository lag for
  the three-formula topology. Existing same-source synthetic rehearsal covers
  only its recorded narrow journey, not these additional scenarios.
* Real inference after idle/later work/restart under the accepted unchanged
  external-runtime boundary, human acceptance, and actual changed-source A/B
  compatibility. No bundled llama closure or external cleanup safety is supplied.
* Final signed/notarized-package clean-Mac smoke and supported platform/trust
  qualification. Then separately authorized coherent publication implementation
  and review; the live/local promotion guards remain unconditional until then.
