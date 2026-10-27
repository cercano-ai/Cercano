# Bounded SQLite backup execution evidence

Date: 2026-10-27. User-authorized cleanup and bounded Online Backup API attempts only.

Outcome: Both backups and completed-copy validation finished.

## Method and boundaries

Source connections used SQLite URI `mode=ro`, normal locking, no immutable mode, and only the Online Backup API. No application opens, source SQL writes, explicit source read transaction, migrations, checkpoint, journal-mode changes, credentials/configuration contents, process inspection, or git operations. SQLite read-only WAL access can participate in shared memory/locking. Incremental 256-page steps release source read locks between steps; no transaction was held to force progress. A separate parent watchdog checked the common 10-minute deadline, free capacity, total destination size, and source WAL growth every 0.1 seconds, with terminate/kill of its own worker on cancellation. Busy retries were limited to 50 events with 0.1-second sleeps. Additional conservative WAL growth cap: 256 MiB above each observed initial size. A 4 MiB destination guard was reserved before the 24 GiB ceiling.

Completed destinations only were eligible for integrity checking, schema/count inventory, ID overlap and typed full-row SHA-256 equality comparison. No record IDs, message bodies, or individual row hashes are displayed. Separate backups are not a common-time snapshot. Interrupted copies, if any, are retained as incomplete; no extra deletion is authorized. Ordinary unlink is not secure erasure. The accepted older development evidence was untouched.

## Execution record and exact retained paths

