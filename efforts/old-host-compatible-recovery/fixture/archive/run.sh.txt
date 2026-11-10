#!/bin/sh
set -eu
root=$(git rev-parse --show-toplevel)
fixture=$(mktemp -d "$root/.old-recovery-fixture.XXXXXX")
trap 'rm -rf "$fixture"' EXIT HUP INT TERM
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
revision=d7cbdeb21c44e0ceeefdf212203ed04a37d09150
git archive "$revision" source/server source/proto | tar -x -C "$fixture"
mkdir -p "$fixture/source/server/cmd/oldrecoveryfixture"
cp "$root/efforts/old-host-compatible-recovery/fixture/oldmain.go.txt" "$fixture/source/server/cmd/oldrecoveryfixture/main.go"
(cd "$fixture/source/server" && go build -o "$fixture/old-source" ./cmd/oldrecoveryfixture)
(cd "$root/source/server" && OLD_RECOVERY_BINARY="$fixture/old-source" go test ./internal/server -run '^TestActualOldRecoveryCharacterization$' -count=1 -v -timeout=45s)
