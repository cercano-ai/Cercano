#!/bin/bash
set -eu
R=/opt/cercano-failure-20261115-recovery
B=$R/brew-fixture.sh
F=$R/brew/Library/Taps/migration/homebrew-cercano/Formula/cercano-cli.rb
"$B" unlink migration/cercano/cercano
"$B" install --formula migration/cercano/cercano-agent
test "$(readlink "$R/brew/bin/cercano")" = ../Cellar/cercano-agent/0.27.0/bin/cercano
test ! -e "$R/brew/bin/cercano-cli"
# Inject only a missing URL; do not alter trusted archive/version/checksum.
cp "$F" "$R/cli-before-injection.rb"
sed 's|/artifacts/cercano-0.27.0-darwin-arm64.tar.gz|/artifacts/missing-cli-artifact.tar.gz|' "$R/cli-before-injection.rb" > "$F"
chown 5501:5501 "$F"
set +e
"$B" install --formula migration/cercano/cercano-cli > "$R/missing-cli.log" 2>&1
rc=$?
set -e
cat "$R/missing-cli.log"; echo MISSING_CLI_EXIT=$rc
test "$rc" -ne 0
grep -E 'Could not read|Couldn.t open|No such file|Download failed|Failed to download' "$R/missing-cli.log"
test ! -d "$R/brew/Cellar/cercano-cli/0.27.0"
test ! -d "$R/brew/Cellar/cercano/0.27.0"
test "$(readlink "$R/brew/bin/cercano")" = ../Cellar/cercano-agent/0.27.0/bin/cercano
# Safe pre-convenience recovery: unlink new owner before linking saved monolith.
"$B" unlink migration/cercano/cercano-agent
"$B" link migration/cercano/cercano
test "$(readlink "$R/brew/bin/cercano")" = ../Cellar/cercano/0.25.2/bin/cercano
test "$(readlink "$R/brew/bin/cercano-cli")" = ../Cellar/cercano/0.25.2/bin/cercano-cli
shasum -a 256 "$R/brew/Cellar/cercano/0.25.2/bin/"*
"$B" list --formula --versions
cp "$R/cli-before-injection.rb" "$F"; chown 5501:5501 "$F"
echo PASS_MISSING_CLI_PRE_CONVENIENCE_RELINK
