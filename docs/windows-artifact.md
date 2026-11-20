# Windows artifact (experimental, unsigned)

Status: **Experimental, explicitly unsigned, not a supported release.**

Cercano's supported platform today is macOS (Apple Silicon), distributed as a
signed and notarized tarball plus a Homebrew formula via
[release-macos.yml](../.github/workflows/release-macos.yml) — see
[macos-release-build.md](macos-release-build.md). For Windows there is a
manual, opt-in **unsigned** `windows-x64` ZIP produced by the same manual
release pipeline. Chocolatey v0.27.0 packages have also been submitted, with
moderation/availability not confirmed; see the [Windows installation guide](installation-windows.md)
for status, commands, and upgrade precautions. There is no signing or claim
of full Windows support. The artifact details below describe the ZIP pipeline,
not Chocolatey approval or complete runtime acceptance.

## What is produced

`cercano-<version>-windows-x64.zip` and its `.sha256` checksum, attached to the
GitHub release when `publish=true` (the default; `false` is an optional dry run).

```
cercano-<version>-windows-x64/
├── bin/
│   ├── cercano.exe      # agent entrypoint
│   └── cercano-cli.exe  # terminal client
├── LICENSE
└── README.txt           # restates the unsigned/experimental status
```

Both binaries are built from the same validated source (a `v<version>` tag
matching HEAD), on a `windows-latest` runner, with `CGO_ENABLED=0` and
`GOOS=windows GOARCH=amd64`. CGO is not required: the SQLite driver is
`modernc.org/sqlite` (pure Go), the credential keyring uses `wincred` through
`99designs/keyring` (pure Go), and no reachable dependency needs a cross
toolchain. This was verified by cross-compiling both binaries for
`windows/amd64` before the pipeline was written.

## What the pipeline verifies (and what it does not)

The `build-windows` job of [release-macos.yml](../.github/workflows/release-macos.yml):

- runs the release test gates first, before producing anything;
- refuses unstable versions (`X.Y.Z` only, no leading `v`) and a tag that does
  not match HEAD;
- builds both `cercano.exe` and `cercano-cli.exe` from the same version;
- requires an executable AMD64 PE32+ header, rejecting non-PE and other architectures;
- extracts the exact ZIP contents and smoke-tests **both** binaries' exact
  `--version` output on the Windows host, with `HOME`, `USERPROFILE`,
  `APPDATA`, `LOCALAPPDATA`, `TEMP`/`TMP` pointed at throwaway paths under
  `RUNNER_TEMP` so no real user state is read or written (stray writes fail
  the job);
- runs the Windows-native, hermetic helper tests
  (`localruntime/llamaserver` registry/PATH unit tests);
- verifies the archive layout and integrity before upload;
- renders from that same ZIP and recorded checksum, uses native `choco pack`
  for `cercano-agent`, `cercano-cli` and `cercano`, inspects packed metadata,
  licenses, payload and dependencies, and uploads packages/checksums as a
  **separate Actions review artifact** even with `publish=false` (never
  Chocolatey submission or GitHub Release package assets). This new wiring has
  portable/structural macOS verification only; the updated Windows CI job has
  **not yet been executed**. See the [Chocolatey CI details](../release/chocolatey/README.md#release-ci-review-packages-not-submission);
- publishes with `publish=true` (the default), after **all three**
  platform verification gates (macOS, Windows, Linux) pass, and refuses to replace any already
  published asset (macOS, Windows or Linux).

The smoke test exercises **only** `--version` on both binaries. It does not
launch the agent, does not use provider credentials, and does not download
models or any third-party runtime. The archive is explicitly unsigned; Windows
will show SmartScreen/"unknown publisher" warnings, which is expected.

### Scope limits of the native tests

Only the genuinely Windows-native, hermetic tests run on the runner today.
`localruntime/llamaserver` carries build-tagged (`windows`) unit tests for
PATH merging and registry reading. The rest of the Windows-relevant surface
(`localruntime` path handling, `procx` process-group management on Windows,
keyring via `wincred`) does not yet have dedicated Windows-host test coverage;
`procx` has process-group tests that run on macOS/Linux. Extending Windows
host coverage is part of the support checklist below.

## Safety invariants

- The `build-windows` job holds **no secrets and no `environment`**: no
  signing credentials of any kind, macOS or Windows, reach it. There is no
  Windows signing at all — see the checklist.
- The workflow remains manual-dispatch only; there is no push/tag/PR trigger,
  so no untrusted code can start it.
- Publication defaults to `publish=true` in the same run, guarded by
  the protected `release` environment, and it never overwrites an existing
  asset.
- The successful, immutable `v0.20.0` macOS release is untouched by this
  pipeline; a Windows asset is only added for **new** versions.

## Supporting Windows: pending checklist

This artifact is a stepping stone. A *supported* Windows release still owes:

1. **Code signing** — an EV or standard Authenticode certificate and a
   signing step (e.g. `signtool`/`azure.trust-signing` or equivalent),
   hardware-protected, in a protected environment. Without it SmartScreen
   warnings remain.
2. **Windows test coverage on CI** — run the full `go test ./...` suite on a
   Windows host in CI, not just the hermetic helpers, and fix the
   (so far unobserved) fallout rather than skipping it.
3. **Documented OS support floor** — which Windows 10/11 builds are supported,
   validated on a real machine.
4. **Distribution & install story** — complete Chocolatey moderation and
   release/migration acceptance; submission alone does not establish availability
   or a supported lifecycle. See the [installation guide](installation-windows.md).
5. **Runtime verification** — the agent's process management (`procx`),
   local model runtime (`llamaserver`), and keyring behavior verified
   end-to-end on Windows, including long-running processes and job objects.
6. **Support policy** — issue triage for Windows reports, and an explicit
   statement of what "supported" means (crashes vs. UX parity).

Nothing in this experimental artifact should be interpreted as progress on
items 1–6 being complete.

## Reproducing the artifact locally

```sh
# A real tag must match HEAD; the escape hatch is for local dry runs only.
CERCANO_ALLOW_UNTAGGED=1 \
  python3 scripts/build-windows-release.py 0.20.0 dist
```

`scripts/test-windows-release-build.py` covers the failure paths (bad/untagged
version, tag mismatch, failed build, non-Windows binary, existing artifact) —
a refused or failed build never leaves a ZIP behind.

## Verification recorded for this change

Both real Go entrypoints cross-compiled successfully on macOS into the local-only
`0.20.1` rehearsal ZIP. That version was used only as build metadata: no tag or
release was created. The builder tests and workflow structure tests passed.
The build fixtures are portable Python subprocess fixtures, not POSIX-only
shell scripts. Native Windows execution is still pending the first Actions
run; cross-compilation alone does not validate runtime behavior.

Before claiming a supported Windows release, also verify installed binary
lookup outside a checkout, agent singleton/reconnection behavior, credentials
in Windows Credential Manager, model/runtime download and execution, Unicode
and space-containing paths, upgrade preservation of conversations/configuration,
and uninstall semantics. These are required follow-up acceptance checks, not
optional polish on the experimental artifact.
