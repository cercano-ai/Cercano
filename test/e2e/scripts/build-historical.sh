#!/usr/bin/env bash
# Builds cercano and cercano-cli from earlier commits into test/e2e/.bin/<rev>/
# for the upgrade-transition specs (specs/per-profile-flag-*.test.mjs) and the
# Go transition test (cmd/cercano/per_profile_flag_transition_test.go):
#
#   c23f4656  the last commit before profiles (a pre-profile install)
#   f79bf87a  per-profile agent hosts without the CERCANO_PER_PROFILE_AGENTS flag
#
# Each revision is exported with `git archive` into a temporary directory, so
# the working tree is never touched, and built with the module cache only
# (GOPROXY=off): nothing is downloaded.
set -euo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
root="$(cd "$here/../.." && pwd)"
revs=("$@")
[ ${#revs[@]} -gt 0 ] || revs=(c23f4656 f79bf87a)

for rev in "${revs[@]}"; do
  out="$here/.bin/$rev"
  src="$(mktemp -d "${TMPDIR:-/tmp}/cercano-src-$rev-XXXXXX")"
  trap 'rm -rf "$src"' EXIT
  git -C "$root" archive "$rev" source | tar -x -C "$src"
  mkdir -p "$out"
  (cd "$src/source/server" && GOPROXY=off GOFLAGS=-mod=readonly go build -o "$out/cercano" ./cmd/cercano/)
  (cd "$src/source/clients/cli" && GOPROXY=off GOFLAGS=-mod=readonly go build -ldflags "-X main.version=e2e-$rev" -o "$out/cercano-cli" .)
  rm -rf "$src"
  trap - EXIT
  echo "built $out/cercano and $out/cercano-cli"
done
