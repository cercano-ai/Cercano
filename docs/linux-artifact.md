# Linux artifact (experimental, unsigned)

Status: **Experimental, explicitly unsigned, not a supported release.**

Cercano's supported platform today is macOS (Apple Silicon), distributed as a
signed and notarized tarball plus a Homebrew formula via
[release-macos.yml](../.github/workflows/release-macos.yml) — see
[macos-release-build.md](macos-release-build.md). For Linux there is a single
bounded deliverable: manual, opt-in **unsigned** `linux-x64` and `linux-arm64`
tarballs produced by the same manual release pipeline. The tarballs are
unsigned, and there is **no claim of full Linux support yet**. To install with
a package manager, see [Install with apt](#install-with-apt). A supported Linux release
is a possible future deliverable; this artifact is its first
build-and-smoke-test milestone, not a support commitment.

## Install with apt

The `.deb` packages from this pipeline are published separately, from
[cercano-ai/packages](https://github.com/cercano-ai/packages), to an apt
repository for **amd64 and arm64**. This is still experimental. The apt
repository can be ahead of the latest GitHub release and the other install
channels.

Tested with 0.28.2:

- **amd64:** Debian 12 and 13 and Ubuntu 22.04 and 24.04 containers before
  publication. From the live repository, install, upgrade and reinstall on
  Debian 12 and Ubuntu 24.04.
- **arm64:** the same four distributions before publication, under QEMU
  emulation only. **Not yet tested on real arm64 hardware.**

Download the repository key and show its fingerprint:

```sh
sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg
key="$(mktemp)"
curl -fsSL https://cercano-ai.github.io/packages/cercano.gpg -o "$key"
gpg --show-keys --with-fingerprint "$key"
```

Continue **only** if the primary key (`pub`) fingerprint is exactly:

```
5536 8AC6 14FE 6CD9 DAC0  4A45 8A03 FD1D 0192 F44A
```

Then add the key and repository, and install:

```sh
sudo install -d -m 0755 /etc/apt/keyrings
sudo install -m 0644 "$key" /etc/apt/keyrings/cercano.gpg
echo "deb [signed-by=/etc/apt/keyrings/cercano.gpg] https://cercano-ai.github.io/packages/ stable main" |
  sudo tee /etc/apt/sources.list.d/cercano.list
sudo apt-get update
sudo apt-get install -y cercano
```

Run `cercano-cli` as your normal user. The headless keyring caveat below
applies to packaged installs too.

To upgrade, close Cercano first, then:

```sh
sudo apt-get update
sudo apt-get install --only-upgrade -y cercano cercano-agent cercano-cli
```

To remove the packages:

```sh
sudo apt-get purge -y cercano cercano-agent cercano-cli
```

Per-user configuration and conversations in your home directory are not
removed.

## What is produced

`cercano-<version>-linux-x64.tar.gz` (x86-64) and
`cercano-<version>-linux-arm64.tar.gz` (AArch64), each with its `.sha256`
checksum, attached to the GitHub release when `publish=true` (the default;
`false` is an optional dry run). Each has this layout (`linux-arm64` in place
of `linux-x64`):

```
cercano-<version>-linux-x64/
├── bin/
│   ├── cercano      # agent entrypoint
│   └── cercano-cli  # terminal client
├── LICENSE
└── README.txt       # restates the unsigned/experimental status
```

Both binaries are built from the same validated source (a `v<version>` tag
matching HEAD) with `CGO_ENABLED=0`, producing statically linked ELF64
little-endian executables: natively on an `ubuntu-24.04` runner with
`GOOS=linux GOARCH=amd64` (x86-64), and natively on an `ubuntu-24.04-arm`
runner with `GOOS=linux GOARCH=arm64` (AArch64). CGO is not required: the SQLite driver is
`modernc.org/sqlite` (pure Go), and no reachable dependency needs a C
toolchain. This was verified by cross-compiling both binaries for
`linux/amd64` on macOS before the pipeline was written, and confirmed again
on the real cross-build (`file`: "ELF 64-bit LSB executable, x86-64,
statically linked"). The arm64 build was first checked natively on a
Raspberry Pi 4 (Debian 13 arm64) at `v0.28.1`: the archive, all twelve
packages and an apt install from a disposable test repository. `os/user` is
resolved by Go's pure-Go `/etc/passwd` fallback when CGO is disabled.

**Runtime caveat that matters for support:** the credential keyring
(`99designs/keyring`) speaks Linux Secret Service/KWallet over D-Bus from
pure Go. On a desktop session with `gnome-keyring` this works; on a headless
server or container with no keyring service, credential storage will fail.
That difference — and the lack of any distribution testing across distros —
is a core reason Linux is **not** a supported platform yet.

## What the pipeline verifies (and what it does not)

The `build-linux` (amd64) and `build-linux-arm64` jobs of
[release-macos.yml](../.github/workflows/release-macos.yml) each:

- runs the release test gates first, before producing anything;
- refuses unstable versions (`X.Y.Z` only, no leading `v`) and a tag that does
  not match HEAD;
- builds both `cercano` and `cercano-cli` from the same version;
- requires an executable ELF64 little-endian header for its own architecture
  (x86-64 or AArch64), rejecting non-ELF, 32-bit, big-endian and the other
  architecture;
- verifies the tarball layout and contents, and that the binaries keep
  executable (0755) permissions while `LICENSE`/`README.txt` stay 0644;
- extracts the exact tarball contents and smoke-tests **both** binaries' exact
  `--version` output on the Linux host, with `HOME` and every `XDG_*`
  directory pointed at throwaway paths under `RUNNER_TEMP` so no real user
  state is read or written (stray writes fail the job);
- runs the Linux-safe hermetic native tests (`procx` unix process-group
  tests, which spawn and reap throwaway `/bin/sh` children only);
- publishes with `publish=true` (the default), after **every** platform
  verification gate (macOS, Windows, Linux amd64 and arm64) passes, verifies
  each Linux checksum against the digest its build recorded, and refuses to
  replace any already published asset.

The smoke test exercises **only** `--version` on both binaries. It does not
launch the agent, does not use provider credentials, and does not download
models or any third-party runtime. The archive is explicitly unsigned; there
is no GPG or other signing step.

### Scope limits of the native tests

Only genuinely hermetic tests run on the runner today: `procx`'s
build-tagged (`unix`) process-group tests. The rest of the Linux-relevant
surface (keyring via Secret Service, `sysram`/memory detection on Linux, the
local llama-server runtime, glibc-vs-musl differences) does not yet have
dedicated Linux-host test coverage; `sysram` and `pkg/config/memory` carry
`linux` build-tagged source with no Linux-host tests. Extending Linux host
coverage is part of the support checklist below.

## Safety invariants

- The `build-linux`, `build-linux-arm64` and `linux-packages-review` jobs
  hold **no secrets and no `environment`**: no signing credentials of any kind
  reach them. There is no Linux signing at all — see
  the checklist.
- The workflow remains manual-dispatch only; there is no push/tag/PR trigger,
  so no untrusted code can start it.
- Publication defaults to `publish=true` in the same run, guarded by
  the protected `release` environment, and it never overwrites an existing
  asset.
- Existing releases are untouched by this pipeline; a Linux asset is only
  added for **new** versions.

## Supporting Linux: pending checklist

This artifact is a stepping stone. A *supported* Linux release still owes:

1. **Code signing and provenance** — decide whether to adopt GPG, Sigstore or
   SLSA provenance attestation, in a protected environment.
2. **Linux test coverage on CI** — run the full `go test ./...` suite on a
   Linux host in CI, not just the hermetic helpers, and fix the fallout
   rather than skipping it.
3. **Distribution matrix** — validate against a real set of distros
   (Ubuntu, Debian, Fedora, Arch), glibc versions and libc-less containers.
4. **Keyring behavior** — verify Secret Service/KWallet storage on desktop
   sessions and document the headless-server story (or provide a fallback).
5. **Distribution & install story** — `.deb` packages and a signed apt
   repository are published (see [Install with apt](#install-with-apt)). Still
   open: RPM/`dnf`, and downgrade documentation.
6. **Runtime verification** — the agent's process management, local model
   runtime, and GPU/CPU detection verified end-to-end on Linux.
7. **Support policy** — issue triage for Linux reports, and an explicit
   statement of what "supported" means.

Nothing in this experimental artifact should be interpreted as progress on
items 1–7 being complete.

## Reproducing the artifact locally

```sh
# A real tag must match HEAD; the escape hatch is for local dry runs only.
CERCANO_ALLOW_UNTAGGED=1 \
  python3 scripts/build-linux-release.py 0.0.1 dist
```

`scripts/test-linux-release-build.py` covers the failure paths (bad/untagged
version, tag mismatch, failed build, wrong architecture, wrong ELF class or
endianness, non-ELF binary, existing artifact) — a refused or failed build
never leaves a tarball behind. The test fixtures are portable Python
subprocess fixtures that write inert binaries with proper ELF64 little-endian
x86-64 or AArch64 headers; no Go compilation is needed to run them. Pass
`arm64` as a third argument to build the `linux-arm64` tarball.

## Verification recorded for this change

Both real Go entrypoints cross-compiled successfully on macOS with
`GOOS=linux GOARCH=amd64 CGO_ENABLED=0` into the local-only `0.0.1`
rehearsal tarball (that version was used only as build metadata: no tag or
release was created). Independent `file`/`tar`/`shasum` checks confirmed
statically linked ELF64 LSB x86-64 binaries, the expected archive layout and
executable permissions, and a matching checksum. The builder tests and
workflow structure tests passed. **Native Linux execution was not available
locally and was not performed**; cross-compilation alone does not validate
runtime behavior. The first real execution check belongs to the
`build-linux` CI job's smoke test.

Before claiming a supported Linux release, also verify installed binary
lookup outside a checkout, agent singleton/reconnection behavior, credentials
in the desktop keyring and the headless failure mode, model/runtime download
and execution, Unicode and space-containing paths, upgrade preservation of
conversations/configuration, and uninstall semantics. These are required
follow-up acceptance checks, not optional polish on the experimental artifact.
