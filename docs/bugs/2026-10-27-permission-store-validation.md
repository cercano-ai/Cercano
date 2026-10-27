# PermissionStore validation and path boundaries

Review follow-up to `8c63af382044`.

## Reproductions and repair

Before the fix, `TestPermissionInvalidAuthority` failed: initial loads accepted
unknown modes, empty/mode-omitted files, and multiple YAML documents; Mode could
install bypass from the first document while allowlist readers could install `*`
from a policy whose mode was invalid. `TestPermissionEstablishedDisappearance`
also failed: deleting an observed/committed strict file and calling AddMCPAllow
recreated it with permissive mode. These reproductions were run before changing
implementation. The hardlink characterization passed both before and after.

All disk readers and mutators now share single-document validation. Existing
files must declare a recognized mode, including on initial load (which returns
nil plus an error for invalid policy). Refresh installs the whole validated
mode/allowlist or retains the whole last-known valid policy. Unknown compatible
fields remain preserved by mutation. Every store tracks whether it has observed
an authority or committed one; missing established authority cannot be recreated
by either mutator. Empty and mode-omitted existing files are rejected, not guessed.
A genuinely absent initial file still defaults to permissive; file-less runtime
updates are unchanged. This is store-lifetime knowledge, not a durable history of
deleted files: a fresh process cannot distinguish a never-created path from a
previously deleted path. No new persistent registry or path semantics were added.

Regression coverage exercises all three readers, both mutators, initial errors,
whole-policy refresh, missing authority across concurrent established stores,
and the existing multi-store/process coordination suite. Broker tests verify
errors propagate without broadcasting a successful mode change. The server's
always-allow caller logs persistence errors and still honors the human's explicit
one-call approval (`internal/server/server.go:3850-3858`); it does not install a
persistent grant on failure. Startup cmd callers are outside this change's scope.

## Hardlink aliases: characterized, not fixed

`TestPermissionHardlinkCharacterization` creates two names for one inode, loads
both, then publishes bypass through one. The other stays strict: target-path
sidecar locks differ, and atomic rename replaces only one directory entry.
Thus subsequent policy changes diverge even without concurrency. Inode locking
alone would not fix this: publication still splits the links.

The PermissionStore API accepts a path; its implementation explicitly promises
existing **symlink** target coordination (the adjacent alias comment refers to
that symlink case). The permission design/user documentation describes the
permissions.yaml path and persistence, not general inode/hardlink alias
coherence. No explicit hardlink-alias promise was found in those docs. Existing
symlink behavior and supported path semantics are unchanged here.

Smallest prospective safety fix: reject multiply-linked authority files before
mutation (platform-specific link-count checks), documenting that clients must
share one canonical path or supported symlink target. This would be a separate
contract change, not implemented here. Supporting hardlinks transparently would
require a shared authority identity and publication design, not just a different
lock key; in-place writes would sacrifice current atomic-reader guarantees.

## Verification

With `CERCANO_DISABLE_CREDENTIALS=1`, `GOPROXY=off`, `GOSUMDB=off`:

- `go test -race ./internal/agent -run 'Permission|MCPAllow|RuntimeUpdate' -count=5`
- `go test -race ./internal/hostsvc/permissions -count=1`
- `go test ./internal/agent -count=1`

All passed. Tests use temporary policy files; no live configuration, credentials,
production processes, or cmd files were changed.
