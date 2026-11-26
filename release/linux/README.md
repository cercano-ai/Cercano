# Linux packages

Draft packaging for the experimental, unsigned `linux-x64` and `linux-arm64`
release tarballs (see
[linux-artifact.md](../../docs/linux-artifact.md), checklist item 5), built
with [nfpm](https://github.com/goreleaser/nfpm) in four formats:

| Format | Distributions (PR92-reported tests) |
| --- | --- |
| `.deb` | Ubuntu 24.04, Debian 12 |
| `.rpm` | Fedora 42, Rocky Linux 9, openSUSE Leap 15.6 |
| `.apk` | Alpine 3.20 |
| `.pkg.tar.zst` | Arch Linux |

| Package | Commands | Dependencies |
| --- | --- | --- |
| `cercano-agent` | `/usr/bin/cercano` | none (no CLI dependency) |
| `cercano-cli` | `/usr/bin/cercano-cli` | none (a remote agent stays possible) |
| `cercano` | none | `cercano-agent`, `cercano-cli` (minimum version, not a lock) |

This is the same split as the Homebrew formulas and the Chocolatey packages.
Each component installs only its own executable, byte for byte from the
verified `cercano-<version>-linux-x64.tar.gz` (amd64) or
`cercano-<version>-linux-arm64.tar.gz` (arm64). Both binaries are statically
linked, so the packages have no library dependencies. No local model runtime
is bundled. The package renderer does not sign packages or publish anything. The separate
apt staging helpers sign repository metadata locally; publication remains
unconfigured. See [production gates and key custody](../../docs/linux-package-production.md).

## Build

On Linux (file modes are part of the packages), with nfpm on `PATH`. Tested
with nfpm v2.48.0; `install-nfpm.sh` pins and checks its Linux x86_64 and
arm64 digests.

```bash
python3 release/linux/render_packages.py \
  --version 0.27.0 \
  --archive cercano-0.27.0-linux-x64.tar.gz \
  --sha256 <published sha256> \
  --maintainer "cercano maintainers <maintainers@cercloud.ai>" \
  --output render
mkdir dist
for p in cercano-agent cercano-cli cercano; do
  for f in deb rpm apk archlinux; do
    nfpm package -f "render/$p.yaml" -p "$f" -t dist/
  done
done
```

`render_packages.py` checks the archive's SHA-256 before opening it, then the
exact release layout, the runtime inventory and an ELF64 header for the
selected architecture on both binaries, and writes nothing unless every check
passes. `--arch arm64` reads `cercano-<version>-linux-arm64.tar.gz` and expects
AArch64 binaries; `--without-convenience` leaves out the `cercano` package. It then writes one
nfpm config per package plus the files it ships: the binary, and for `.deb`
a DEP-5 `copyright`, `changelog.gz` and lintian overrides; the other formats
get the release `LICENSE` in `/usr/share/licenses/<package>/`. nfpm only
packages bytes the renderer has already verified. With `SOURCE_DATE_EPOCH`
set (for both steps), every package is byte-identical across builds.

The `.deb` version is the plain release version (`0.27.0`); rpm and Arch add
their required release number (`0.27.0-1`).

Tests: `cd release/linux && python3 -m unittest test_render_packages`. They
use tiny fake ELF fixtures; the nfpm build test runs only where `nfpm` is on
`PATH`. The `linux-packaging` CI job runs these tests, failure-propagation
regressions, all-format inert-fixture builds and a disposable-key local apt
rehearsal without secrets. It does not prove real-binary lifecycle acceptance.

## Container tests

`tests/run-container-matrix.sh <dist> [<old dist> <new version>]` runs, in
throwaway Docker containers on all seven distributions above:

- `tests/container-lifecycle.sh`: install all three, both commands report the
  version, the agent starts and listens as a normal user, removing `cercano`
  keeps the components, removing those leaves no `cercano` files.
- `tests/container-upgrade.sh` (with an old set): install the old set; the
  convenience package alone is refused while the components are older; all
  three upgrade together.

Historical results reported by PR92 (not revalidated by this integration):
for v0.27.0, and an upgrade from 0.25.2 (rendered with a test-only
harness, since its tarball predates the runtime inventory): all pass.
Additionally, on Ubuntu and Debian: the agent survives reinstall or removal
while running (Linux can replace a running executable; nothing is stopped or
restarted, matching the Homebrew formulas), a new agent starts on an older
version's `~/.config/cercano`, and a downgrade needs `--allow-downgrades`.
`~/.config/cercano` is user data and is kept on removal.

arm64: the release workflow builds a `linux-arm64` tarball and its
`cercano-agent` and `cercano-cli` packages natively on an arm64 runner
(`package-verified.sh ... arm64`). The architecture-independent `cercano`
package comes only from the amd64 build, and `merge-review-packages.sh`
refuses an arm64 set that contains it. arm64 deb, rpm and apk packages passed
the lifecycle test under QEMU on Ubuntu, Fedora and Alpine (PR92), and arm64
debs built from v0.28.1 installed from a disposable apt repository on a
Raspberry Pi 4 (Debian 13).

## apt repository

`build-apt-repo.sh <site> <debs> <key id>` adds `.deb` files to a static,
signed apt repository (suite `stable`, component `main`) for GitHub Pages or
any static host. It needs `apt-ftparchive` and `gpg` with the repository's
private key in `GNUPGHOME`; it never creates a key, and it refuses to replace
a published file with different bytes. Users then run:

```bash
curl -fsSL https://cercano-ai.github.io/packages/cercano.gpg | sudo tee /etc/apt/keyrings/cercano.gpg >/dev/null
echo "deb [signed-by=/etc/apt/keyrings/cercano.gpg] https://cercano-ai.github.io/packages/ stable main" \
  | sudo tee /etc/apt/sources.list.d/cercano.list
sudo apt update && sudo apt install cercano
```

PR92 reported local tests with a throwaway key, served over HTTP to Ubuntu 24.04 and
Debian 12 containers: the user steps install `cercano` (apt pulls in both
components), publishing a newer version reaches users through `apt upgrade`,
and apt refuses an `InRelease` altered after signing (`BADSIG`), an altered
package index (hash mismatch) and a repository signed by another key
(`NO_PUBKEY`).

The live repository is published by `cercano-ai/packages` on GitHub Pages at
`https://cercano-ai.github.io/packages/` (first release 0.28.1, 2026-11-26).
Verify the primary key fingerprint before installing, as in the top-level
README. This repository adds no publication job or credentials. Signing must
stay outside the unsigned `build-linux` job. The offline
primary / CI signing-subkey custody model and remaining provisioning gates are
in [production setup](../../docs/linux-package-production.md).

## lintian

Clean apart from these, accepted on purpose (overrides are shipped in
`/usr/share/lintian/overrides/`):

- `statically-linked-binary`, `unstripped-binary-or-object`: the binaries are
  the release's own Go builds, shipped unmodified so they match the published
  checksum.
- `empty-binary-package`: `cercano` is a dependency-only convenience package.

Remaining warnings: `copyright-without-copyright-notice` (needs a copyright
year and holder; the file says "Cercano contributors") and `no-manual-page`.

## Open decisions

1. **Maintainer**: the `Maintainer` field needs a real name and email.
2. **Repository signing key**: who generates and holds it, how it is stored
   as a release secret, and how it would be rotated. Users trust it once.
3. **Repository URL**: `https://cercano-ai.github.io/<repo>/` or a project
   domain. It ends up in every user's apt sources, so it should not change.
4. **rpm repository**: the same Pages site could host a dnf repository
   (`createrepo_c`); not built yet. Alpine and Arch would use release
   downloads, or an AUR package for Arch.
5. **Headless keyring**: credential storage needs Secret Service or KWallet,
   which servers and containers usually lack (linux-artifact.md, item 4). The
   package description says so; packaging can't fix it.
6. **Mixed versions**: a newer agent with an older CLI (or the reverse) is
   installable. Whether the components should require matching versions
   depends on the agent/CLI compatibility promise.
7. **Directory name**: `efforts/cross-platform-updates/plan.md` names
   `release/debian/`; this is `release/linux/` because it covers all four
   formats.
