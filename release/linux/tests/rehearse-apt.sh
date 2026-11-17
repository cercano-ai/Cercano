#!/bin/bash
# Disposable LOCAL ONLY test. No network, sudo or publication.
# A test subkey export exists ONLY inside the trapped temporary GNUPGHOME.
# usage: rehearse-apt.sh <directory containing debs>
set -euo pipefail
[ $# -eq 1 ] || exit 2
for tool in gpg gpgconf gpgv apt-ftparchive apt-get dpkg-deb; do
  command -v "$tool" >/dev/null || { echo "SKIP: missing $tool" >&2; exit 77; }
done
here=$(cd "$(dirname "$0")/.." && pwd)
debs=$(cd "$1" && pwd)
umask 077
tmp=$(mktemp -d)
cleanup() {
  GNUPGHOME="$tmp/gnupg" gpgconf --kill all >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
export GNUPGHOME="$tmp/gnupg"
mkdir -m 700 "$GNUPGHOME"
unset CERCANO_GPG_PASSPHRASE_FILE
# Test-only identity. All private material stays in the trapped temporary home.
gpg --batch --no-tty --pinentry-mode loopback --passphrase '' \
  --quick-generate-key 'Disposable rehearsal <test@example.invalid>' ed25519 cert 1d >/dev/null 2>&1
primary=$(gpg --batch --with-colons --list-keys | awk -F: '$1 == "fpr" {print $10; exit}')
gpg --batch --no-tty --pinentry-mode loopback --passphrase '' \
  --quick-add-key "$primary" ed25519 sign 1d >/dev/null 2>&1
subkey=$(gpg --batch --with-colons --list-keys | awk -F: '$1 == "sub" {subkey=1; next} subkey && $1 == "fpr" {print $10; exit}')
# Exercise the isolated import wrapper, using only temporary test material.
gpg --batch --pinentry-mode loopback --passphrase '' --export-secret-subkeys "$primary" > "$GNUPGHOME/disposable-subkey"
bash "$here/sign-apt-repo.sh" "$tmp/site" "$debs" "$GNUPGHOME/disposable-subkey" "$subkey"
rm "$GNUPGHOME/disposable-subkey"
gpgv --keyring "$tmp/site/cercano.gpg" "$tmp/site/dists/stable/InRelease"
gpgv --keyring "$tmp/site/cercano.gpg" "$tmp/site/dists/stable/Release.gpg" "$tmp/site/dists/stable/Release"
cp -a "$tmp/site" "$tmp/good"
mkdir -p "$tmp/apt/lists/partial" "$tmp/apt/cache/archives/partial" "$tmp/apt/empty"
: > "$tmp/apt/status"
printf 'deb [signed-by=%s] file:%s stable main\n' "$tmp/site/cercano.gpg" "$tmp/site" > "$tmp/apt/sources.list"
apt_run() {
  apt-get -o "Dir::Etc::sourcelist=$tmp/apt/sources.list" \
    -o "Dir::Etc::sourceparts=$tmp/apt/empty" \
    -o "Dir::Etc::trusted=$tmp/apt/no-trusted" -o "Dir::Etc::trustedparts=$tmp/apt/empty" \
    -o "Dir::State::lists=$tmp/apt/lists" -o "Dir::State::status=$tmp/apt/status" \
    -o "Dir::Cache=$tmp/apt/cache" -o "APT::Sandbox::User=$(id -un)" \
    -o APT::Architecture=amd64 -o APT::Update::Error-Mode=any "$@"
}
apt_update() {
  rm -rf "$tmp/apt/lists"; mkdir -p "$tmp/apt/lists/partial"
  apt_run update
}
apt_update
mkdir "$tmp/downloads"
(cd "$tmp/downloads" && apt_run download cercano-agent)
rm "$tmp/downloads"/*
# Even with authentic indexes, altered package payloads must be rejected.
for file in "$tmp/site"/pool/main/c/cercano-agent/*.deb; do printf 'tamper' >> "$file"; done
if (cd "$tmp/downloads" && apt_run download cercano-agent); then
  echo 'ERROR: accepted tampered package' >&2; exit 1
fi
rm -rf "$tmp/site"; cp -a "$tmp/good" "$tmp/site"
# The signed metadata must fail after modification.
printf '\ntamper\n' >> "$tmp/site/dists/stable/InRelease"
if apt_update; then echo 'ERROR: accepted tampered signature' >&2; exit 1; fi
cp "$tmp/good/dists/stable/InRelease" "$tmp/site/dists/stable/InRelease"
# Change every index variant; avoid cached lists and force verification.
for file in "$tmp/site/dists/stable/main/binary-amd64/Packages"*; do
  printf 'tamper' >> "$file"
done
if apt_update; then echo 'ERROR: accepted tampered index' >&2; exit 1; fi
rm -rf "$tmp/site"; cp -a "$tmp/good" "$tmp/site"
# A different disposable public key is not an authorized repository signer.
gpg --batch --no-tty --pinentry-mode loopback --passphrase '' \
  --quick-generate-key 'Wrong rehearsal key <wrong@example.invalid>' ed25519 sign 1d >/dev/null 2>&1
gpg --batch --export 'wrong@example.invalid' > "$tmp/site/cercano.gpg"
if apt_update; then echo 'ERROR: accepted untrusted signer' >&2; exit 1; fi
cp "$tmp/good/cercano.gpg" "$tmp/site/cercano.gpg"
# Published pool bytes cannot be replaced under an existing filename.
mkdir "$tmp/changed"
cp "$debs"/*.deb "$tmp/changed/"
for file in "$tmp/changed"/*.deb; do printf 'tamper' >> "$file"; done
if bash "$here/build-apt-repo.sh" "$tmp/site" "$tmp/changed" "$subkey!"; then
  echo 'ERROR: replaced immutable pool bytes' >&2; exit 1
fi
echo 'PASS: disposable signing, apt verification, signature/index/package/trust tamper, immutable pool'
