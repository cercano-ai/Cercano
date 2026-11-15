# Signed v0.27.0 native synthetic-loopback smoke — bounded PASS

## Scope and retained state

Only acceptance clone `5df50e96-bb17-4248-aa6e-50ae559cd765` was operated on by the pinned `native-smoke.py` transport. No source VM operation, snapshot switch, snapshot creation/deletion, publication or push occurred in this slice. The source VM was not queried or modified. This is signed-package normal startup/interaction evidence, **not** real local-model inference, human visual acceptance, installed-user acceptance, or genuine rolling A/B.

New synthetic root: `/opt/cercano-native-smoke-20261115-1357`, with separate `brew`, `home`, `tmp`, `artifacts`, `inspect`, `evidence` and `evidence-tier-retry`. Retain it: these scripts are stateful, not safe to rerun over evidence. Original fixture roots and `/opt/cercano-snapshot-recovery-20261115` were not removed/reset. Initial/final snapshot-list outputs (records1/14) match exactly, including current `7c251a7a-8c29-4ff4-aab3-635ae38954e9`.

Authoritative evidence: `native-smoke-commands.jsonl`, **1-based line/record numbers** below. Guest payloads are hex encoded in argv and can be decoded with `bytes.fromhex`; stdout/stderr/status remain unabridged JSON. `native-smoke-public-release.json` retains public release metadata. The scripts describe the final fixture; recorded payloads preserve earlier versions and failures.

## Package identity before execution

Records3–4 obtain public release provenance and perform static archive inspection, without executing released binaries. The archive was copied from retained recovery artifacts, not rebuilt or re-signed; exact SHA matches the public release's archive digest:

- Archive: `18c68ccc69662da6d18b8017a3eba7eaebc67e057065b90bb66deba2838d2aa0`.
- `bin/cercano`: `19191f59f5528d46bcc3073c20dd287fab23f3bab5e3f0e4ae3e2683ad3dd7fc`.
- `bin/cercano-cli`: `f70f1aefbf8f43b4b30a8078486c7c2caa3360e2f6af9b75d60c0608d060c525`.

Archive executable hashes match its runtime inventory. Strict `codesign --verify --strict --verbose=2` passed for both members, with `TeamIdentifier=4HPM47RCM4`, Developer ID authority, runtime flag and secure timestamp. Record7 repeats strict signature/Team checks and explicit expected executable-hash comparisons on installed commands **before startup**. Record6 installed split `migration/cercano/{cercano-agent,cercano-cli,cercano}` 0.27.0 through real Homebrew in the new prefix, with formula URLs changed only to the local copy of the exact public archive. These remain unpublished formula fixtures, not a production tap update. Homebrew's fixture caveats, custom-prefix warnings and nonfatal `ps` EPERM messages remain in evidence.

Public release: https://github.com/cercano-ai/Cercano/releases/tag/v0.27.0 . Its notes attribute notarization/code-directory hash verification to [CI run 43655050601, attempt2](https://github.com/cercano-ai/Cercano/actions/runs/43655050601/attempts/2), Accepted submission `873414ff-cb1f-4d46-832d-8071e1b64aeb`, artifact ZIP digest `89a85b1902925be5c4fadb67d5088bacc29fb29eb8d248663536cb64ecdc7bf6`. This is **published CI provenance**, not independent Apple ticket validation or fresh clean-Mac Gatekeeper acceptance.

## Isolation and credential capability

`native-smoke.sb` is root-owned, mode444 in guest. Guest/host policy SHA256 matches: `3d57aee11ca9cf87aa38c27874f9375afbcb4eb1fd296fa8583978feb8066d35` (records6/13). Review: general network denied; IP bind/inbound/outbound permissions name only localhost ports49391 (agent) and49392 (synthetic Ollama); Unix sockets only under this fixture for production workers; signals only within the same sandbox. File-data reads and writes denied under `/Users/test`, `/Library/Keychains`, `/opt/homebrew`, prior migration/recovery/failure roots, plus all Keychains paths; securityd Mach lookups denied. Policy is an allow-default profile with explicit protections, not a universal filesystem allowlist. It was not weakened for provider-selection or persistence failures.

Execution uses synthetic UID/GID5501 `_cercanorehearsal`, scrubbed environment, synthetic HOME/TMPDIR, `CERCANO_DISABLE_CREDENTIALS=1`, and the outer sandbox inherited by agent, CLI, provider and workers. No credential sign-in or paid provider. Static marker presence was only preliminary evidence; actual valid=1 startup logs report `keychain unavailable (credentials disabled by CERCANO_DISABLE_CREDENTIALS)` while normal worker interaction succeeds. The synthetic provider emits a fixed response and no tool calls. Host/root orchestration copies fixture inputs before sandbox entry; it does not run released programs outside containment.

**Important negative finding:** the malformed-value probe uses `invalid`, **not** `1`. The release warns `Failed to load config: credentials disabled by CERCANO_DISABLE_CREDENTIALS: expected 0 or 1 (using defaults)`, continues using default config, then exits when the unchanged sandbox denies default `:50052`. Its exit1 is **not fail-closed credential validation**. The earlier interpretation that value1 was rejected is incorrect. No port50052/11434 permission was added. Valid=1 operation and protective keychain denial are the evidence for this bounded smoke, not malformed-value exit status. The runtime script comment now states this limitation explicitly.

