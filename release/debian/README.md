# Debian packages

Draft `.deb` packaging for the experimental, unsigned `linux-x64` release
tarball (see [linux-artifact.md](../../docs/linux-artifact.md), checklist
item 5). Nothing here signs, publishes or hosts an apt repository; those are
open decisions, listed below.

| Package | Commands | Dependencies |
| --- | --- | --- |
| `cercano-agent` | `/usr/bin/cercano` | none (no CLI dependency) |
| `cercano-cli` | `/usr/bin/cercano-cli` | none (a remote agent stays possible) |
| `cercano` | none | `cercano-agent`, `cercano-cli` (minimum version, not a lock) |

This is the same split as the Homebrew formulas and the Chocolatey packages.
Each component installs only its own executable, byte for byte from the
verified `cercano-<version>-linux-x64.tar.gz`. Both binaries are statically
linked, so the packages have no library dependencies. No local model runtime
is bundled.

## Build

On Linux (file modes are part of the package, so not on a Windows mount):

```bash
python3 release/debian/render_packages.py \
  --version 0.27.0 \
  --archive cercano-0.27.0-linux-x64.tar.gz \
  --sha256 <published sha256> \
  --maintainer "Name <email>" \
  --output pkgs
for p in cercano-agent cercano-cli cercano; do
  dpkg-deb --root-owner-group -Zxz --build "pkgs/$p" .
done
```

`render_packages.py` checks the archive's SHA-256 before opening it, then the
exact release layout, the runtime inventory and an x86-64 ELF64 header on both
binaries, and writes nothing unless every check passes. With
`SOURCE_DATE_EPOCH` set (for both steps), the `.deb` files are byte-identical
across builds.

Tests: `cd release/debian && python3 -m unittest test_render_packages`. They
use tiny fake ELF fixtures; the `dpkg-deb` test runs only where `dpkg-deb` is
installed. Like the Chocolatey tests, they are not wired into CI yet.

## Behaviour (tested on Ubuntu 24.04 and Debian 12 containers, v0.27.0)

- Installing all three: both commands report the version, and the agent starts
  and listens as a normal user.
- Reinstalling or removing `cercano-agent` while the agent runs succeeds; the
  running agent keeps running from the old, now unlinked, binary until it
  exits. Unlike the Chocolatey packages, there is no refusal: Linux can
  replace or remove a running executable safely. Nothing is stopped or
  restarted, matching the Homebrew formulas.
- Removing `cercano-agent` also removes `cercano`, which depends on it.
- Removal leaves no `cercano*` commands or docs behind; `~/.config/cercano` is
  user data and is kept.

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
2. **Distribution**: an apt repository (where hosted, who holds the signing
   key, how users add it) or `.deb` files attached to GitHub releases. Without a
   repository, `apt install ./cercano_<v>_all.deb` cannot resolve its
   dependencies, so users install all three files together.
3. **rpm**: Fedora and similar distributions are not covered.
4. **Headless keyring**: credential storage needs Secret Service or KWallet,
   which servers and containers usually lack (linux-artifact.md, item 4). The
   package description says so; the packaging can't fix it.
5. **Upgrade testing**: only one version exists, so upgrades between real
   versions are untested.
