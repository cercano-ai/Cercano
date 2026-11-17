#!/bin/bash
# Local staging only. Never publishes. Input must be a signing-subkey-only
# export (offline primary secret material must never be supplied).
# usage: sign-apt-repo.sh <site> <debs> <subkey-file> <signing-fingerprint>
set -euo pipefail
[ $# -eq 4 ] || { echo "usage: $0 <site> <debs> <subkey-file> <signing-fingerprint>" >&2; exit 2; }
[[ "$4" =~ ^[A-Fa-f0-9]{40}$ ]] || exit 2
here=$(cd "$(dirname "$0")" && pwd)
umask 077
home=$(mktemp -d)
cleanup() {
  GNUPGHOME="$home" gpgconf --kill all >/dev/null 2>&1 || true
  rm -rf "$home"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
export GNUPGHOME="$home"
# Do not enable shell tracing or upload this directory, import output or inputs.
gpg --batch --no-tty --import "$3" >/dev/null 2>&1
# A subkey-only export has an unavailable primary (sec ... #). Refuse a
# complete primary private key, even if a caller accidentally supplies one.
if gpg --batch --with-colons --list-secret-keys | awk -F: '$1 == "sec" && $15 != "#" { bad=1 } END { exit !bad }'; then
  echo 'error: offline primary secret material supplied; refusing' >&2
  exit 2
fi
bash "$here/build-apt-repo.sh" "$1" "$2" "$4!"
