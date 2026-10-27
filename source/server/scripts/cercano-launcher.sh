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
if [[ "$INFO_ONLY" == 0 ]] && [[ -z "${CERCANO_DEVELOPMENT_STATE:-}" || ! -f "${CERCANO_DEVELOPMENT_STATE}/config.yaml" || ! -f "${CERCANO_DEVELOPMENT_STATE}/conversations.db" ]]; then
    printf '%s\n' '[cercano] Prepared separate development state and explicit config.yaml required.' 'Provision once, exclusively: /absolute/generation/cercano prepare-development --new-state /absolute/NEW/private/state' 'Preparation copies existing config.yaml privately (no legacy database or native credentials). Default state: ~/.config/cercano-development; set CERCANO_DEVELOPMENT_STATE for a custom path.' 'No database or running session has been changed.' >&2
    exit 78
fi
REPO="${CERCANO_REPO:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
exec python3 "$REPO/source/server/scripts/cercano-launcher.py" --repo "$REPO" "$@"
