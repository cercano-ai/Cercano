# Shared config save boundary: inventory and stop decision

2026-10-28. Bounded inspection only; no persistence implementation or activation.
Read root AGENTS.md, Phase 3 plan and production integration spec, and existing
pkg/config and hostsvc/config coordination implementations.

## Production legacy Save inventory

Inspected every non-test Go file under source importing
`cercano/source/server/pkg/config`, including aliased imports, for Save calls.
There are four direct production call sites:

- `source/server/cmd/cercano/main.go:654`: startup inline credential relocation,
  blanking the YAML key after confirming credential storage.
- `source/server/cmd/cercano/main.go:1417`: setup missing-file creation.
- `source/server/cmd/cercano/main.go:1467`: setup final full save, using the same
  local cfg value as the earlier creation save.
- `source/server/internal/hostsvc/config/config.go:312`: Service.Persist
  (`cfg.Save` alias), covering service callers, not just routing RPCs.

All four still reach `pkg/config/config.go:1306`, which directly calls
os.WriteFile at line 1338. None was converted. Existing coordinator Save/Change
remain opt-in; mixing them with these legacy calls is unsafe.
This inventory is of pkg/config.Save callers, not a claim that every filesystem
writer or external editor in the application has been coordinated.

## Exact baseline/API policy blockers

1. **Successful Save cannot return its next baseline.** Current public signature
   is `Save(cfg Config, path string) error`, receiving Config by value. Adding an
   immutable disk baseline to Config would protect its first save, but cannot
   advance the caller's baseline after success. Setup explicitly saves the same
   cfg twice (creation at 1417, final persistence at 1467). The second save would
   conflict against its own first save. A pointer/result API and corresponding
   caller updates, or an explicitly adopted immutable save-result contract, is
   needed. Silently sharing a mutable baseline among copies would let stale
   copies inherit newer revisions and defeat the requested stale protection.
   Reading a new base immediately before saving is likewise not valid.

2. **Baseline-less full configurations currently can overwrite existing files.**
   Setup discards Load errors and uses Defaults at main.go:1277-1280, then saves
   at 1467 even when the authority already exists. That value has no valid disk
   baseline. The safe action is rejection, but this changes existing setup error
   behavior and needs an explicit policy: abort setup on load failure, allow
   read-only prerequisite work while refusing persistence, or introduce a
   separately authorized replacement/reset operation. Capturing the current
   file at save time cannot establish what this candidate was derived from.

3. **Service initialization/retargeting has no baseline contract.**
   hostsvc/config.New accepts arbitrary Config plus path and returns Service,
   not an error (config.go:82-85). SetPath can retarget independently of Set
   (146-154); server.SetConfig performs SetPath before Set (server.go:1643-1646).
   A constructor-time read cannot prove that an arbitrary supplied candidate
   came from that disk version. Missing baseline on an existing authority must
   be rejected, with explicit admission/initialization error behavior; it must
   not be repaired opportunistically by reading a fresh revision. Fileless
   service behavior must remain separate.

These require choosing the public save-result and baseline-less caller policy
before a coherent conversion, rather than introducing a partially coordinated
writer or changing those semantics silently. Stop before broad edits as requested.

## Other confirmed unsolved behavior

UpdateRoutingAssignments calls Mutate before Persist
(`internal/server/routing_settings.go:23-38`). Publication failure therefore
leaves changed live state. Fixing only that RPC would not coordinate the shared
save boundary, and no such competing path was added.

Fresh creation needs serialized exclusive publication at the canonical parent/
target authority; existing NewConfigCoordinator requires an existing file through
EvalSymlinks. Existing-file disappearance must remain a failure, not become
bootstrap. Full Save historically marshals Config (dropping unknown YAML fields),
whereas Change preserves unknown fields. A conversion must not silently merge
those distinct semantics or persist effective environment overrides through new
precise changes.

## Verification limits

No source edits, native credentials, real configuration access, runtime startup,
or tests were performed. Accordingly there is no red/green or race evidence and
no claim that Phase 3 is complete. Required follow-on verification after the API
policy is settled includes stale-save reproduction, independent handles,
disappearance, exclusive creation, symlink authorities, failed publication/live
state, and affected-package races using temporary fake data with
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off.
