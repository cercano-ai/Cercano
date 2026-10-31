# Full upgrade-discovery blocker inventory — 2026-10-30

## Result and scope

Fresh read-only preflight at `127.0.0.1:50052` exited **1**, with **34 unique blockers** in one enumeration, not successive first-PID runs. All 34 were then inspected with `ps`, `lsof -nP -p PID`, and a repeated `ps`; all repeated identity/command snapshots matched. This is a point-in-time inventory, not an atomic host-wide snapshot or proof that future candidates cannot appear. Raw observations: `processes-final.json`; complete refusal: `inventory-final.txt`.

Only diagnostic aggregation changed. `PreflightUpgradeHost` enables aggregation through the same production classification routine; production discovery still disables aggregation and refuses before stopping anything. No classifier exclusion was broadened. No ambient signals, database/config contents, installation, landing or push were used. `lsof` observes descriptors, not file contents. Disposable test children alone are cleaned up by tests.

## Classification and unknowns

**No demonstrated classifier bug.** PID 7439 and six other missing-path workers use `worker --socket PATH --prepared`, outside the missing-executable classifier's deliberately bounded four-argument form. A Unix socket plus worker-looking argv and no observed TCP/database descriptors does not establish safe exclusion or restore executable identity. The intact orphan workers likewise fail the required parent/executable association to the selected host. Accepting these would change the safety policy, not repair a proven erroneous identity check.

All candidates below are UID 501, PPID 1, with PGID equal to PID. Full executable, generation, arguments, start time, socket paths, cwd, descriptor observations and repeated snapshots are retained in `processes-final.json`. **W** means worker argv + Unix socket(s), no observed conversations.db or TCP descriptor; future writes, peers and trustworthy host association remain unknown. **H** means development-host argv + open conversations.db/WAL/SHM and TCP listener: a concrete possible writer, not proof of a write at inspection time. **M** means missing executable refusal; **A** means no verified host/worker association. No candidate is cleared.

