# Targeted inspection closeout

2026-10-27. Read-only source assessment; no operational acceptance claimed. Four bounded inspections are closed for planning. Remaining details below are implementation test obligations, not requests for another broad audit.

## MCP configuration and lifecycle

`internal/mcp_host/manager.go:68–107,233–381` constructs the manager with its configuration directory, launches stdio servers, starts/stops local handles, and persists add/remove through whole-file load/modify/write to `mcp.yaml` (`:381`, direct WriteFile). Local teardown/defunct-handle protections are reusable. Cross-host definition mutation and reconciliation are not provided by local handle locking. Preserve manager functionality, coordinate fresh-read definition updates, reconcile each consumer, and explicitly own external connections/processes. Arbitrary configured servers cannot be presumed safe to duplicate. Use protocol-faithful fixtures for discovery, update/restart races and retiring one host without closing another's resources.

## Telemetry

Delegated source trace: `internal/telemetry/telemetry.go:143–170` opens with per-connection busy timeout, WAL and initialization; `:209–230` legacy ALTER handling does not establish complete successful migration. `accounting_store.go:29–124` uses immediate transactional initialization/version checks, and `:169–209` validates transactional batches with identity/revision deduplication. These are reusable SQLite mechanisms, not grounds for a new telemetry server. Direct bounded grep for DELETE FROM, Prune, Retention, retention in this package returned no matches; this is not proof there is no retention elsewhere. Require simultaneous open/writes, incompatible schema refusal, collector drain and consumer-query visibility tests. No data deletion is authorized.

## Runtime/download/cache

Delegated source trace: `internal/localruntime/llamaserver/provider.go:1438–1472` uses configured ModelDirs then ~/.cercano/models; `internal/modelhost/registry.go:33` defaults rendezvous to ~/.cercano/run/modelhost. `localruntime/manager.go:366–431` atomically admits downloads inside one manager; `:300–364` reloads configuration on restart. `localruntime/holds.go:10–29` describes caller ownership/epoch handling. `modelhost/server.go:651–684` dispatches download/cancel and `protection.go:30` rejects protected mutations. Full parity requires routing through existing shared authority, not retaining blanket denial. Independent managers cannot be presumed safe to write the same assets. Test configured paths, simultaneous download/cancel, file publication, reconnect and sibling-held resources. Other runtime-specific publication/cache details are execution obligations; no asset relocation is proposed.

## Config, credentials and enterprise visibility

Delegated source trace: `internal/server/config_watcher.go:8–17,35–43,85–96,179` already routes file changes through normal validation/apply/rebuild/broadcast and distinguishes hot reload. Reuse it while preventing stale saves and save-notification loops. `internal/enterprise/host.go:72–95,141–160` enforces connection locking; `keychain_darwin.go:12–13` uses service cercano-enterprise; `client.go:37,108–119,223–228,540–545` reads/caches/persists/deletes active connection credentials and `:471` renames bundle publication. Lock exclusion alone is not policy distribution. Do not weaken it; extend existing enforcement interfaces only where required.

Direct reads: `internal/chatgptauth/source.go:45–132` and `internal/anthropicauth/source.go:45–132` use a per-Source mutex, reread stored tokens each Token call, check read-only mutation policy, refresh externally and save rotated tokens. Cross-host refresh serialization is absent in those paths; process-local mutex does not protect rotating tokens. Reuse reread and storage seams, serialize mutations by credential identity, prevent refresh-after-logout resurrection and preserve ambiguous issuer outcomes. Login/logout and static-provider caching must participate in the same contract; exact callers become test coverage rather than a new inventory phase. Tests use memory stores and fake issuers, never native credentials.

## Backup and cleanup supersede earlier proposed state

See `backup-evidence.md`: authorized exact rejected capture deletion completed after identity checks; both Online Backup API copies completed and integrity validation passed within limits. Copies have no shared compared record IDs. Separate snapshots do not prove exclusive access or currency at live cutover. The older accepted development evidence remains untouched. The new sensitive backup directory is recorded in the evidence and retained; no further cleanup authorized.