## Exact failures before fixes

1. Record5: profile parser exit65, `host must be * or localhost in network address`, at `(local ip "127.0.0.1:49391")`. Changed literal network address spelling to supported `localhost:49391`/`localhost:49392`, retaining only the same two fixture ports. Record6 parses the policy using `/usr/bin/true` before installing.
2. Record7: contained malformed-value probe above cannot bind default `:50052`; retained as a negative probe, not repaired. Normal configured agent starts on49391, but `provider_observed: False`, `response_in_pty: False`; assertion `synthetic response absent`. Record8 preserves `main.provider_error`: `locus mode "open_only": no local provider available for main work`. CLI UI rendered; this first interaction **failed**.
3. Record10: retry fails before runtime with `PermissionError: [Errno 13] Permission denied: '/opt/cercano-native-smoke-20261115-1357/evidence-tier-retry'`. Root-owned fixture root is intentionally not writable by the synthetic UID. Fix: orchestration pre-creates/chowns only this new evidence directory; no protective policy change. Recorded retry payloads remain authoritative for config backup/override state.
4. Record11: retry uses supported `models.open.overrides.ollama` entries for `everyday`, `most_capable`, `fast_light`, `fast_light_text`, all `fixture-smoke`. Original config/DB are backed up in `evidence`. Same sandbox and production worker mode. No source patch, alternate binary or bypass entrypoint.
5. Record12: read-only SQLite open fails `OperationalError: unable to open database file`; finalization aborts before shutdown. Fix: add `immutable=1` to `mode=ro` URI after runtime has exited (avoids read-only SQLite sidecar locking); record13 passes actual marker/integrity checks. This is not a fresh acceptance run or DB rewrite.

## Successful supported interaction and persistence

Record11: actual installed `cercano agent`; actual installed `cercano-cli --address 127.0.0.1:49391`, controlling PTY, synthetic terminal-query replies. Supported config is Ollama URL `http://127.0.0.1:49392`, open-only, model `fixture-smoke`, worker execution, port49391, `agent.shutdown_on_last_client: false`, plus explicit tier overrides above. The agent reports wildcard listener `0.0.0.0:49391`; containment restricts networking to listed localhost endpoints, not arbitrary remote peers.

PTY input `SYNTHETIC_SMOKE_PROMPT_1357` reaches the real production agent and worker; synthetic HTTP provider is observed; `SYNTHETIC_SMOKE_RESPONSE_1357` reaches CLI PTY. Assertions report `cli_exit: 0`, `provider_observed: True`, `response_in_pty: True`, `agent_exit: 0`. CLI uses `/exit`; owned agent receives SIGTERM and logs draining/shutdown complete. No forced kill was needed on this successful run. Secondary recap destination is unavailable and compaction is a no-op: retained warning, not an additional acceptance claim.

Record13 opens the persisted synthetic `home/.config/cercano/conversations.db` read-only/immutable, confirms **both** synthetic markers and `pragma integrity_check = ok`. Both fixture endpoints refuse connections after owned cleanup. An unlisted localhost bind49393 raises PermissionError. Residual executable check finds no Cercano agent/CLI/model-host process. Recovery root still exists. This checks targeted policy behavior and bounded persistence, not exhaustive sandbox verification or whole-filesystem immutability.

## Cleanup, shutdown and validation

Evidence and installed synthetic prefix deliberately retained; no prior fixture, snapshot, tap or cache deleted. Record14 retains unchanged snapshot tree. Graceful guest shutdown record15 loses transport with255 (expected when VM stops); immediate record16 still reports running, so this alone is insufficient. After waiting, **record17 confirms acceptance clone stopped**. No source/reset/force-stop action.

Post-run script hardening: preflight now exits70 if the credential marker is absent; runtime comments no longer call malformed-value fallback fail-closed; finalizer now polls for stopped state instead of relying on immediate inventory. These edits were syntax-checked, not rerun over retained guest evidence. The raw executed payloads remain intact.

Final evidence reconciliation: record7 is the failed first interaction; record11 is the successful tier-override retry. They are distinct runs, not contradictory assertions from one run. Record13 independently checks persisted markers after the retry. Fresh clone-only inventories at records18–19 again confirm `State: stopped`; no guest startup or stateful fixture was rerun during this reconciliation. Python AST parsing for all `native-smoke*.py`, `bash -n native-smoke-preflight.sh`, exact comparison of snapshot-list records1/14, and host policy SHA256 comparison passed. These are syntax/evidence checks, not another native runtime run.

Remaining gates are unchanged except this bounded signed-package startup/interaction smoke: no real model runtime, model quality, human visual review, live drain/restart continuity, genuine old/new rolling A/B, real-user/keychain migration, independent notarization-ticket validation, clean-machine distribution acceptance, publication or shipping authorization.