| PID | Start (local, 2026) | Identity/root | Observations | Refusal | Provenance / limit |
|---|---|---|---|---|---|
| 7439 | Oct 26 20:14:05 | `ca-2eyh34z1` | W; 1 Unix FD | M | No matching retained provenance found |
| 12347 | Oct 26 20:53:22 | `ca-ehwggq0u` | W; 1 Unix FD | M | Acceptance runtime-root match only; no exact PID in cited records |
| 24401 | Oct 26 22:39:10 | `ca-djmp5zri` | W; 1 Unix FD | M | No matching retained provenance found |
| 27420 | Oct 26 22:47:47 | `ca-2b5ugzmz` | W; 1 Unix FD | M | No matching retained provenance found |
| 27885 | Oct 26 22:49:07 | `ca-yvberozz` | W; 1 Unix FD | M | No matching retained provenance found |
| 28448 | Oct 26 22:55:10 | `ca-n19njilm` | W; 1 Unix FD | M | No matching retained provenance found |
| 29024 | Oct 26 22:57:02 | `ca-v_iglp0q` | W; 1 Unix FD | M | Acceptance runtime-root match only; no exact PID in cited records |
| 98106 | Oct 26 19:59:49 | `ca-pmvkb3h2` | H; 127.0.0.1:51044 (LISTEN) | M | No matching retained provenance found |
| 98571 | Oct 26 20:02:31 | `ca-kiioyg4f` | H; 127.0.0.1:51063 (LISTEN) | M | No matching retained provenance found |
| 99375 | Oct 26 20:04:21 | `ca-zggyirz3` | H; 127.0.0.1:51320 (LISTEN) | M | No matching retained provenance found |
| 34013 | Oct 26 23:08:15 | `ca-h92fhm40` | W; 1 Unix FD | A | No matching retained provenance found |
| 35298 | Oct 26 23:09:35 | `ca-q8yb6lo4` | W; 1 Unix FD | A | No matching retained provenance found |
| 36021 | Oct 26 23:10:40 | `ca-hasybwcj` | W; 1 Unix FD | A | No matching retained provenance found |
| 36627 | Oct 26 23:11:47 | `ca-g046csib` | W; 1 Unix FD | A | No matching retained provenance found |
| 36905 | Oct 26 23:14:40 | `ca-g046csib` | W; 1 Unix FD | A | No matching retained provenance found |
| 37252 | Oct 26 23:15:14 | `ca-suu_tvjs` | W; 1 Unix FD | A | Acceptance runtime-root match only; no exact PID in cited records |
| 37555 | Oct 26 23:31:37 | `ca-suu_tvjs` | W; 1 Unix FD | A | Acceptance runtime-root match only; no exact PID in cited records |
| 38449 | Oct 27 02:26:37 | `ca-ryjtjujz` | W; 1 Unix FD | A | No matching retained provenance found |
| 38845 | Oct 27 02:27:10 | `ca-ryjtjujz` | W; 1 Unix FD | A | No matching retained provenance found |
| 44943 | Oct 27 09:30:54 | `ca-03tuwgvk` | W; 1 Unix FD | A | No matching retained provenance found |
| 45341 | Oct 27 09:31:28 | `ca-03tuwgvk` | W; 1 Unix FD | A | No matching retained provenance found |
| 59320 | Oct 27 09:45:29 | `ca-t_b3p03x` | W; 1 Unix FD | A | No matching retained provenance found |
| 59725 | Oct 27 09:46:01 | `ca-t_b3p03x` | W; 1 Unix FD | A | No matching retained provenance found |
| 60422 | Oct 27 09:46:41 | `ca-ya1jupfg` | W; 1 Unix FD | A | Acceptance runtime-root match only; no exact PID in cited records |
| 60824 | Oct 27 09:47:14 | `ca-ya1jupfg` | W; 1 Unix FD | A | Acceptance runtime-root match only; no exact PID in cited records |
| 61528 | Oct 27 09:48:01 | `ca-8rw3_skh` | W; 1 Unix FD | A | Acceptance runtime-root match only; no exact PID in cited records |
| 61926 | Oct 27 09:48:35 | `ca-8rw3_skh` | W; 1 Unix FD | A | Acceptance runtime-root match only; no exact PID in cited records |
| 63130 | Oct 27 10:13:21 | `cercano-development-synthetic-78c4769fad / cd-1yqwwm0e` | H; 127.0.0.1:61589 (LISTEN) | A | No matching retained provenance found |
| 70217 | Oct 27 11:32:44 | `.config/cercano-development / cd-oslfhol6` | H; 127.0.0.1:62427 (LISTEN) | A | No matching retained provenance found |
| 70225 | Oct 30 05:37:20 | `.config/cercano-development / cd-oslfhol6` | H; 127.0.0.1:54242->127.0.0.1:59586 (ESTABLISHED); 127.0.0.1:54243 (LISTEN) | A | No matching retained provenance found |
| 78630 | Oct 22 10:29:42 | `installed libexec worker` | W; 2 Unix FD | A | No matching retained provenance found |
| 87628 | Oct 27 19:22:34 | `dh-2089263077` | W; 1 Unix FD | A | No matching retained provenance found |
| 87629 | Oct 27 19:22:34 | `dh-2089263077` | W; 1 Unix FD | A | No matching retained provenance found |
| 87630 | Oct 27 19:22:34 | `dh-2089263077` | W; 1 Unix FD | A | No matching retained provenance found |

### Provenance assessment (all candidates)

Eight workers have attributable **acceptance runtime roots**, not proven individual process lifetimes: 12347 (`everyday-launcher-compiled-evidence`), 29024 (`everyday-launcher-compiled-after-026cdf9c-transfer`), 37252/37555 (`everyday-launcher-final-528a-manifest.json` and acceptance evidence), 60422/60824/61528/61926 (`sqlite-contention-landing-evidence.md`). Those retained records were re-read and excerpts saved in `provenance-final.json`; none contains the respective exact PID. Runtime-root matching supports probable acceptance leftovers, but does not establish launch start-time identity, current ownership, harmlessness or permission to terminate. Older referenced temporary compiled records were unavailable in the earlier bounded lookup (`older-fixture-records.json` is empty); retained repository excerpts remain the available evidence.

