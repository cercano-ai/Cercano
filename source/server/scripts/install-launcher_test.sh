#!/usr/bin/env bash
# No-clobber explicit destination tests only; never touch the installed launcher.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$SCRIPT_DIR/test-launcher-safety.py" LauncherSafety.test_explicit_destination_required LauncherSafety.test_install_and_refuse_overwrite_or_symlink
