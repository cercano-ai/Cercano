# macOS upgrade: human acceptance handoff

**Status: all human checks below are PENDING; none has been executed for this handoff.** Phase 4's current-code native rehearsal is running; this document claims no rehearsal result or engineering/release completion. Recorded focused tests cover worker selection, admission/reconnect and synthetic runtime holds, not full human usability, real inference or arbitrary released-version compatibility. See [verification](verification.md), [scope](spec.md) and [worker/retention policy](../../docs/homebrew-worker-selection.md).

## Preconditions — PENDING

- [ ] Use an authorized disposable macOS environment and isolated synthetic conversations/state. Record macOS/architecture, Homebrew version, A/B CLI and agent versions, source revisions and artifact hashes. Do not use the developer's live installation or credentials. Authorize any model acquisition, network, disk/memory needs and test credentials separately before execution.
- [ ] Establish **prepared/admitted** A and B generation state through existing supported admission. Keg selection alone grants no prepared-store authority. If starting from legacy/unprepared state, expect explicit refusal/deferred activation where required: **no silent migration, mutation or fabricated B coexistence**. Stop and report if valid admission cannot be established.
- [ ] Confirm persistent `HOMEBREW_NO_CLEANUP_FORMULAE=cercano-agent` in the effective supported `brew.env`, preserving other settings/exclusions. Capture sanitized `brew config` from the **actual upgrade and cleanup invocation contexts** (user, HOME/XDG overrides, service/shell and prefix/precedence); a synthetic user's setting is not global. Do not configure other users or the host.
- [ ] For local checks, record the selected external llama executable, libraries, backends/resources and model paths/identities. Keep **all of them unchanged** throughout dependent sessions, including idle intervals and later work/restarts. Defer any external upgrade/cleanup affecting those paths until dependent sessions finish. This accepted Cercano-only upgrade boundary is not external-runtime cleanup protection.

## Conversation journey — each row PENDING

Use a harmless distinctive conversation marker (for example `A-context-17`) and a bounded task whose completion can be recognized. Capture session/host/worker identities before and after each step; public command version or socket readiness alone is insufficient.

| Status | Action | Expected outcome |
| --- | --- | --- |
| PENDING | Start an ordinary A conversation, give it the marker, and complete a normal turn. Record its admitted generation, host and worker. | Correct response and persisted history on A; baseline identities are available. |
| PENDING | Start a bounded long/in-flight A turn; while it is active, upgrade Cercano externally through the approved Homebrew procedure to B, leaving external runtime paths unchanged. | Turn completes without forced cancellation, duplicated work or lost history; installing/selecting B does not silently move A. |
| PENDING | Exercise automatic and explicit agent cleanup with the exclusion effective. Inspect cleanup diagnostics and canonical A executable identity/hash. | A stays intact and the public selection points to B. Explicit cleanup may print `Error: Refusing to clean ...` yet return 0; exit status alone is not acceptance. |
| PENDING | In the same A conversation, request a later marker-aware turn and a harmless delegated task after upgrade/cleanup. Use normal pool lifecycle to obtain a genuinely fresh worker; record how freshness was established without killing shared processes. | Both finish correctly with A context and persisted results. Each new relevant worker has a new launch identity/PID and canonical A executable/hash, not merely warm-worker reuse or alias B. Check delegated completion/bookkeeping as well as its text response. |
| PENDING | Disconnect only the client, reconnect to that same session, and complete another turn. | Session ID, history and serving A generation remain pinned; no silent B switch. If incompatible, an explicit safe error replaces fallback. |
| PENDING | Open a new eligible B conversation using prepared/admitted B state while A remains active; alternate turns in A and B. | B serves the new session; A remains usable on A. State/credential ownership boundaries hold; no forced A shutdown, cross-session history leakage or implicit legacy migration. |
| PENDING | With an authorized real local model, complete local inference in A before upgrade; leave A idle across the Cercano-only upgrade, then request later inference and a delegation using the local runtime. Record idle duration. Where supported, also exercise later runtime restart/reuse without changing external paths. | Actual local inference and later work/restart remain usable under the unchanged executable/library/backend/resource/model boundary. No cloud fallback counts as a pass; one surviving process or synthetic HOLD response is insufficient. If restart cannot safely be exercised, record it as pending, not passed. |
| PENDING | In a separate authorized incompatible candidate scenario, request activation while dependent sessions remain active; inspect installed/session version and activation diagnostics. | Installed B and session-serving A are clearly distinguished. Incompatible activation is deferred/refused **before shared-state mutation** with an understandable reason and drain/retry guidance; no forced cancellation or runtime replacement. After dependent sessions finish, retry only through supported admission. Compatible mixed CLI/agent pairs connect; incompatible pairs give a clear error, not false activation success. |

## Remaining human/release gates — PENDING

- [ ] **Next actual changed-source release:** repeat the journey with actual released A/B artifacts and record their real source revisions/versions. Current-code A/B version-only, same-source fixtures establish neither changed-source behavior nor arbitrary cross-version compatibility; do not manufacture a source change to claim this gate.
- [ ] **Historical monolith applicability:** establish whether it was distributed to users requiring migration. If yes, test that specific real upgrade and restart-hook behavior before claiming supported migration. If no, record the evidence and omit it. Until established, this question is pending—not an implied migration guarantee or a hypothetical package matrix.
- [ ] **Final signed-package smoke:** use the existing [macOS release gates](../../docs/macos-release-build.md), not a new pipeline. With the final verified signed/notarized artifacts, check clean-Mac install/launch, command ownership and version reporting, then the applicable upgrade/conversation/reconnect/local-idle journey above. Keep artifact checksum/provenance, notarization/trust and supported macOS/architecture qualification in that release process. A skipped/unverified trust check is not a pass. No new credentials/runners or publication are authorized by this checklist.

## Limits and recording results

The agent exclusion retains old agent kegs only: **no automatic retirement**, safe-to-delete detection or external llama/dependency/model retention. Disk use may grow. It is **not uninstall protection**; explicit uninstall, policy bypass or direct deletion are outside continuity guarantees. Loss/replacement of selected A must fail closed for new launches, never fall back to B. Do not delete retained kegs or stop unowned processes as test cleanup; record remaining owned processes, packages and configuration for separately authorized teardown.

Record human outcomes separately from automated/native evidence: checklist row, pending/pass/fail/blocked, tester/date, artifact identities and sanitized evidence location. Keep unperformed checks pending. A checklist, accepted support boundary or completed engineering slice is not human acceptance or release approval.

### Compact sanitized failure report

```text
Check / status / time:
macOS + architecture / Homebrew version:
A/B artifact versions + revisions + hashes; installed CLI/agent vs serving version:
Host identities (generation + sanitized endpoint/run ID + PID):
Session IDs / admitted generations / worker IDs + PIDs + canonical executable/hash:
Local runtime/dependency/backend/resource/model identities; unchanged? idle duration:
Effective cleanup context (user alias, HOME/XDG aliases, prefix, exclusion):
Minimal steps; expected -> actual; exact deferred/error message:
Relevant timestamped client/host/worker/modelhost + brew log excerpts:
Owned residual processes/packages/configuration; sanitized evidence location:
```

Redact tokens, credentials, auth headers, cookies, private keys, environment secrets, private prompts and personal path/account details before sharing. Use stable aliases for sensitive paths/identities so correlation survives sanitization; never attach raw credential/discovery records or full environment dumps.
