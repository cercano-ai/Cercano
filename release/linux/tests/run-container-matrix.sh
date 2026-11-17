#!/bin/bash
# Runs the lifecycle test (and, with an old set, the upgrade test) across a
# distribution matrix in throwaway Docker containers. Needs Docker; installs
# nothing on the host.
#
#   run-container-matrix.sh <dist dir> [<old dist dir> <new version>]
#
# <dist dir> holds all 12 packages from `nfpm package` (see ../README.md).
set -u
here=$(cd "$(dirname "$0")" && pwd)
dist=$(cd "$1" && pwd)
old=${2:+$(cd "$2" && pwd)}
new=${3:-}
matrix=(
  "ubuntu:24.04|deb|"
  "debian:12|deb|"
  "fedora:42|rpm|dnf install -y -q procps-ng shadow-utils util-linux >/dev/null 2>&1;"
  "rockylinux:9|rpm|dnf install -y -q procps-ng >/dev/null 2>&1;"
  "opensuse/leap:15.6|rpm|zypper -n -q install procps shadow >/dev/null 2>&1;"
  "alpine:3.20|apk|apk add -q procps >/dev/null 2>&1;"
  "archlinux:latest|arch|pacman -Sy --noconfirm procps-ng >/dev/null 2>&1;"
)
for row in "${matrix[@]}"; do
  IFS='|' read -r image fam prep <<< "$row"
  docker run --rm --platform linux/amd64 -v "$dist:/pkgs:ro" -v "$here:/tests:ro" "$image" \
    sh -c "$prep sh /tests/container-lifecycle.sh $fam" 2>&1
  if [ -n "$old" ]; then
    docker run --rm --platform linux/amd64 -v "$old:/old:ro" -v "$dist:/new:ro" -v "$here:/tests:ro" "$image" \
      sh -c "sh /tests/container-upgrade.sh $fam $new" 2>&1
  fi
  echo
done
