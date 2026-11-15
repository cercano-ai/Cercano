#!/bin/bash
set -eu
R=/opt/cercano-failure-20261115-lag
B=$R/brew-fixture.sh
D=$R/brew/Library/Taps/migration/homebrew-cercano/Formula
# Repository-lag injection: convenience and agent present, client definition absent.
mv "$D/cercano-cli.rb" "$R/cli-held.rb"
set +e
"$B" install --formula migration/cercano/cercano > "$R/repository-lag.log" 2>&1
rc=$?
set -e
cat "$R/repository-lag.log"; echo REPOSITORY_LAG_EXIT=$rc
test "$rc" -ne 0
grep -E 'No available formula|unavailable|No formulae|No available formula with the name' "$R/repository-lag.log"
test ! -d "$R/brew/Cellar/cercano/0.27.0"
mv "$R/cli-held.rb" "$D/cercano-cli.rb"
# Genuine historical bytes packaged as agent-only fixture, not a historical split release.
"$B" install --formula migration/cercano/cercano-agent
"$B" install --formula migration/cercano/cercano-cli
"$B" test migration/cercano/cercano-agent
"$B" test migration/cercano/cercano-cli
"$B" install --formula migration/cercano/cercano
"$B" test migration/cercano/cercano
"$B" list --formula --versions
test "$(readlink "$R/brew/bin/cercano")" = ../Cellar/cercano-agent/0.25.2/bin/cercano
test "$(readlink "$R/brew/bin/cercano-cli")" = ../Cellar/cercano-cli/0.27.0/bin/cercano-cli
test ! -d "$R/brew/Cellar/cercano/0.27.0/bin"
echo PASS_REPOSITORY_LAG_AND_MIXED_PACKAGE_SET
