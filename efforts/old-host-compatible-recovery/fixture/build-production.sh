#!/bin/sh
# Explicit opt-in: archive/build only, never launches a host or reads user config.
set -eu
[ "${OLD_RECOVERY_PRODUCTION_FIXTURE:-}" = 1 ] || { echo 'set OLD_RECOVERY_PRODUCTION_FIXTURE=1 to build pinned production host/worker offline' >&2; exit 2; }
root=$(git rev-parse --show-toplevel)
fixture=$(mktemp -d "$root/.old-recovery-production.XXXXXX")
trap 'rm -rf "$fixture"' EXIT HUP INT TERM
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
revision=d7cbdeb21c44e0ceeefdf212203ed04a37d09150
git archive "$revision" source/server source/proto | tar -x -C "$fixture"
# Same executable contains ordinary generation host and worker entrypoints.
# Runtime probe adapts only the tagged synthetic inference backend to loopback.
if [ "${OLD_RECOVERY_EXECUTE:-}" = 1 ]; then
  python3 "$root/efforts/old-host-compatible-recovery/fixture/loopback-provider.py" "$fixture"
fi
(cd "$fixture/source/server" && go build -tags 'developmentfixture cercano_development_acceptance' -o "$fixture/cercano" ./cmd/cercano)
echo "Built historical production host/worker $revision (offline fixture tags)"
if [ "${OLD_RECOVERY_EXECUTE:-}" = 1 ]; then
  (cd "$root/source/server" && OLD_RECOVERY_PRODUCTION_BINARY="$fixture/cercano" go test ./cmd/cercano -run '^TestHistoricalProductionController$' -count=1 -v -timeout=180s)
fi
