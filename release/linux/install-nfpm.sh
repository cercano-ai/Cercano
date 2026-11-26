#!/bin/bash
# Caller-owned tool directory only; no system installs. Linux x86_64 or aarch64 CI.
# Digests from upstream v2.48.0/checksums.txt, retrieved 2026-11-17 (x86_64)
# and 2026-11-26 (arm64); pinned here so a replaced asset/checksum cannot
# silently change executable tooling.
set -euo pipefail
[ $# -eq 1 ] || exit 2
case "$(uname -s)/$(uname -m)" in
  Linux/x86_64)
    asset=nfpm_2.48.0_Linux_x86_64.tar.gz
    sha=6d7f9a28b2f397934403950fcbba83f598b08d02edbe79a0815b3b4897341750 ;;
  Linux/aarch64)
    asset=nfpm_2.48.0_Linux_arm64.tar.gz
    sha=a9a75213763783b7669719e505474c748c5ad7dc66591a195619deb50f85c2f4 ;;
  *) echo 'requires Linux/x86_64 or Linux/aarch64' >&2; exit 2 ;;
esac
mkdir -p "$1"
dest=$(cd "$1" && pwd)
[ ! -e "$dest/nfpm" ] || { echo 'refusing existing nfpm' >&2; exit 2; }
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
curl --proto '=https' --tlsv1.2 -fsSL "https://github.com/goreleaser/nfpm/releases/download/v2.48.0/$asset" -o "$tmp/nfpm.tar.gz"
echo "$sha  $tmp/nfpm.tar.gz" | sha256sum -c -
tar -xzf "$tmp/nfpm.tar.gz" -C "$tmp" nfpm
install -m 0755 "$tmp/nfpm" "$dest/nfpm"
