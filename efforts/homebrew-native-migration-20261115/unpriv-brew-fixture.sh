#!/bin/bash
set -euo pipefail
R=/opt/cercano-migration-unpriv-20261115
if /bin/ps -axo comm= | /usr/bin/grep -E '(^|/)(cercano|cercano-cli|cercano-model-host)$'; then
 echo 'REFUSED: unexpected Cercano process'; exit 70
fi
/usr/bin/sudo -u _cercanorehearsal /usr/bin/env -i HOME="$R/home" USER=_cercanorehearsal LOGNAME=_cercanorehearsal PATH="$R/brew/bin:/usr/bin:/bin:/usr/sbin:/sbin" TMPDIR="$R/tmp" HOMEBREW_CACHE="$R/cache" HOMEBREW_LOGS="$R/logs" HOMEBREW_TEMP="$R/tmp" HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_INSTALL_FROM_API=1 HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_CLEANUP_FORMULAE=cercano,cercano-agent HOMEBREW_AVOID_NESTED_SANDBOXING=1 /usr/bin/sandbox-exec -f "$R/safety.sb" "$R/brew/bin/brew" "$@"
