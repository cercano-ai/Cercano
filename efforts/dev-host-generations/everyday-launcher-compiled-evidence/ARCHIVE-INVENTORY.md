# Completed diagnostic evidence archive

This inventory covers only the four old, completed diagnostic trees below.
These failed/blocked historical runs are not fresh acceptance results.

## Private originals and provenance

- Archive: `/tmp/cercano-completed-evidence-2lxzd2g_` (local, temporary; not committed).
- Original files: `originals/<repository-relative-path>`, byte-for-byte verified.
- Archive directories mode `0700`; files mode `0600`.
- `manifest.json` records each original path, byte length, SHA-256, tracked status,
  scope roots, and source HEAD `026cdf9c577a66a117fbcd1fdc2163fa06065969`.
- `manifest.json` SHA-256: `7998a05f579dcc293851a18cb98060c18e7ed22bf8f7306d7e476706a864b854`.
- `SHA256SUMS` SHA-256: `45fa0b3fa7db115191c616e9e3ecaf0a2be768ce5a63ef265be0e9205cbc04ce`.
- Verify originals privately with `shasum -a 256 -c SHA256SUMS` from the archive.
  Do not publish the originals: they contain synthetic fixture bearer material.

| Original tree | Archived files | Tracked retained | Untracked relocated |
| --- | ---: | ---: | ---: |
| `efforts/dev-host-generations/everyday-launcher-compiled-evidence` | 5 | 5 | 0 |
| `efforts/dev-host-generations/compiled-after-ec7378` | 50 | 0 | 50 |
| `efforts/dev-host-generations/compiled-permission-blocker` | 15 | 4 | 11 |
| `source/server/efforts/dev-host-generations/compiled-after-ec7378` | 1 | 0 | 1 |

## Repository disposition and secret check

All four paths existed when archived. All 71 originals were verified before
relocation. The 62 untracked files (including six synthetic `.token` bodies,
selected-host JSON copies and diagnostic logs) now reside only in the private
archive, preventing accidental broad staging. The full per-file inventory and
original hashes are in the private manifest; no raw credentials are in this note.

The nine tracked files were checked against all six archived token bodies and
for bearer/authorization/credential assignments. No tracked bearer values or
token bodies existed in this snapshot; tracked evidence remains byte-for-byte
unchanged. TokenFile path references are not token bodies and were retained.
If future copied evidence needs redaction, use the constant
`[fixture-token-redacted]`, never a partial secret. No historical Git rewrite
was performed; this check describes the current files, not all past commits.

Only this inventory is added to Git. No active acceptance artifacts, source,
installed binaries, real-user configuration or credentials were inspected or
modified. The archive is private local evidence, not durable remote storage;
retain it securely for any required audit before temporary-directory cleanup.
