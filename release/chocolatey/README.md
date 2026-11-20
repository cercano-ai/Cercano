# Chocolatey packages

All three v0.27.0 packages were submitted successfully; this is not approval.
Last known: agent/CLI Pending automated review, convenience package waiting on
dependencies. Public package-page rechecks on 2026-11-15 returned HTTP 403, so
current approval/default-source availability remains unconfirmed. See the
[Windows installation guide](../../docs/installation-windows.md) for package
links, install commands, and dependency-aware upgrades. Windows remains
experimental and unsigned.

Windows phase 2 package definitions for `efforts/cross-platform-updates/plan.md`.
The rendered packages are written to pass the community repository's
package validator; publishing them (account, API key, `choco push`) and
code signing are separate, manual steps. Nothing here signs or publishes.
The Phase 1 evidence cited below (`native-go-next-results.md`,
`windows-transaction-audit.md`, the effort handoff) is under
`efforts/cross-platform-updates/` on branch `handoff/windows-replacement-sanitized`.

| Package | Commands (shims) | Dependencies |
| --- | --- | --- |
| `cercano-agent` | `cercano` | none (no CLI dependency) |
| `cercano-cli` | `cercano-cli` | none (a remote agent stays possible) |
| `cercano` | none | `cercano-agent`, `cercano-cli` (minimum version, not a lock) |

Each component embeds only its own executable from the verified
`cercano-<version>-windows-x64.zip`, so Chocolatey auto-creates exactly one shim
per component. Embedding (rather than downloading in the install script) keeps
installs offline-verifiable and means the install script does no acquisition.
`VERIFICATION.txt` records the archive URL, the archive SHA-256 and the
executable SHA-256. The ZIP has **no bundled llama/model runtime**, so neither
component claims local-runtime support.

## Running-process handling (what Phase 1 supports, and only that)

`guard.ps1` is copied into each component as `tools\cercano-guard.ps1`.

- `chocolateyInstall.ps1` throws if any process runs from this package's
  `lib`, `lib-bkp` or `lib-bad` directory, or from its shim in `bin`. In the
  Phase 1 ARM64 fixture this was the path Chocolatey honored: the upgrade
  failed (-1), Chocolatey rolled back, and an ordinary retry after exit
  succeeded (`native-go-next-results.md`).
- `chocolateyUninstall.ps1` throws the same way. Without it, uninstall with
  the agent running from `lib` reported success, removed the package and
  shim, and stranded the running executable in `lib-bkp` (measured, see
  below). Chocolatey removes shims before this script, so after a refusal the
  shim can be missing until the retry; the package stays.
- `chocolateyBeforeModify.ps1` only **warns**. A throw there is not a veto
  and can skip the backup that rollback needs (`windows-transaction-audit.md`).
