#!/bin/bash
# Stateful ordinary-upgrade case (historical filename); never rerun on preserved fixtures.
# Direct-link conflict remains a separate case, recorded in commands.jsonl 134.
set -eu
R=/opt/cercano-failure-20261115-conflict
B=$R/brew-fixture.sh
old_hashes() {
  shasum -a 256 "$R/brew/Cellar/cercano/0.25.2/bin/cercano" \
    "$R/brew/Cellar/cercano/0.25.2/bin/cercano-cli"
}
state_hashes() {
  shasum -a 256 "$R/home/.config/cercano/config.yaml" \
    "$R/home/.cercano/conversation-marker" "$R/home/.cercano/credential-marker"
}
# Capture actual before bytes, not just a post-upgrade sample.
old_hashes > "$R/ordinary-old-before.sha256"
state_hashes > "$R/ordinary-state-before.sha256"
cat "$R/ordinary-old-before.sha256" "$R/ordinary-state-before.sha256"
# Continue collecting every postcondition even when brew or a check fails.
set +e
"$B" upgrade --formula migration/cercano/cercano > "$R/conflict.log" 2>&1
rc=$?
cat "$R/conflict.log"
echo "ORDINARY_UPGRADE_EXIT=$rc"
failed=0
check() {
  "$@"
  result=$?
  if test "$result" -ne 0; then
    echo "FAILED_POSTCONDITION: $* (exit $result)" >&2
    failed=1
  fi
}
old_hashes > "$R/ordinary-old-after.sha256"
check test "$?" -eq 0
state_hashes > "$R/ordinary-state-after.sha256"
check test "$?" -eq 0
cat "$R/ordinary-old-after.sha256" "$R/ordinary-state-after.sha256"
check "$B" list --formula --versions
check ls -l "$R/brew/bin/cercano" "$R/brew/bin/cercano-cli"
check test "$rc" -eq 0
check test "$(readlink "$R/brew/bin/cercano")" = ../Cellar/cercano-agent/0.27.0/bin/cercano
check test "$(readlink "$R/brew/bin/cercano-cli")" = ../Cellar/cercano-cli/0.27.0/bin/cercano-cli
check test -d "$R/brew/Cellar/cercano/0.27.0"
check test ! -e "$R/brew/Cellar/cercano/0.27.0/bin"
check cmp "$R/ordinary-old-before.sha256" "$R/ordinary-old-after.sha256"
check cmp "$R/ordinary-state-before.sha256" "$R/ordinary-state-after.sha256"
# Recorded historical executable identities (commands.jsonl 125).
check grep -Fx "effc0cb3c0ec4785b0f2244b872d75f0621249247ca067363c3d49b980c5b24e  $R/brew/Cellar/cercano/0.25.2/bin/cercano" "$R/ordinary-old-after.sha256"
check grep -Fx "c94534e942967207dacfa715ad534a3abddf75eb453d3ca75c501d71feee42d2  $R/brew/Cellar/cercano/0.25.2/bin/cercano-cli" "$R/ordinary-old-after.sha256"
if test "$failed" -ne 0; then
  exit 1
fi
echo PASS_ORDINARY_UPGRADE_NO_OVERWRITE
