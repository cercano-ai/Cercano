#!/bin/bash
# Add .deb files to a static, signed apt repository (suite and codename
# "stable", component "main"), for hosting on GitHub Pages or any static host.
#
#   build-apt-repo.sh <site dir> <deb dir> <signing key id>
#
# Needs apt-ftparchive (apt-utils) and gpg, with GNUPGHOME holding the
# repository's private signing key. It never creates or exports a private
# key. Existing pool files are never replaced, so a published version cannot
# change under users. Writes cercano.gpg (the public key users install).
#
# Users then run:
#   curl -fsSL <url>/cercano.gpg | sudo tee /etc/apt/keyrings/cercano.gpg >/dev/null
#   echo "deb [signed-by=/etc/apt/keyrings/cercano.gpg] <url> stable main" \
#     | sudo tee /etc/apt/sources.list.d/cercano.list
#   sudo apt update && sudo apt install cercano
set -euo pipefail
[ $# -eq 3 ] || { echo "usage: $0 <site dir> <deb dir> <signing key id>" >&2; exit 2; }
site=$1
# Resolve inputs before changing directory; relative deb paths must keep working.
debs=$(cd "$2" && pwd)
key=$3
[[ "$key" =~ ^[A-Fa-f0-9]{40}!?$ ]] || { echo "error: use a full signing fingerprint" >&2; exit 2; }
: "${GNUPGHOME:?explicit isolated GNUPGHOME required}"
[ -d "$GNUPGHOME" ] && [ ! -L "$GNUPGHOME" ] || exit 2
[ "$(stat -c %a "$GNUPGHOME")" = 700 ] && [ -O "$GNUPGHOME" ] || {
  echo "error: GNUPGHOME must be owned by caller and mode 700" >&2; exit 2;
}
# Fail rather than opening pinentry or reading a terminal. For a protected CI
# subkey, the isolation wrapper supplies a private passphrase file.
gpg_args=(--batch --yes --no-tty --pinentry-mode error)
if [ -n "${CERCANO_GPG_PASSPHRASE_FILE:-}" ]; then
  gpg_args=(--batch --yes --no-tty --pinentry-mode loopback --passphrase-file "$CERCANO_GPG_PASSPHRASE_FILE")
fi
shopt -s nullglob
inputs=("$debs"/*.deb)
[ ${#inputs[@]} -gt 0 ] || { echo "error: no deb packages" >&2; exit 2; }
gpg --batch --list-secret-keys "$key" >/dev/null 2>&1 || { echo "error: no secret key $key in GNUPGHOME" >&2; exit 1; }
mkdir -p "$site"
cd "$site"
# Written outside the scanned tree, so Release never lists itself.
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
shopt -s nullglob
added=0
for deb in "$debs"/*.deb; do
  pkg=$(dpkg-deb -f "$deb" Package)
  dest="pool/main/${pkg:0:1}/$pkg/$(basename "$deb")"
  if [ -e "$dest" ]; then
    cmp -s "$deb" "$dest" || { echo "error: $dest is already published with different bytes" >&2; exit 1; }
    continue
  fi
  mkdir -p "$(dirname "$dest")"
  cp "$deb" "$dest"
  added=$((added + 1))
done
for arch in amd64 arm64 all; do
  dir=dists/stable/main/binary-$arch
  mkdir -p "$dir"
  # --arch X lists X and "all" packages; binary-all lists only the "all" ones.
  if [ "$arch" = all ]; then
    apt-ftparchive --arch all packages pool | awk 'BEGIN { RS = ""; ORS = "\n\n" } /\nArchitecture: all\n/' > "$dir/Packages"
  else
    apt-ftparchive --arch "$arch" packages pool > "$dir/Packages"
  fi
  gzip -9nkf "$dir/Packages"
done
apt-ftparchive \
  -o APT::FTPArchive::Release::Origin=Cercano \
  -o APT::FTPArchive::Release::Label=Cercano \
  -o APT::FTPArchive::Release::Suite=stable \
  -o APT::FTPArchive::Release::Codename=stable \
  -o "APT::FTPArchive::Release::Architectures=amd64 arm64 all" \
  -o APT::FTPArchive::Release::Components=main \
  release dists/stable > "$tmp"
mv "$tmp" dists/stable/Release
chmod 0644 dists/stable/Release
gpg "${gpg_args[@]}" --local-user "$key" --clearsign -o dists/stable/InRelease dists/stable/Release
gpg "${gpg_args[@]}" --local-user "$key" -abs -o dists/stable/Release.gpg dists/stable/Release
gpg --batch --yes --export "$key" > cercano.gpg
echo "added $added package(s); index lists $(grep -c '^Package:' dists/stable/main/binary-amd64/Packages) amd64/all entries"
