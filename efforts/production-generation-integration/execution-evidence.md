# Execution evidence

## 2026-10-27 — Phase 1 shared routing foundation

Execution worktree: `/Users/bryancostanich/git_repos/cercano-ai/Cercano-production-generation-integration`, branch `feat/production-generation-integration`, planning baseline `f306b61d4491e8e21c20bd087338c28cad13c2cd`.

Completed child task reports checkpoint `bf865c0ed007`: ordinary and prepared startup now use shared invocation-local lazy routing, validator, coordinator and session construction in `cmd/cercano/production_composition.go`. Provider/embedding dependencies are passed in; prepared restrictions remain unchanged. This is a bounded extraction, not complete service parity or live enablement.

Parent read the shared composition source and independently ran from source/server:

```
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off \
  go test -race ./cmd/cercano -run '^TestProductionComposition' -count=1
```

Result: exit 0, package PASS (1.526s). No native credential or live process qualification. Remaining composition covers service registration, store/lifecycle policies, credential seams and background producers. The acceptance matrix records required product coverage separately; its evidence slots remain pending until their full checks run.

## Further bounded slices

`7be8c983ed60` shares capability registration, dispatch-engine assembly, permission binding and explicit worker policy. Parent reran `go test -race ./cmd/cercano -run 'TestProduction|TestDevelopmentProduction' -count=1` with credentials disabled and offline Go settings: PASS. This is not full service parity.

`6911420450d3` adds invocation-local authority resolution and shared credential dependencies with child fake-backend checks. Child reports focused/race tests passing; canonical authority resolution was not activated in startup in that checkpoint. Staged startup integration is next.

Permission persistence `8c63af382044` reproduced stale mode overwrite and introduced fresh-read/mutate/atomic publication under the existing file lock. Independent review found inconsistent reader validation and missing-file privilege relaxation. `162b66585fda` records reproductions/fixes with shared policy validation and established-authority tracking. Parent reran permission-matching agent tests under the race detector three times: PASS. The same filter selected no hostsvc/permissions tests, so parent separately ran the whole `./internal/hostsvc/permissions` package with `-race -count=1`: PASS. All ran with credentials disabled and offline Go settings.

Hard-link authority divergence is characterized but unresolved; see `docs/bugs/2026-10-27-permission-store-validation.md`. No complete cross-host settings visibility or permission-worker update acknowledgement is claimed. Phase 3 remains in progress. These isolated prerequisites do not enable generation startup against live state.
