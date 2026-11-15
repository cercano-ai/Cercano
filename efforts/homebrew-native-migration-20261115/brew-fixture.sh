#!/bin/bash
set -euo pipefail
R=/opt/cercano-migration-20261115
# Refuse before any package operation if any real Cercano agent/worker exists.
if /bin/ps -axo comm= | /usr/bin/grep -E '(^|/)(cercano|cercano-cli|cercano-model-host)$'; then
 echo 'REFUSED: unexpected Cercano process'; exit 70
fi
/usr/bin/sudo -u test /usr/bin/env -i HOME="$R/home" USER=test LOGNAME=test PATH="$R/brew/bin:/usr/bin:/bin:/usr/sbin:/sbin" TMPDIR="$R/tmp" HOMEBREW_CACHE="$R/cache" HOMEBREW_LOGS="$R/logs" HOMEBREW_TEMP="$R/tmp" HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_INSTALL_FROM_API=1 HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_CLEANUP_FORMULAE=cercano,cercano-agent /usr/bin/sandbox-exec -f "$R/safety.sb" "$R/brew/bin/brew" "$@"
