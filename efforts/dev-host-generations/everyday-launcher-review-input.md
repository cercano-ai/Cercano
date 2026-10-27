# Independent base API review for integration

Read-only review (not runtime proof) identified required integration checks:

- `Retain` acquires ownership/reloads but does not create the conversation row. Create new rows under capability before dependent writes; reconnect never reacquires.
- Prepared WorkerRunner injection does not reproduce production bridges automatically. Do not call legacy SelectExecutionMode afterward. Wire supported callbacks against prepared services.
- Worker host profile mutation forwards worker-selected ConversationId while reads use host turn ID (`worker/host.go:1022–1047`). A copied legacy callback changes broker state without ownership validation. Bind mutation to admitted turn ID and exact ownership/current-turn authority.
- Dispatch child creation needs BOTH parent and child capabilities (`conversation/store.go:539–554`, ownership tests). Legacy EnsureSubagent bridge only carries parent context. Required independent-work acceptance must exercise real owned dispatch, not advertise a silently broken capability or swap it for a synthetic-only path.
- Modelhost protection must be set across ensure/reconnect/runtime paths; prepared constructor alone does not provide it. No alternate writable restart callback.
- Provider factories must consume frozen credential/config service; no native writable fallback on required refresh/login. Mutation-required auth should return explicit refusal.
- Keep direct InvokeTool, ID-only schedulers/retention, runtime restart refused rather than reopening unsafe legacy paths.

Publisher checkpoint deb61938e60b exposes:
```
fp=$(python3 "$P" fingerprint --repo "$repo" --go "$go" --sign)
python3 "$P" publish --repo "$repo" --go "$go" --sign --root "$root" --generation "dev-$fp" --fingerprint "$fp" --reuse
```
Same environment/options for fingerprint and publication; canonical manifest build_id/binaries hashes. Full real-build review pending. Socket worker reports max 103-byte endpoint, ResourceRoot max 38 resolved bytes; ensure short private root.
