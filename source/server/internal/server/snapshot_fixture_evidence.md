# Snapshot API integration fixture repair (2026-10-28)

Test fixtures only; production baseline validation is unchanged.

## Reproduction before editing

All commands below used `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`.
From `source/server`:

```
go test ./internal/server -run '^(TestStartConfigWatcher_EndToEnd|TestRoutingSettingsClientPersistenceSnapshotExecution|TestSetupResetKeepsExistingRPCSession)$' -count=1
```

All three failed at the reported seed saves (original lines 136, 94, 34), with
`configuration changed since base snapshot; reload and reconcile before saving`.

A second pre-edit run reproduced the same seed failure in both
`TestReloadConfigFromDisk_*` tests, `TestSetupResetRediscoversPreservedDefaultDownloads`,
all three `TestAutomaticConfigToLaunchConfirmationAndBudget` subtests,
`TestSetupPreservesContextSelectionAcrossSaves`, and all seven
`TestProductionSingleHostStartupShutdown` scenarios. A separate pre-edit run of
`TestRoutingSettingsSnapshotWorkerHTTP` reproduced it in all four backup combinations.

## Cause and bounded repair

The paths are newly allocated temporary config paths, not pre-existing user data.
`config.Defaults()` and config literals have no authority-bound snapshot.
`Save` forwards that zero baseline to `SaveSnapshot`; coordinator `checkBase`
rejects it even when the file is absent. `Load` on genuine absence captures the
required authority-bound absent snapshot. Legacy `Save` also discards the returned
revision, so repeated saves must not reuse an old draft.

Fixtures now explicitly load absence, publish with `SaveSnapshot`, and retain
its returned baseline with `WithSnapshot`. Existing reload checks and original
behavioral assertions remain. The RPC-reset fixture installs its newly committed
baseline into the service before reset. The routing cross-layer fixture explicitly
binds its in-memory routing result to its newly allocated persistence path.
No conflict retries, production edits, or new mocks were introduced. An import-aware
scan found no remaining imported pkg/config `Save` test callers in server or CLI;
pkg/config's own compatibility/boundary tests were deliberately not changed.

## Passing verification

From `source/server`, same disabled/offline environment:

```
go test ./internal/server ./internal/localruntime/llamaserver ./cmd/cercano -run '^(TestStartConfigWatcher_EndToEnd|TestRoutingSettingsClientPersistenceSnapshotExecution|TestSetupResetKeepsExistingRPCSession|TestReloadConfigFromDisk_.*|TestSetupResetRediscoversPreservedDefaultDownloads|TestAutomaticConfigToLaunchConfirmationAndBudget|TestSetupPreservesContextSelectionAcrossSaves|TestProductionSingleHostStartupShutdown)$' -count=1
go test -race ./internal/server ./internal/localruntime/llamaserver ./cmd/cercano -run '^(TestStartConfigWatcher_EndToEnd|TestRoutingSettingsClientPersistenceSnapshotExecution|TestSetupResetKeepsExistingRPCSession|TestReloadConfigFromDisk_.*|TestSetupResetRediscoversPreservedDefaultDownloads|TestAutomaticConfigToLaunchConfirmationAndBudget|TestSetupPreservesContextSelectionAcrossSaves|TestProductionSingleHostStartupShutdown)$' -count=2
go test -race ./internal/worker -run '^TestRoutingSettingsSnapshotWorkerHTTP$' -count=2
go test -race ./pkg/config ./internal/hostsvc/config ./internal/hostsvc/credentials ./internal/setupreset ./internal/setupresetcmd -count=1
go test ./... -run '^$'
```

`go test ./... -run '^$'` also passed in `source/clients/cli` and
`source/enterpriseapi` (compilation only, not full-suite execution).

Existing credential-enabled fixtures retain their scoped `credentialtest.Enable`
seam: temporary HOME/AWS inputs, in-memory stores and loopback-only HTTP. No new
credential enabling was added. Worker HTTP tests use fake credential fetching and
httptest endpoints; runtime launch confirmation executes the test helper binary,
not an installed model server. Startup tests use injected runtime/storage/listener
seams. No live apps, native credential stores, or user config were invoked.

## Production defects

None exposed by these reproduced failures or passing bounded checks. The missing
baseline rejection is intentional fail-closed behavior, not a production defect.
This does not claim full-suite or live-startup qualification.