```json
{
  "started_utc": "2026-10-27T19:43:50.401888+00:00",
  "limits": {
    "overall_seconds": 600,
    "initial_free_bytes": 32212254720,
    "free_floor_bytes": 10737418240,
    "destination_max_bytes": 25769803776,
    "source_wal_growth_max_bytes": 268435456,
    "backup_pages_per_step": 256,
    "busy_total_seconds": 5,
    "busy_sleep_seconds": 0.1,
    "watchdog_poll_seconds": 0.1
  },
  "cleanup": {
    "requested": "/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-db-evidence-20gdwqsu",
    "canonical": "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-db-evidence-20gdwqsu",
    "canonical_alias_note": "/var resolves to /private/var on this host; root and all descendants are not symlinks",
    "verified_metadata": {
      ".": {
        "device": 16777232,
        "inode": 77705150,
        "mode": "0o40700",
        "size": 160,
        "mtime_ns": 1793127539057131058
      },
      "legacy": {
        "device": 16777232,
        "inode": 77705151,
        "mode": "0o40700",
        "size": 128,
        "mtime_ns": 1793127539041411219
      },
      "development": {
        "device": 16777232,
        "inode": 77705253,
        "mode": "0o40700",
        "size": 128,
        "mtime_ns": 1793127539055814071
      },
      "manifest.json": {
        "device": 16777232,
        "inode": 77705256,
        "mode": "0o100600",
        "size": 3335,
        "mtime_ns": 1793127539057384722
      },
      "legacy/conversations.db": {
        "device": 16777232,
        "inode": 77705152,
        "mode": "0o100600",
        "size": 19236786176,
        "mtime_ns": 1793127527618189622
      },
      "legacy/conversations.db-wal": {
        "device": 16777232,
        "inode": 77705252,
        "mode": "0o100600",
        "size": 15656032,
        "mtime_ns": 1793127539045060640
      },
      "development/conversations.db": {
        "device": 16777232,
        "inode": 77705254,
        "mode": "0o100600",
        "size": 135168,
        "mtime_ns": 1793127539055256910
      },
      "development/conversations.db-wal": {
        "device": 16777232,
        "inode": 77705255,
        "mode": "0o100600",
        "size": 65952,
        "mtime_ns": 1793127539056389940
      }
    },
    "expected_sizes_source": "audit.md retained sensitive snapshots",
    "status": "deleted",
    "absent_after": true
  },
  "outcomes": {
    "legacy": {
      "status": "completed",
      "source": "/Users/bryancostanich/.config/cercano/conversations.db",
      "destination": "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/legacy.db",
      "steps": 34049,
      "busy_events": 0,
      "remaining_pages": 0,
      "total_pages": 4700016
    },
    "development": {
      "status": "completed",
      "source": "/Users/bryancostanich/.config/cercano-development/conversations.db",
      "destination": "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/development.db",
      "steps": 1,
      "busy_events": 0,
      "remaining_pages": 0,
      "total_pages": 33
    }
  },
  "retained_paths": [
    "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/development-backup-result.json",
    "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/development.db",
    "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/development.db-shm",
    "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/development.db-wal",
    "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/legacy-backup-result.json",
    "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/legacy.db",
    "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/legacy.db-shm",
    "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/legacy.db-wal",
    "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/validated-inventory.json"
  ],
  "initial_free_bytes": 1893877219328,
  "private_directory": "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s",
  "source_wal_initial_bytes": {
    "legacy": 15656032,
    "development": 65952
  },
  "source_wal_peak_bytes": {
    "legacy": 15656032,
    "development": 65952
  },
  "minimum_free_bytes": 1874610221056,
  "peak_destination_bytes": 19251466854,
  "validation_status": "completed",
  "inventory_path": "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/validated-inventory.json",
  "counts": {
    "legacy": {
      "autonomy_runs": 106,
      "conversation_chat_routes": 5,
      "conversation_compaction": 117,
      "conversation_context_usage": 58,
      "conversation_request_usage": 58,
      "conversations": 3885,
      "delegated_tasks": 597,
      "dispatch_events": 271548,
      "turns": 315418
    },
    "development": {
      "autonomy_runs": 0,
      "coexistence_metadata": 1,
      "conversation_chat_routes": 0,
      "conversation_compaction": 0,
      "conversation_context_usage": 0,
      "conversation_handovers": 0,
      "conversation_ownership": 1,
      "conversation_request_usage": 1,
      "conversations": 1,
      "delegated_tasks": 0,
      "dispatch_events": 0,
      "turns": 1
    }
  },
  "comparison": {
    "legacy_only_tables": [],
    "development_only_tables": [
      "coexistence_metadata",
      "conversation_handovers",
      "conversation_ownership"
    ],
    "tables": {
      "autonomy_runs": {
        "identity_columns": [
          "run_id"
        ],
        "same_id_count": 0,
        "legacy_only_id_count": 106,
        "development_only_id_count": 0,
        "hash_status": "schema differs; whole-row hash comparison skipped"
      },
      "conversation_chat_routes": {
        "identity_columns": [
          "conversation_id"
        ],
        "same_id_count": 0,
        "legacy_only_id_count": 5,
        "development_only_id_count": 0,
        "hash_algorithm": "SHA-256 typed length-prefixed full rows in schema column order",
        "same_id_equal_hash": 0,
        "same_id_different_hash": 0
      },
      "conversation_compaction": {
        "identity_columns": [
          "conversation_id"
        ],
        "same_id_count": 0,
        "legacy_only_id_count": 117,
        "development_only_id_count": 0,
        "hash_algorithm": "SHA-256 typed length-prefixed full rows in schema column order",
        "same_id_equal_hash": 0,
        "same_id_different_hash": 0
      },
      "conversation_context_usage": {
        "identity_columns": [
          "conversation_id"
        ],
        "same_id_count": 0,
        "legacy_only_id_count": 58,
        "development_only_id_count": 0,
        "hash_status": "schema differs; whole-row hash comparison skipped"
      },
      "conversation_request_usage": {
        "identity_columns": [
          "conversation_id"
        ],
        "same_id_count": 0,
        "legacy_only_id_count": 58,
        "development_only_id_count": 1,
        "hash_algorithm": "SHA-256 typed length-prefixed full rows in schema column order",
        "same_id_equal_hash": 0,
        "same_id_different_hash": 0
      },
      "conversations": {
        "identity_columns": [
          "id"
        ],
        "same_id_count": 0,
        "legacy_only_id_count": 3885,
        "development_only_id_count": 1,
        "hash_status": "schema differs; whole-row hash comparison skipped"
      },
      "delegated_tasks": {
        "identity_columns": [
          "id"
        ],
        "same_id_count": 0,
        "legacy_only_id_count": 597,
        "development_only_id_count": 0,
        "hash_algorithm": "SHA-256 typed length-prefixed full rows in schema column order",
        "same_id_equal_hash": 0,
        "same_id_different_hash": 0
      },
      "dispatch_events": {
        "identity_columns": [
          "conversation_id",
          "seq"
        ],
        "same_id_count": 0,
        "legacy_only_id_count": 271548,
        "development_only_id_count": 0,
        "hash_algorithm": "SHA-256 typed length-prefixed full rows in schema column order",
        "same_id_equal_hash": 0,
        "same_id_different_hash": 0
      },
      "turns": {
        "identity_columns": [
          "id"
        ],
        "same_id_count": 0,
        "legacy_only_id_count": 315418,
        "development_only_id_count": 1,
        "hash_algorithm": "SHA-256 typed length-prefixed full rows in schema column order",
        "same_id_equal_hash": 0,
        "same_id_different_hash": 0
      }
    }
  },
  "elapsed_seconds": 56.321,
  "retained_directory": "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s"
}
```

