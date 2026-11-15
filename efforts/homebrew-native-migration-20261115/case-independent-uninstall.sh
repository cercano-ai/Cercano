#!/bin/bash
set -eu
R=/opt/cercano-failure-20261115-lag
B=$R/brew-fixture.sh
shasum -a 256 "$R/home/.config/cercano/config.yaml" "$R/home/.cercano/"* > "$R/data-before.sha256"
# Agent-only upgrade while client and convenience are unchanged.
"$B" upgrade --formula migration/cercano/cercano-agent
"$B" test migration/cercano/cercano-agent
test "$(readlink "$R/brew/bin/cercano")" = ../Cellar/cercano-agent/0.27.0/bin/cercano
test "$(readlink "$R/brew/bin/cercano-cli")" = ../Cellar/cercano-cli/0.27.0/bin/cercano-cli
"$B" list --formula --versions
# Remove convenience first; dependencies stay installed, avoiding --force.
"$B" uninstall --formula migration/cercano/cercano
test -x "$R/brew/bin/cercano"; test -x "$R/brew/bin/cercano-cli"
"$B" uninstall --formula migration/cercano/cercano-cli
test -x "$R/brew/bin/cercano"; test ! -e "$R/brew/bin/cercano-cli"
"$B" install --formula migration/cercano/cercano-cli
"$B" test migration/cercano/cercano-cli
"$B" install --formula migration/cercano/cercano
"$B" test migration/cercano/cercano
test ! -d "$R/brew/Cellar/cercano/0.27.0/bin"
echo PASS_INDEPENDENT_AGENT_UPGRADE_CLI_REINSTALL_CONVENIENCE_REINSTALL
"$B" uninstall --formula migration/cercano/cercano
"$B" uninstall --formula migration/cercano/cercano-cli
"$B" uninstall --formula migration/cercano/cercano-agent
test ! -e "$R/brew/bin/cercano"; test ! -e "$R/brew/bin/cercano-cli"
shasum -a 256 -c "$R/data-before.sha256"
"$B" list --formula --versions
echo PASS_UNINSTALL_PRESERVES_SYNTHETIC_DATA