For the other 26 candidates, the retained efforts/docs provenance search found no matching record. In particular PID 7439 (`ca-2eyh34z1`, started Oct 26 20:14:05) has no attributable record. The `ca-*`, `dh-*` and synthetic-looking names are not proof of fixture provenance. PID 78630 is an installed libexec worker, not attributable to the cited acceptance runs. PIDs 70217/70225 are in the ordinary development state root; 70225 additionally has an established loopback connection. No live state/config/token/DB was opened to resolve ownership or current activity.

### Specifically requested acceptance evidence

All three supplied files were parsed in full and SHA-256 captured in `acceptance-records.json` (local retained copy). No exact current blocker PID occurs in any of them. Their cleanup claims cover their recorded owned processes only, not this inventory:

- `/private/tmp/cstop-zgipsem9/result.json` — SHA-256 `74f00a094098cbb9a07ccca1563c5cdcfdfbbb9e751cce978387bb47f0695c50`.
- `/private/tmp/fresh-acceptance-20261030-m0apb27h/compiled.json` — SHA-256 `6ea96cdfd7927eb27ca2d60b4aa8071a3018c45b4f31899d9826f172e83992f2`.
- `/private/tmp/fresh-acceptance-20261030-m0apb27h/verification.json` — SHA-256 `31f358e119393cab61f92ee0931c17150a984e8a067028354b80bac30e8e2347`.

The combined-stop record reports acceptance true, existing PIDs 25415/25429/25463, late worker 25480 and cleanup_remaining `{}`. Fresh compiled acceptance reports passed true and cleanup remaining `[]`, with runtime root `ca-azjde0k6`; verification reports combined shutdown/backup/migration counts 1/1/1, compatible shutdown/migration counts zero, and empty owned-PID checks. These do not attribute or exonerate any of the 34 current blockers. Historical success does not supersede live refusal.

## Necessary user action / escalation

Do not proceed with upgrade while these blockers remain. The user/process owners must identify retained sessions/work and authorize their orderly shutdown or identity-checked fixture cleanup, preserving data first. In particular, confirm ownership and disposition of the six possible writer hosts **98106, 98571, 99375, 63130, 70217, 70225** before any disruption; their database descriptors and listeners make blanket fixture cleanup inappropriate. Confirm ownership/disposition of **all 28 workers listed above**, including eight runtime-matched likely leftovers; no permission to kill follows from these matches. Revalidate PID/start-time/executable identities immediately before any separately authorized action; do not execute a stale PID kill list.

Exactly unknown: current work/session value, whether/when each host writes, worker peer/current authority and future write ability, precise launch provenance for unmatched candidates, individual lifetime linkage for runtime-matched candidates, and owner permission to stop them. Resolving these requires owner evidence/consent, not a classifier bypass. After owners resolve the full set, rerun the read-only full-inventory preflight. Broadening prepared-worker exclusions or ignoring unrelated development hosts would be a consequential safety-policy change outside this bounded diagnostic fix.

## Verification and artifacts

Commands from `source/server`:

- `go test ./internal/brewrestart -run '^TestDiscoveryInventoryAggregatesWithoutChangingRefusal$' -count=1 -v`: deliberately forcing first-error behavior in the shared routine failed with an omitted PID; restored aggregation passed. Only disposable children used. Logs: `aggregation-regression-before.txt`, `aggregation-regression-after.txt`. This is an aggregation regression, not a classifier change.
- `go test ./internal/brewrestart -count=1 -v`: PASS, 11.074s; `tests.txt`.
- `go build -o efforts/upgrade-inventory-evidence/preflight ./cmd/upgrade-discovery-preflight`: passed. Fresh binary SHA-256 `dfcb46e21e2ff308c868f42e4997d7d485b2cafa92f87503ec743be59a1efe74`.
- `efforts/upgrade-inventory-evidence/preflight -address 127.0.0.1:50052`: exit 1; `inventory-final.txt`, 34 unique blockers. The missing-path diagnostics repeat the PID prefix cosmetically; deduplication is by PID, not line count.

No upgrade/A-B acceptance exercise was rerun: classification and upgrade refusal criteria are unchanged. Raw local historical evidence and binaries are retained but not included in the explicit solved-file checkpoint. Worktree remains `/Users/bryancostanich/git_repos/cercano-ai/Cercano-upgrade-fixes`, branch `fix/upgrade-discovery-wal`; no installation, landing or push.
