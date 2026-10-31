# Track A: CLI startup and resume parity

## Changes

Normal pinned and unpinned models now share startup commands (config, tools,
permission posture, runtime status, events, connection state and splash).
Explicit fixtures retain restricted initialization. Pinned setup uses the host's
explicit config authority, never the caller's default config when authority is
unknown. Existing host RPC config loading remains authoritative.

Picker/initial `-r`, `/resume`, and pinned reopen converge on progressive
hydration. Streaming starts when the command executes rather than while the
reducer constructs it. Metadata carries client, conversation, hydration and title
revision fences. Empty persisted titles clear previous titles; rename results
carry their conversation ID. Rebuilds retain same-conversation/picker titles and
advance hydration generation. Compaction boundary, prompt history, dev metadata,
sub-agent restoration and profile hydration remain supported.

Shared-file diff: `internal/ui/dev_host.go` only preserves pending/same-session
title and advances hydration generation during model reconstruction. No changes
to `dev_host_reopen.go`, discovery/transfer algorithms, server attachment/lifecycle
accounting, or launcher progress files.

## Evidence and verification

The title-resume probe was read as reducer-only evidence, not end-to-end proof.
The initial startup regression exposed pinned/ordinary command-count divergence;
its original nil-client test also panicked and was corrected rather than counted
as a valid passing test. The old buffered-resume test was updated to consume
stream events after both entry points became asynchronous.

With disposable HOME/config/state and synthetic test-owned RPC servers:

- CLI `go test ./... -count=1`: pass.
- Focused UI startup/resume tests under `-race`: pass.
- Server interface `go test ./pkg/agentclient ./pkg/proto -count=1`: pass.
- CLI disposable binary build, `--version`, `--help`: pass.
- `git diff --check`: pass.

New tests cover setup authority, ordinary/pinned initialization, fixture
restriction, empty title, stale metadata/client/generation/rename fences, rebuild
title preservation, and actual `/resume`/picker command → synthetic gRPC → reducer
hydration with persisted title and prompt history. Existing dev-mode tests cover
restoration from temporary persisted metadata. Tests never invoke inference.

## Limits

This is not a PTY-driven ordinary launcher → authenticated host → interactive
wizard → restart acceptance test. New transport tests execute hydration commands,
not the entire Bubble Tea event loop or all background subscriptions. Rendering,
setup completion against a running real host, large-history scroll positioning,
and full host-reopen/disconnect behavior are not established end-to-end here.
No live configuration/history/database/credentials/models or installed binaries
were used. No main merge, landing or push was performed. Unknown host config
authority permits explicit setup only; it deliberately does not infer first run
from a caller's unrelated config file.
