# Isolated offline upgrade verification — 2026-10-29

Baseline: `feat/production-generation-integration` at `9b353408e179b7ca61e9cf699fb27408635f04a8`, including pending harness and refusal-test changes. Unrelated pending `plan.md` changes were preserved, not included in this checkpoint.

## Boundaries and expected invariants

Fresh mode-0700 temporary root, synthetic HOME/config/history/tasks, fixture writer explicitly closed before upgrade, no application hosts launched until afterward. An acceptance-tag-only compiled driver calls the real `conversation.UpgradeOffline`; the production CLI and its mandatory process-discovery preflight are unchanged. No user-facing bypass was added. Existing agents were not given this fixture path.

Before the verification run, expected conservation was one original turn and three unchanged historical task rows (`accepted`, `running`, `completed`) through upgrade and repeat. Upgrade performs no reconciliation/replay. History acquisition requires reachable hosts when no owner is known. B publication must preserve A work; busy reopen must refuse without scheduling transfer; completion drain must release ownership and retire A; B must persist another reply afterward. Historical task rows must remain unchanged after drain.

## Deterministic probes and focused tests

With `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

- From `source/server`: `go test ./cmd/cercano -run '^TestOfflineUpgrade' -count=1 -v` — PASS (known-agent refusal before changes, confirmation/detection limits, discovery failure before database open). Injected snapshots/errors only; no live discovery.
- From `source/clients/cli`: `go test ./internal/ui -run '^TestFindReopenOwnerFakeControls$' -count=1 -v` — PASS. Deterministically confirms stale unreachable possible owner refuses acquisition, while known-unowned reachable hosts permit it. Run before moving the historical-resume check.
- From `source/server`: `go test ./internal/conversation -run 'Offline|Ownership|Handover' -count=1` — PASS.

The late historical-resume check was blocked by A's stale record after retirement, not by upgrade. The harness now checks that history while both hosts are reachable; fail-closed production ownership behavior is unchanged.

## Compiled rehearsal

From repository root, with the same three environment guards:

```sh
python3 efforts/dev-host-generations/everyday-launcher-compiled.py \
  --repo . \
  --evidence efforts/production-generation-integration/evidence/offline-upgrade-isolated-verified \
  --offline-upgrade --same-conversation-close
```

Exit 0 in 87.8 seconds. Builds real tagged executables in a private source snapshot and exercises the actual launcher, hosts, CLI PTYs, worker and synthetic provider/MCP paths. Passed real upgrade + repeat preservation, original-history resume and fresh B reply, busy reopen refusal, task completion/drain, automatic A retirement, durable post-retirement B reply, and unchanged historical tasks.

Local raw evidence (not checkpointed): `evidence/offline-upgrade-isolated-verified/compiled.json`, `B-upgraded-history.pty`, `B-restarted.pty`, `provider.jsonl`, `worker.jsonl`, and `build.jsonl`. `compiled.json` SHA-256: `adca9f13ef6ffceab1c93525362c502447ae0ff20c3a81bb40e42bb931984985`; `passed=true`, cleanup `remaining=[]`. No token contents exported.

The harness retains its temporary artifacts by default. After its fixture-scoped process cleanup, this run's private root was explicitly removed and absence verified. Initial removal encountered sealed generation directory permissions; making only this private root's directories writable allowed complete removal. No real stores or native credentials/providers were used.

## Limitations

Production process discovery was **not exercised end-to-end in this environment**. Its refusal and ordering were tested separately through deterministic injection. This is bounded compiled acceptance with synthetic providers, not full ordinary-app production acceptance. Stale-host unknown-owner recovery is not claimed or changed. No push or landing performed.
