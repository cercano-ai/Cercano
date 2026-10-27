#!/usr/bin/env bash
# Source entrypoint and no-clobber installer template. Never singleton startup.
set -euo pipefail
# Informational invocations reach the same central parser without prepared state.
INFO_ONLY=0
if [[ $# == 1 ]]; then
    case "$1" in -h|--h|-help|--help|-version|--version) INFO_ONLY=1 ;; esac
fi
if [[ "$INFO_ONLY" == 0 ]]; then
    export CERCANO_DEVELOPMENT_STATE="${CERCANO_DEVELOPMENT_STATE:-${HOME}/.config/cercano-development}"
fi
REPO="${CERCANO_REPO:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
exec python3 "$REPO/source/server/scripts/cercano-launcher.py" --repo "$REPO" "$@"
