#!/bin/bash
# Package only a checksum- and inventory-verified release archive. No signing,
# credentials, installs or publication. Output must not exist (immutable bytes).
#
# The architecture defaults to amd64. The architecture-independent `cercano`
# package is built only with amd64: a repository may hold one set of bytes per
# package and version, and separate builders do not reproduce it exactly. An
# arm64 run packages cercano-agent and cercano-cli only.
set -euo pipefail
[ $# -eq 4 ] || [ $# -eq 5 ] || { echo "usage: $0 <version> <archive> <sha256> <output> [amd64|arm64]" >&2; exit 2; }
arch=amd64
[ $# -eq 4 ] || arch=$5
case "$arch" in
  amd64) packages=(cercano-agent cercano-cli cercano); extra=() ;;
  arm64) packages=(cercano-agent cercano-cli); extra=(--without-convenience) ;;
  *) echo "unsupported architecture: $arch" >&2; exit 2 ;;
esac
[ ! -e "$4" ] || { echo 'refusing existing output' >&2; exit 2; }
here=$(cd "$(dirname "$0")" && pwd)
output=$4
mkdir -p "$(dirname "$output")"
parent=$(cd "$(dirname "$output")" && pwd)
output="$parent/$(basename "$output")"
tmp=$(mktemp -d "$parent/.linux-packages.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
python3 "$here/render_packages.py" --version "$1" --archive "$2" --sha256 "$3" \
  --maintainer 'cercano maintainers <maintainers@cercloud.ai>' --output "$tmp/render" \
  --arch "$arch" "${extra[@]}"
mkdir "$tmp/packages"
for package in "${packages[@]}"; do
  for format in deb rpm apk archlinux; do
    nfpm package -f "$tmp/render/$package.yaml" -p "$format" -t "$tmp/packages/"
  done
done
(cd "$tmp/packages" && sha256sum ./* > SHA256SUMS)
mv "$tmp/packages" "$output"
