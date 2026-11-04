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

## Publication is blocked

The macOS review artifact explicitly includes all three rendered formulas;
this upload manifest change does not enable tap publication. Both
`.github/workflows/release-macos.yml` and `update-homebrew.yml` invoke
`update_tap.py`, whose `update()` now fails closed
**before network access**, even with credentials. `promote_to_tap.py` also refuses
local promotion, including `--skip-download`. Legacy single-formula publication
cannot safely publish this split. Future separately authorized workflow work
must stage/review all three formulas coherently, handle dependency availability
and existing monolith migration, and require full runtime/native acceptance.
Do not dispatch release workflows: the macOS workflow has other release actions
outside this bounded guard. No publication, network release action, credentials,
signing or release-tag change is authorized by this slice.

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

## Unresolved acceptance blockers

* **Old-monolith migration:** the old `cercano` keg owns both command paths.
  New components may encounter link conflicts during dependency-first upgrades.
  No `conflicts_with`, overwrite, unlink, process kill or migration hook is used
  to hide this. Rehearse old monolith -> convenience topology with real Homebrew,
  failure/recovery and preserved user state before choosing migration policy.
* **Runtime closure:** no bundled llama/libraries/resources or full installed
  retention integration is supplied here. Complete per-platform dependency
  inventory and Phases 2–3 must prove later A worker launches after B install and
  `brew cleanup`, compatible admission, incompatible activation deferral and
  shared-data safety. Definition tests cannot establish any of these.
* **Native manager evidence:** actual install/upgrade/uninstall, partial upgrades,
  repository lag, cleanup and migration have not been run in this slice.
* **Distribution trust:** signing/notarization and coordinated publication remain
  separately gated obligations. Unsigned fixtures and checksums do not satisfy them.
