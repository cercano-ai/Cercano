# Everyday launcher integration — implementation evidence

## Implemented safety slices

- Normal source launcher now **fails closed (exit 78)** with an explicit transition diagnostic. Removed stale-process killing, singleton startup, mutable libexec builds and PATH/sibling fallback. It does not yet publish or launch generations. This is a deliberate unavailable state, NOT a working everyday launcher.
- Installer requires `--destination /absolute/new/path`; no implicit `~/bin/cercano`, `--force`, replacement, symlink overwrite or nonexclusive publication. `make launcher` requires `LAUNCHER_DEST`. Installation was tested only in temporary directories. `/Users/bryancostanich/bin/cercano` was neither modified nor executed.
- `dev-host` consumes publisher canonical `build_id` / `binaries.cercano.sha256`, not fixture-only `{Build,SHA256}`. Existing compiled fixture tests now emit this canonical shape. No fixture execution behavior was promoted to production.

## Integration contracts retained for the remaining implementation

- Publisher is separately owned: use its `publish --root --generation --repo --sign` contract. Publisher/signing/socket-sizing files were not edited by this slice.
- Publish a complete generation before executing host/client. Never kill stale agents, replace running executables, remove generations or fall back to singleton/PATH discovery.
- Host reuse must authenticate an exact private record and generation identity; serialize same-generation launch. Retained sessions reconnect only, without respawn.
- Production must use prepared durable server plus real tool/provider graph, pinned worker callbacks, frozen shared config/credentials, authenticated peers and guarded modelhost. The fixture is not an acceptable default.
- Legacy databases must fail read-only preflight, never adoption or migration. No user database or native credentials were opened by this work.

## Blocker / incomplete scope

Inspection found `CurrentCoexistenceDeclaration` explicitly declares `cercano-dev-fixture-owned-v2` and `PrepareCoexistence` is documented as synthetic-fixture-only, not an application startup API (`internal/conversation/coexistence.go`). `NewPreparedDurableServer` supplies the durable bridge, but `dev-host` still installs nil production graph dependencies and is credential-disabled. The ordinary production startup cannot simply be reused: it opens the legacy store, performs retention/orphan sweeps, opens native credentials and includes inline credential migration.

No production store contract/provisioning transition was implemented or implicitly chosen. A separately provisioned development store is the preferred direction, but promotion of the fixture-only declaration to a supported real-data contract remains unresolved. Do not adopt existing databases or describe the fixture as production to bypass this boundary.

**Remaining implementation:** production provider/tool graph, guarded modelhost setup, pinned worker callbacks, private host registry/authenticated peer discovery, same-generation launch locking/reuse, publishing orchestration and CLI generation/session selection. The source launcher is intentionally disabled pending these pieces.

**Unmet acceptance:** actual normal launcher A independent task → B invocation → same-generation reuse → reconnect A → busy/unrelated transfers → completion retirement; concurrent launch, publication/host failure, and legacy-store transition acceptance. Existing fixture tests are not substitutes.

## Executed verification

- `python3 source/server/scripts/test-launcher-safety.py`: 3 tests passed (fail-closed launcher, explicit installer selection, no overwrite/symlink replacement).
- `CERCANO_DISABLE_CREDENTIALS=1 go test ./cmd/cercano -run 'TestDevHostCredentialGate|TestDevHostCompiledStartup' -count=1`: passed.
- `CERCANO_DISABLE_CREDENTIALS=1 go test ./cmd/cercano -run 'TestDevHostCanonicalPublisherManifest|TestDevHostLegacyFixtureManifestIsNotCanonical|TestDevHostCompiledExecution' -count=1`: passed, including injected synthetic execution only.

No production inference, native credential access, full-suite verification or normal-launcher generation acceptance was performed. No push/land.
