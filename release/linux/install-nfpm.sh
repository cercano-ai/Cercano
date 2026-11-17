#!/bin/bash
# Caller-owned tool directory only; no system installs. Linux x86_64 CI.
# Digest from upstream v2.48.0/checksums.txt, retrieved 2026-11-17; pinned here
# so a replaced asset/checksum cannot silently change executable tooling.
set -euo pipefail
[ $# -eq 1 ] || exit 2
[ "$(uname -s)/$(uname -m)" = Linux/x86_64 ] || { echo 'requires Linux/x86_64' >&2; exit 2; }
mkdir -p "$1"
dest=$(cd "$1" && pwd)
[ ! -e "$dest/nfpm" ] || { echo 'refusing existing nfpm' >&2; exit 2; }
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
curl --proto '=https' --tlsv1.2 -fsSL https://github.com/goreleaser/nfpm/releases/download/v2.48.0/nfpm_2.48.0_Linux_x86_64.tar.gz -o "$tmp/nfpm.tar.gz"
echo "6d7f9a28b2f397934403950fcbba83f598b08d02edbe79a0815b3b4897341750  $tmp/nfpm.tar.gz" | sha256sum -c -
tar -xzf "$tmp/nfpm.tar.gz" -C "$tmp" nfpm
install -m 0755 "$tmp/nfpm" "$dest/nfpm"
