# Linux package staging and production gates

## Status and approved destination

Linux remains experimental. The apt repository is live: the separate
`cercano-ai/packages` repository publishes it with GitHub Pages at
`https://cercano-ai.github.io/packages/` (first release 0.28.1, 2026-11-26;
x86_64/amd64 only). The public package maintainer is
`cercano maintainers <maintainers@cercloud.ai>`.

**Publication happens in `cercano-ai/packages`, not in this repository.** Its
`apt-pages.yml` workflow promotes one reviewed `linux-packages-review` artifact
from a successful release run, behind protected environments. This repository
adds no Pages job, production key, secret or token. Existing macOS/Windows/Linux
archive publication policy is unchanged; Linux packages here are review-only
workflow artifacts, not release assets.

## Local and CI staging (no signing credentials)

`build-linux` remains outside the protected release environment, with read-only
contents permission and no secrets. After the existing release archive is built
and smoke-tested, `release/linux/package-verified.sh VERSION ARCHIVE SHA256 OUT`
verifies the archive digest/layout/runtime inventory and renders the approved
maintainer into all twelve packages. It refuses an existing output directory.
The workflow uploads these packages and their `SHA256SUMS` for review only.
The digest is the checksum produced alongside that job's archive; this is not
independent provenance attestation or authentication of a downloaded release.

`release/linux/install-nfpm.sh TOOL_DIR` installs only into a caller-owned
location on Linux x86_64. nfpm v2.48.0 is pinned by the SHA-256 of its upstream
Linux x86_64 archive; verification happens before extraction/execution. Updating
that version or digest requires review against the upstream release. New CI
checkout/upload actions are pinned to full commits. CI's apt-utils/GnuPG are
Ubuntu runner packages installed explicitly through the runner's signed apt
repositories, not arbitrary downloaded executables.

The `linux-packaging` CI job runs renderer/script regression tests and uses
**inert ELF fixture bytes**, not runnable application binaries, to build all
formats. `tests/rehearse-apt.sh DEB_DIR` is a local-only disposable rehearsal:

- Creates a temporary mode-700 GNUPGHOME, a disposable certification primary
  and a disposable signing subkey; never touches the user's normal keyring.
- Exports only the disposable secret subkey to a file inside that temporary
  home, imports it into the signing wrapper's separate temporary home and
  removes both homes/agents on exit or handled interruption.
- Checks detached and inline signatures with gpgv, apt file-transport trust,
  authentic package download, altered metadata/index/package rejection,
  wrong-signer rejection and immutable pool replacement refusal.
- Does not use networking, sudo, publish, or log/export private keys to evidence
  or source. Missing dependencies produce exit 77 (a skip, not acceptance).

Example on an already provisioned Linux x86_64 test host:

```sh
bash release/linux/install-nfpm.sh "$PWD/.tools"
export PATH="$PWD/.tools:$PATH"
python3 -m unittest discover -s release/linux -p 'test_*.py' -v
python3 release/linux/tests/make-rehearsal-fixture.py /tmp/cercano-fixture-UNIQUE
bash release/linux/tests/rehearse-apt.sh /tmp/cercano-fixture-UNIQUE/packages
```

Use a fresh caller-owned fixture path and remove it afterward. Tools needed by
the rehearsal: GnuPG (gpg/gpgconf/gpgv), apt-utils, apt-get and dpkg-deb. The
inert fixture must **not** be used for lifecycle acceptance or publication.
`tests/run-container-matrix.sh DIST [OLD_DIST NEW_VERSION]` needs Docker and real
verified release packages to test install/start/removal and upgrades across
seven distributions. These are separate gates, not proven by unit tests or by
the inert fixture rehearsal. Matrix/package-manager failures now fail closed.

## Future production custody (documentation only)

Use an **offline certification primary OpenPGP key**, with the approved public
identity above, and a separate signing-capable subkey for CI. Keep primary
secret material offline, outside CI, with an independently protected backup and
revocation certificate. Choose expiry/rotation and incident-response ownership
before provisioning. No production key generation/import was performed for
this integration, and the repository intentionally contains no such commands
or key material.

During a separately authorized offline ceremony:

1. Record and independently verify the full primary and signing-subkey
   fingerprints. Transfer only public material and a subkey-only secret export
   (a primary secret stub, **not** the primary secret) to the authorized CI
   custody path. Do not paste private keys into tickets, logs or source.
2. Agree passphrase custody and runner/environment access. The wrapper supports
   a protected subkey through `CERCANO_GPG_PASSPHRASE_FILE`, an existing private
   file; it never reads a terminal. Missing passphrase fails noninteractively.
   Never use shell tracing around secret provisioning or signing.
3. Restrict any future package-signing/deployment job to protected, reviewed
   release inputs and a separately approved environment. Keep all secret
   references, imports and signing outside `build-linux` and pull-request CI.
4. `sign-apt-repo.sh SITE DEBS SUBKEY_FILE SIGNING_FINGERPRINT` creates a fresh
   mode-700 temporary GNUPGHOME, imports the supplied subkey-only material,
   rejects available primary secret material, selects the full subkey
   fingerprint exactly (`!`), stages locally and cleans up its agent/home.
   It does not remove caller-owned credential files: the future protected job
   must clean those with an always-run cleanup as well. Never supply the
   offline primary to this wrapper, even though it rejects it after import.
5. Independently verify the staged public fingerprint, signatures, package
   checksums and client installation before allowing any deployment.

`build-apt-repo.sh` is the lower-level helper for an explicit caller-owned
mode-700 GNUPGHOME. Production automation should use the isolation wrapper,
not a persistent host keyring. Stage from a copy of the complete existing apt
site so older packages remain available; **never replace an existing pool
filename with different bytes**. Never overwrite an archive/release asset or
reuse a package version for changed content. Preserve existing immutable
release/tag/publication guards in `docs/macos-release-build.md`. A production
process must serialize staging/publication and retain a reviewed rollback
snapshot; no deployment workflow implementing these gates is included here.

## Client configuration

Verify the primary key fingerprint before installing:

```sh
sudo install -d -m 0755 /etc/apt/keyrings
curl --proto '=https' --tlsv1.2 -fsSL \
  https://cercano-ai.github.io/packages/cercano.gpg -o cercano.gpg
# STOP: independently compare the public primary fingerprint with the
# maintainer-published, separately authenticated fingerprint before install.
gpg --show-keys --with-fingerprint cercano.gpg
sudo install -m 0644 cercano.gpg /etc/apt/keyrings/cercano.gpg
echo 'deb [signed-by=/etc/apt/keyrings/cercano.gpg] https://cercano-ai.github.io/packages/ stable main' \
  | sudo tee /etc/apt/sources.list.d/cercano.list
sudo apt update
sudo apt install cercano
```

The primary key fingerprint is `55368AC6 14FE6CD9 DAC04A45 8A03FD1D 0192F44A`
(also in the top-level README). A repository signature authenticates apt
metadata/packages, not a signature on the original Linux archive or a Linux
support commitment.

## Production setup

Provisioning, credentials, protected environments, the real-package acceptance
gate and recovery are documented in the `cercano-ai/packages` README. Do not
send private key material in a chat response.
