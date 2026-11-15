#!/bin/bash
# Static preflight only; NEVER executes the released binaries.
set -euo pipefail
R=/opt/cercano-native-smoke-20261115-1357
A=cercano-0.27.0-darwin-arm64.tar.gz
EXPECTED=18c68ccc69662da6d18b8017a3eba7eaebc67e057065b90bb66deba2838d2aa0
test ! -e "$R"
mkdir -m 755 "$R"
mkdir "$R"/{home,inspect,evidence,tmp,artifacts}
cp "/opt/cercano-snapshot-recovery-20261115/artifacts/$A" "$R/artifacts/$A"
test "$(shasum -a 256 "$R/artifacts/$A" | awk '{print $1}')" = "$EXPECTED"
shasum -a 256 "$R/artifacts/$A"
tar -tzf "$R/artifacts/$A" | tee "$R/evidence/archive-members.txt"
# Archive is pinned to already inspected public bytes; extraction is not execution.
tar -xzf "$R/artifacts/$A" -C "$R/inspect"
B="$R/inspect/cercano-0.27.0-darwin-arm64/bin"
for n in cercano cercano-cli; do
 shasum -a 256 "$B/$n"
 codesign --verify --strict --verbose=2 "$B/$n"
 codesign -dv --verbose=4 "$B/$n" 2> "$R/evidence/$n.codesign.txt"
 cat "$R/evidence/$n.codesign.txt"
 grep -qx 'TeamIdentifier=4HPM47RCM4' "$R/evidence/$n.codesign.txt"
 /usr/bin/strings -a "$B/$n" > "$R/evidence/$n.strings.txt"
 if grep -Fq CERCANO_DISABLE_CREDENTIALS "$R/evidence/$n.strings.txt"; then
  echo "$n: disabled-credentials marker PRESENT (further verification required)"
 else
  echo "$n: BLOCKER: disabled-credentials marker ABSENT; do not execute"
  exit 70
 fi
 # Positive controls demonstrate string extraction covers config/keychain code.
 grep -oE 'CERCANO_[A-Z_]+|github.com/cercano[^ ]*/internal/secrets[^ ]*' "$R/evidence/$n.strings.txt" | sort -u | head -60 || true
done
cat "$R/inspect/cercano-0.27.0-darwin-arm64/runtime-inventory.json"
echo 'STATIC GATE ONLY: no agent/CLI execution, install, provider, or credential access'
sync
