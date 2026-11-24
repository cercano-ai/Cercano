#!/usr/bin/env bash
# Sign a freshly built binary with this machine's Developer ID identity, when
# one exists. A stable signing identity keeps the macOS Keychain ACL matching
# across rebuilds, so the agent's secret reads (cloud profile API keys, OAuth
# tokens) stop prompting for a password after every rebuild — an unsigned Go
# binary gets a fresh ad-hoc signature each build and looks like a stranger
# to the Keychain.
#
# Usage: codesign-if-available.sh [--development] <binary> [stable-identifier]
# --development explicitly disables network timestamping for local builds.
# Without it, retain codesign's system default. Releases use sign-macos-release.sh.
#
# Identity: $CERCANO_CODESIGN_ID when set ("none" disables signing entirely);
# otherwise the first "Developer ID Application" identity in the keychain.
# No identity found = silent no-op, so builds on machines without one (CI,
# other platforms) are unaffected.
set -euo pipefail

development=false
if [[ "${1:-}" == "--development" ]]; then
    development=true
    shift
fi

bin="${1:?usage: codesign-if-available.sh <binary> | --resolve-identity}"
# --resolve-identity emits the signing selector on line one, followed by
# discovery evidence for hashing only (never persist this output in manifests).
# Publisher resolves once, fingerprints the certificate token, then pins it for
# both binaries. Normal build callers retain the same optional signing policy.
resolve=false
[[ "$bin" == "--resolve-identity" ]] && resolve=true

# Identity discovery needs the macOS `security` tool. Without it (Linux, other
# platforms) there are no identities to find, so signing stays a no-op instead
# of failing the build under `set -e`.
find_identities() {
    command -v security >/dev/null 2>&1 || return 0
    security find-identity -v -p codesigning 2>/dev/null
}

id="${CERCANO_CODESIGN_ID:-}"
[[ "$id" == "none" ]] && exit 0
if $resolve; then
    # Use certificate hashes, not display names, so certificate renewal also
    # invalidates cached generations. Explicit hashes/ad-hoc need no discovery.
    if [[ "$id" == "-" || "$id" =~ ^[[:xdigit:]]{40}$ ]]; then
        printf '%s\n' "$id"
        exit 0
    fi
    identities=$(find_identities)
    resolved=$(printf '%s\n' "$identities" | awk -v requested="$id" '
        /"/ {
            name=$0; sub(/^[^"]*"/, "", name); sub(/".*$/, "", name)
            if ((requested == "" && name ~ /^Developer ID Application: /) ||
                (requested != "" && name == requested)) { print $2; exit }
        }')
    # Preserve arbitrary codesign selectors: codesign itself diagnoses them.
    # Include discovery evidence for cache identity too: arbitrary explicit
    # selectors may resolve differently after certificate renewal/import.
    printf '%s\n%s' "${resolved:-$id}" "$identities"
    exit 0
fi
if [[ -z "$id" ]]; then
    id=$(find_identities \
        | sed -n 's/.*"\(Developer ID Application: [^"]*\)".*/\1/p' | head -1)
fi
[[ -z "$id" ]] && exit 0

# The launcher builds into a PID-suffixed temporary file. Use the final
# executable name as its identifier so Keychain requirements survive rebuilds.
identifier="${2:-$(basename "$bin")}"
sign=(codesign --force --sign "$id" --identifier "$identifier")
# Omitting --timestamp does NOT disable it for Developer ID identities.
# Do not retry signing errors or fall back to another identity.
if $development; then
    sign+=(--timestamp=none)
fi
"${sign[@]}" "$bin"
echo "[codesign] signed $bin as $id" >&2
