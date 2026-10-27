#!/usr/bin/env bash
# Fail closed until the production prepared-store contract and host orchestration
# are available. In particular, never invoke the old singleton launcher or kill
# a process merely because a newer binary exists.
set -euo pipefail
REPO="${CERCANO_REPO:-$HOME/git_repos/bryan_costanich/cercano}"
cat >&2 <<'MESSAGE'
[cercano] Generation launcher integration is not yet available; refusing legacy singleton startup.
The existing conversation database cannot be implicitly adopted or migrated to
an immutable-generation host. No database or running session has been changed.
A separately provisioned development store with an approved coexistence contract
and authenticated production host orchestration is required before launch.
See everyday-launcher-integration.md in this worktree for implementation status.
MESSAGE
exit 78
