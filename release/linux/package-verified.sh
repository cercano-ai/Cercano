#!/bin/bash
# Package only a checksum- and inventory-verified release archive. No signing,
# credentials, installs or publication. Output must not exist (immutable bytes).
set -euo pipefail
[ $# -eq 4 ] || { echo "usage: $0 <version> <archive> <sha256> <output>" >&2; exit 2; }
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
  --maintainer 'cercano maintainers <maintainers@cercloud.ai>' --output "$tmp/render"
mkdir "$tmp/packages"
for package in cercano-agent cercano-cli cercano; do
  for format in deb rpm apk archlinux; do
    nfpm package -f "$tmp/render/$package.yaml" -p "$format" -t "$tmp/packages/"
  done
done
(cd "$tmp/packages" && sha256sum ./* > SHA256SUMS)
mv "$tmp/packages" "$output"
