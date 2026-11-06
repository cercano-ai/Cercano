#!/usr/bin/env bash
# Explicit-destination launcher installer tests; never touch the installed launcher.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$SCRIPT_DIR/test-launcher-safety.py" LauncherSafety.test_explicit_destination_required LauncherSafety.test_missing_parent_refused LauncherSafety.test_install_then_update_existing_launcher LauncherSafety.test_install_replaces_symlink_without_touching_target LauncherSafety.test_directory_destination_refused_without_staging_leftovers
