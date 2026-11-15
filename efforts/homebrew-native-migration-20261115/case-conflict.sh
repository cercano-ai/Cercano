#!/bin/bash
set -eu
R=/opt/cercano-failure-20261115-conflict
B=$R/brew-fixture.sh
set +e
"$B" upgrade --formula migration/cercano/cercano > "$R/conflict.log" 2>&1
rc=$?
set -e
cat "$R/conflict.log"
echo "ORDINARY_UPGRADE_EXIT=$rc"
test "$rc" -ne 0
grep -E 'Could not symlink|already a symlink|link step did not complete' "$R/conflict.log"
# No --overwrite: both historical links and bytes must survive unchanged.
test "$(readlink "$R/brew/bin/cercano")" = ../Cellar/cercano/0.25.2/bin/cercano
test "$(readlink "$R/brew/bin/cercano-cli")" = ../Cellar/cercano/0.25.2/bin/cercano-cli
test ! -d "$R/brew/Cellar/cercano/0.27.0"
shasum -a 256 "$R/brew/Cellar/cercano/0.25.2/bin/"*
"$B" list --formula --versions
ls -l "$R/brew/bin/cercano"*
echo PASS_ORDINARY_CONFLICT_NO_OVERWRITE