- A process with the same file name whose location Windows will not reveal
  (typically another user's, seen without elevation) is treated as running:
  fail closed, ask for exit or an elevated retry.
- Nothing is stopped, started or restarted, and no user data is read.

Known gaps, deliberately not papered over:

- Snapshot only: a launch after the check is not caught (concurrent launch is
  unqualified and its probe is blocked, see the effort handoff).
- A launch through the `cercano` shim leaves the shim process alive as the
  agent's parent (measured). The guard reports both. Phase 3 may want the CLI
  to launch the shim's target directly.
- `cercano`'s dependencies are minimums, and Chocolatey resolves them to the
  **highest** available version: `choco install cercano --version 0.0.1`
  installed 0.0.2 components when the feed had both (measured). Install
  components explicitly to pin them.
- Quitting `cercano-cli` leaves the agent (and its workers) running by
  design, and there is no stop command, so "exit them" in the refusal means
  Task Manager today (measured on a real install, below).
- Not covered: cross-user processes, concurrent launch, interrupted installs,
  repository lag.

## Native x64 lifecycle (private Chocolatey root)

`tests/private-root-lifecycle.ps1` copies Chocolatey's program files into a
new folder (path with spaces and non-ASCII on purpose), points
`ChocolateyInstall` at it for that process only, and uses a local feed with
0.0.1 and 0.0.2 built by `build-windows-release.py`. The machine's own
Chocolatey and any installed Cercano are not used; `cercano acp` (idle on an
open stdin, throwaway profile folders) is the running agent, and closing its
stdin is the stop.

2026-11-10, Windows 11 Pro 26300 x64, pwsh 7.6.9, not elevated:
**22 of 22 checks passed on Chocolatey 2.2.2 (installed) and on 2.7.5**
(program files from the official `chocolatey.2.7.5.nupkg`, SHA-256
`f86f99a1...aa5839`, matching the Phase 1 provenance). The 2.7.5 run used the
final guard (canonical paths, name-filtered); the guard's own tests include
junction and 8.3 forms of `ChocolateyInstall`, which the first version missed.

| Case | Result |
| --- | --- |
| Install components at 0.0.1, `cercano` on top; closed upgrade to 0.0.2 | exit 0; exactly two shims; shims report the installed version |
| Agent upgrade while running (via shim, and directly from `lib`) | refused, exit -1; rolled back to 0.0.1; process untouched; retry after exit succeeds |
| CLI upgrade while the agent runs | allowed, exit 0 (components independent) |
| Payload held open (share-read) | exit 1, stays 0.0.1; retry after release succeeds |
| Uninstall while running (via shim, and directly from `lib`) | refused (exit 1 / -1), package kept; retry after exit succeeds, nothing left in `lib-bkp` |
| `cercano-cli` alone | installs without the agent; no `cercano` shim |
| Final uninstall | package and shims gone; synthetic profile file unchanged |

Non-elevated choco waits 20 s on a "continue?" prompt each call, then continues.

## Real install (second machine)

2026-11-10, Windows 11 x64 laptop with no prior Cercano, Chocolatey 2.7.5
installed with the official command, elevated:

- `tests/private-root-lifecycle.ps1 -UseInstalledChocolatey` against the real
  `C:\ProgramData\chocolatey`: **22 of 22 checks passed**.
- Then a real install of current `main` plus the Windows config fix
  (`fix/windows-config-link-count`), labelled 0.26.1 locally. From a normal
  terminal, `cercano-cli` found `cercano` on PATH and started the agent
  through the shim (CLI shim, CLI, agent shim, agent, workers), setup with a
  subscription sign-in applied, and a message was answered.
- Without the config fix, setup failed on apply with "configuration authority
  must have one link" on every Windows install.
- `choco upgrade cercano` with the CLI and agent running: both components
  refused with the process list, nothing changed, `cercano` failed on its
  dependency. After the processes were stopped, the upgrade went through 3/3.

## Community repository (package validator)

Checked against the rule docs (CPMR0001-0076) in `chocolatey/docs`, and
enforced by `test_render_packages.py` where testable:

- Requirements met: `LICENSE.txt` and `VERIFICATION.txt` beside the embedded
  binary (CPMR0005/0006), `projectUrl`, `licenseUrl` with no license
  acceptance, description, tags without commas, no templated values.
- Packaged scripts have their comments stripped, keeping one header line
  linking to the commented source here (CPMR0022), and the renderer refuses
  `cinst`, `choco install` or `choco upgrade` anywhere in them (CPMR0010).
- Guidelines met: `title`, `summary`, `packageSourceUrl`, `iconUrl` (the
  release tag's `website/app/icon.svg` through jsDelivr, not raw GitHub,
  CPMR0076), `docsUrl`, `bugTrackerUrl`, `releaseNotes`, versioned
  dependencies.
- All three generated packages declare `Copyright Cercloud` and bundle the
  upstream `LICENSE` bytes unchanged as `tools/LICENSE.txt`, including the
  binaryless convenience package. This source fix applies to the next release;
  the submitted v0.27.0 packages are unchanged.
- Left out on purpose: `projectSourceUrl` (would equal `projectUrl`, CPMR0041).
- Expect moderator notes for embedded binaries (CPMR0060) and unsigned
  executables. Moderation of a new package is manual.

## Tests and rendering

From the repository root (Windows; the choco and guard tests skip elsewhere):

```sh
python -m unittest discover -s release/chocolatey -p 'test_*.py'
powershell -NoProfile -ExecutionPolicy Bypass -File release/chocolatey/test_guard.ps1
```

`test_guard.ps1` uses a throwaway fake Chocolatey root in TEMP and copies of
`ping.exe` as fixture processes; it never touches the real installation.

Render from a local release ZIP and its independently trusted SHA-256, then pack:

```sh
python release/chocolatey/render_packages.py --version 1.2.3 \
  --archive dist/cercano-1.2.3-windows-x64.zip \
  --sha256 "$TRUSTED_SHA256" --output dist/choco
choco pack dist/choco/cercano-agent/cercano-agent.nuspec --out dist/choco
```

## Release CI review packages (not submission)

The Windows job in `.github/workflows/release-macos.yml` now wires the existing
renderer and native pack/guard tests into release CI. Previously it uploaded
only the Windows ZIP and its checksum; the renderer, optional local pack test,
and lifecycle tooling existed but release CI did not create Chocolatey packages.

After the tag/HEAD gate, build, archive smoke test and native helper checks,
`pack_packages.py` consumes **that run's exact ZIP path and recorded SHA-256**
from the build step outputs. The renderer verifies the digest, runtime inventory,
layout and PE headers; no release download or rebuild supplies package payloads.
The wrapper requires `choco`, packs all three nuspecs, then opens all three
`.nupkg` files and checks rendered metadata (including copyright), dependency
IDs/minimum versions, exact tools manifest and every payload byte. License bytes
must also match the checkout's upstream `LICENSE`; `VERIFICATION.txt` and scripts
must match the verified rendered sources. Unexpected package members fail closed.
Only after every inspection passes are per-package SHA-256 sidecars emitted.
Existing output directories are refused and failed runs leave no partial output.

The separate `cercano-<version>-chocolatey` Actions review artifact contains the
three `.nupkg` files and three `.nupkg.sha256` files (14-day retention). It is
produced with either `publish=false` or `publish=true`, but is **not** part of
the GitHub Release asset upload and is never pushed/submitted to Chocolatey.
No new secrets, signing, install/upgrade operations or publishing access are used.
The release's other-platform and ZIP publication behavior is unchanged.

The job requires both `choco` and Windows PowerShell before running the isolated
Python renderer/packing tests and `test_guard.ps1` (throwaway root and fixture
processes). Native test prerequisites cannot silently skip on CI. The private-root
lifecycle script remains a separate manual qualification: it needs a two-version
0.0.1/0.0.2 local feed, installs/upgrades packages and launches real agents; it is
not run against the one-version release set and is not claimed as release CI coverage.

**Verification status:** this CI wiring has been tested structurally on macOS,
with portable renderer tests and simulated pack-output inspection tests. The
native `choco pack` and Windows PowerShell guard tests skip on macOS. The updated
Windows CI job has **not yet been executed**; native packing success, package
approval and default-source availability are not established by this change.

Local equivalent (Windows, from a fresh output directory):

```sh
python release/chocolatey/pack_packages.py --version 1.2.3 \
  --archive dist/cercano-1.2.3-windows-x64.zip \
  --sha256 "$TRUSTED_SHA256" --output dist/chocolatey
```
