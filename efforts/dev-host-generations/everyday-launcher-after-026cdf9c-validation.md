# Fresh compiled ordinary-launcher validation (2026-10-26)

**Result: FAIL — new busy-transfer destination reservation leak. Not full acceptance.**

## Provenance and execution

Command: `python3 efforts/dev-host-generations/everyday-launcher-compiled.py --repo . --evidence efforts/dev-host-generations/everyday-launcher-compiled-after-026cdf9c-transfer`

Exit 1, 43.681 seconds. Checkout: `3158cfa6216f2a2cbe192abb8f93b68259ee5784` (includes permission fix 026cdf9c, provider configuration ec7378, continuation metadata f0559a8). Disposable committed source snapshot: `7321165ac14840fbd2da0b58ece4953f064464da`; source tree `d96eccded6d8a9f9ff0fa7f94ec560f435a4d51f`; injected fixture SHA256 `24701e62e9a00577f34fd483cb7acf574daa1714140f855f07d624d4e0531fc5`. The snapshot includes the test-only streaming tool-input repair. Dispatch explicitly selects local because the offline fixture has no secondary cloud destination.

Actual helper shell -> real publisher -> generation host/worker and CLI binaries -> production graph, with build-tag memory credentials/runtime and synthetic provider only. No lifecycle substitute or seeded task. Private temporary root, outer credentials disabled, no native credentials/model calls; no installation. Evidence copies logs, host metadata, PTY streams and provider events, never token files; token contents represented only by SHA256. Temporary hosts are terminated and root removed in finally.

## Fresh compiled checks reached

- A ordinary-launcher acquisition and approved real independent dispatch; database status running at child provider barrier.
- A terminal process killed and fresh CLI PTY reconnected via wrapper saved-selection flags. Same provider PID, identity/epoch selection, no build or acquisition, new foreground inference while child remains running.
- Separate Y acquired on A and terminal exited retaining its session.
- Source byte change publishes B; distinct generation/host while A's independent task remains running.
- Unchanged source reuses B; two concurrent ordinary wrapper invocations select the same B without rebuild/conflict.
- Saved-selection management list has no build/acquisition.
- Y successfully transfers A -> B while A's unrelated independent task remains running.
- A reconnect terminal physically killed before testing busy transfer, so refusal is not merely an attached-terminal gate.

## New production failure (exact observations)

B list before busy transfer: `pending_incoming` absent (0).

Busy A -> B transfer exits 1, returns `pending:true` and a prepare receipt, followed by:

```
cercano: rpc error: code = Unknown desc = retained sessions or work prevent transition
```

B list afterward: `pending_incoming:1`.

Probe assertion:

```
AssertionError: busy transfer refusal leaked destination pending_incoming: 0 -> 1
```

Root: `source/server/pkg/agentclient/lifecycle_controls.go:40-52` prepares the destination before asking the source to relinquish. Source refusal returns the acceptance/error without rolling back the destination reservation. This is an explicit source refusal, not a lost claim response. No production source changed. Raw calls, receipt and list responses are preserved in `everyday-launcher-compiled-after-026cdf9c-transfer/compiled.json`. Earlier localdispatch diagnostic also observed B retirement refusal with `pending_incoming:1`; that historical run is not substituted for this fresh validation.

## Exact remaining gaps

This fresh run intentionally stops at the new reservation-leak assertion, before releasing the child barrier. It does NOT establish task completion, exactly-one notification, release/retirement of A while B remains inference-usable, or end-to-end acceptance. Concurrent reuse establishes wrapper acquisition only, not concurrent inference. Reconnect checks fresh inference/same provider process and no publication/acquisition; it is not a packet-level transport trace. Existing downstream probe code remains incomplete and must be extended after production resolution; no pass is claimed for it.

## Separately labeled negative-entrypoint checks

Fresh command: `CERCANO_DISABLE_CREDENTIALS=1 python3 efforts/dev-host-generations/everyday-launcher-pty.py --repo . --evidence efforts/dev-host-generations/everyday-launcher-refusal-after-026cdf9c.json` (exit 0).

Actual shell entrypoint PTY: legacy refusal before publisher and unchanged file snapshot; mocked publisher failure preserves preexisting state and acquires nothing. These are fake-publisher negative checks, NOT a real compiler/publication-failure preservation test, and NOT compiled A/B acceptance.
