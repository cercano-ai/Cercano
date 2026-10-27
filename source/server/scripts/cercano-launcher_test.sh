#!/usr/bin/env bash
# Actual source entrypoint with fake publisher/host dependencies; no inference.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$SCRIPT_DIR/test-launcher-safety.py"
