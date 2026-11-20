# Offline database upgrade

## Approved scope

On 2026-11-19 the user approved building an upgrade for existing chat databases. Development and tests use temporary databases only. No actual user database has been migrated. This is a persistence prerequisite, not completion of client-side execution.

## Implemented storage change

Production declaration advances from `cercano-development-owned-v2` / schema 2 to `cercano-development-bindings-v3` / schema 3. A compiled-in offline migration adds `execution_bindings` and updates the declaration and SQLite `user_version` in one transaction.

The new table stores conversation, host/client location, executor ID, workspace ID/root, and positive ownership generation. Constraints enforce structure, not authorization. Roots are opaque executor-local paths. No binding write API or execution routing is added.

**Existing conversations receive no binding rows and retain current host behavior.** No identities are inferred from `project_dir`, `dev_work_dir`, or delegated task directories. Existing history, task statuses, and host ownership remain unchanged. The fixture declaration `cercano-dev-fixture-owned-v2` remains separate and unchanged.

New production stores and ordinary unmarked stores get the table. Existing declared stores still refuse mismatched versions at startup; startup never silently upgrades them. Unknown/inconsistent declarations and incompatible same-name binding tables are refused. Current-version offline calls are idempotent.

## Safety contract

All writers must be stopped and remain stopped through backup, migration, and compatible restart. CLI process checks and writer admission are safeguards, not a guarantee against renamed/custom or subsequently launched writers.

Before either declared-version migration or unmarked adoption mutates the source, the public API creates `<database>.pre-upgrade-backup` exclusively with private permissions. SQLite `VACUUM INTO` captures committed data, including data still in the write-ahead log. Platform-specific durability barriers precede migration: file and parent-directory sync on Unix; private same-directory staging, snapshot flush/close and non-replacing write-through publication on supported Windows local NTFS. Existing backups are never overwritten; backup errors refuse mutation. Migration errors roll back the schema and both version markers together.

A failed backup can leave an incomplete reserved file. Inspect and archive/remove it explicitly before retrying; do not assume it is usable. Upgrading an already-current store needs no new backup. Declared upgrades preserve task statuses even with `--interrupt-tasks`; that option reconciles unfinished work only during unmarked legacy adoption. CLI output documents this distinction.

**Current platform boundary:** macOS regression and native Windows ARM64 upgrade selections passed. Windows supports manual offline upgrade only with absent/empty host inventory, validated private paths/ACLs and local NTFS backup publication. Retained hosts (even apparently stopped/renamed), unknown/noncanonical entries, uncertain known-process identity/liveness, automatic shutdown (`--stop-hosts-config`) and development lifecycle remain fail-closed. All writers must still be stopped by the operator; this is not a universal writer fence. No Linux runtime or physical power-loss qualification is claimed. See [Windows evidence](windows-upgrade-evidence.md) for exact commands, results and two existing skips.

## Verification

Main-agent run from `source/server`, 2026-11-19, PASS:

```
go test ./internal/conversation ./cmd/cercano ./internal/brewrestart ./internal/server -count=1 -timeout=120s
```

Tests include a frozen production-v2 DDL fixture, production public-API preservation checks, CLI/startup refusal→upgrade→admission round trip, rollback, idempotence, unknown/fixture declaration refusal, schema collision refusal, backup non-overwrite, injected file/directory-sync failures, and a live committed-WAL backup. The frozen DDL was captured from the existing v2 initializer, not an external production database archive.

Earlier baselines passed for server context/worker/dispatch/server and CLI UI packages. These results do not establish client execution acceptance. Broader operation inventory, transport, routing, and client executor remain unfinished.
