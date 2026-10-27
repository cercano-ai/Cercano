#!/usr/bin/env bash
# Source entrypoint and no-clobber installer template. Never singleton startup.
set -euo pipefail
if [[ -z "${CERCANO_DEVELOPMENT_STATE:-}" || ! -f "${CERCANO_DEVELOPMENT_STATE}/config.yaml" || ! -f "${CERCANO_DEVELOPMENT_STATE}/conversations.db" ]]; then
    printf '%s\n' '[cercano] Prepared separate development state and explicit config.yaml required.' 'Provision once, exclusively: /absolute/generation/cercano prepare-development --new-state /absolute/NEW/private/state' 'Then supply an explicit config.yaml (no legacy data or native credentials) and set CERCANO_DEVELOPMENT_STATE.' 'No database or running session has been changed.' >&2
    exit 78
fi
REPO="${CERCANO_REPO:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
exec python3 "$REPO/source/server/scripts/cercano-launcher.py" --repo "$REPO" "$@"