## Validated result summary

- Both completed copies passed `PRAGMA integrity_check` (`ok`). One backup call per source; no busy events and no retries. All work, including completed-copy inspection, took 56.321 seconds.
- Legacy: 3,885 conversations, 315,418 turns, 597 delegated tasks, 271,548 dispatch events. Development: one conversation and one turn. Full per-table counts appear above.
- No shared IDs were found in any common table (composite primary keys used where declared). Consequently there were no same-ID row pairs to hash; equal/different hash counts are zero for compatible schemas, and incompatible whole-row schemas are explicitly marked skipped. This is not a claim of full cross-schema content equality.
- Development has three additional tables: `coexistence_metadata`, `conversation_handovers`, `conversation_ownership`. Full schema SQL and column metadata are retained in `validated-inventory.json`, with no record contents.
- Source WAL sizes showed no sampled growth: legacy 15,656,032 bytes; development 65,952 bytes. Initial free space was 1,893,877,219,328 bytes; minimum observed free space 1,874,610,221,056 bytes. Peak observed destination total was 19,251,466,854 bytes, below 24 GiB. Limits were not reached; no blocker or escalation. Sampling is not a continuous measurement.
- The backup directory is mode `0o700`; retained artifact metadata below was checked after validation. Read-only inspection created destination WAL/SHM sidecars; they are intentionally retained, not cleanup-authorized.

```json
{
  "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/development.db-shm": {
    "bytes": 32768,
    "mode": "0o600"
  },
  "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/development.db-wal": {
    "bytes": 0,
    "mode": "0o600"
  },
  "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/legacy.db-shm": {
    "bytes": 32768,
    "mode": "0o600"
  },
  "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/legacy.db-wal": {
    "bytes": 0,
    "mode": "0o600"
  },
  "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/legacy-backup-result.json": {
    "bytes": 303,
    "mode": "0o600"
  },
  "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/validated-inventory.json": {
    "bytes": 55306,
    "mode": "0o600"
  },
  "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/legacy.db": {
    "bytes": 19251265536,
    "mode": "0o600"
  },
  "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/development.db": {
    "bytes": 135168,
    "mode": "0o600"
  },
  "/private/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-online-backup-c2u6nm9s/development-backup-result.json": {
    "bytes": 311,
    "mode": "0o600"
  }
}
```

## Previously retained evidence (untouched)

The accepted development capture remains at `/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-dev-evidence-p7if48qy` (as documented in `audit.md`; not reopened or modified during this execution), containing the previously inventoried `conversations.db`, `conversations.db-wal`, `query/conversations.db`, `manifest.json`, `safe-record-inventory.json`, and `schema-inventory.json`. No deletion of that directory or any live path was performed.
