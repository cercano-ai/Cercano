#!/bin/bash
# Merge the per-architecture package sets into the one review set that apt
# promotion consumes. No signing, credentials, installs or publication.
#
#   merge-review-packages.sh <amd64 dir> <arm64 dir> <output>
#
# Each input must be a flat directory whose SHA256SUMS lists exactly its other
# files, with matching bytes. The amd64 set carries the architecture-independent
# `cercano` package; the arm64 set may hold only cercano-agent and cercano-cli
# files, so the merge can never carry two different builds of one package.
# The output gets one SHA256SUMS in the same `sha256sum ./*` format. Output
# must not exist (immutable bytes).
set -euo pipefail
[ $# -eq 3 ] || { echo "usage: $0 <amd64 dir> <arm64 dir> <output>" >&2; exit 2; }
[ ! -e "$3" ] && [ ! -L "$3" ] || { echo 'refusing existing output' >&2; exit 2; }

check_set() {  # <dir> <label>
  local dir=$1 label=$2 name listed actual
  [ -d "$dir" ] && [ ! -L "$dir" ] || { echo "$label: not a directory" >&2; return 1; }
  [ -f "$dir/SHA256SUMS" ] && [ ! -L "$dir/SHA256SUMS" ] || { echo "$label: missing SHA256SUMS" >&2; return 1; }
  for name in "$dir"/* "$dir"/.[!.]* "$dir"/..?*; do
    [ -e "$name" ] || [ -L "$name" ] || continue
    [ -f "$name" ] && [ ! -L "$name" ] || { echo "$label: not a regular file: ${name##*/}" >&2; return 1; }
  done
  grep -Evq '^[0-9a-f]{64}  \./[A-Za-z0-9_+.~-]+$' "$dir/SHA256SUMS" \
    && { echo "$label: malformed SHA256SUMS" >&2; return 1; }
  listed=$(sed 's|^[0-9a-f]*  \./||' "$dir/SHA256SUMS" | LC_ALL=C sort)
  [ -z "$(uniq -d <<< "$listed")" ] || { echo "$label: duplicate SHA256SUMS entry" >&2; return 1; }
  actual=$(cd "$dir" && find . -maxdepth 1 -type f ! -name SHA256SUMS -printf '%f\n' | LC_ALL=C sort)
  [ -n "$actual" ] || { echo "$label: no packages" >&2; return 1; }
  [ "$listed" = "$actual" ] || { echo "$label: SHA256SUMS does not list exactly the files present" >&2; return 1; }
  (cd "$dir" && sha256sum --quiet --strict -c SHA256SUMS) || { echo "$label: checksum mismatch" >&2; return 1; }
}

amd64=$1
arm64=$2
check_set "$amd64" amd64
check_set "$arm64" arm64
for path in "$arm64"/*; do
  name=${path##*/}
  [ "$name" = SHA256SUMS ] && continue
  case "$name" in
    cercano-agent[-_]*|cercano-cli[-_]*) ;;
    *) echo "arm64: only cercano-agent and cercano-cli belong in this set: $name" >&2; exit 1 ;;
  esac
  [ ! -e "$amd64/$name" ] || { echo "both sets contain $name" >&2; exit 1; }
done

output=$3
mkdir -p "$(dirname "$output")"
parent=$(cd "$(dirname "$output")" && pwd)
output="$parent/$(basename "$output")"
tmp=$(mktemp -d "$parent/.linux-review.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir "$tmp/packages"
for path in "$amd64"/* "$arm64"/*; do
  [ "${path##*/}" = SHA256SUMS ] || cp "$path" "$tmp/packages/"
done
(cd "$tmp/packages" && sha256sum ./* > SHA256SUMS)
mv "$tmp/packages" "$output"
