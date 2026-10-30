# Native credential backend contract audit — 2026-10-28

Historical audit of the superseded pending/quarantine design. The approved reduced
contract removes the stronger backend requirements discussed below; they are not
current activation prerequisites. See [reduced coordination evidence](credential-reduced-coordination.md).
No native adapter or activation is supplied by that change.

Read-only source audit, no native API, security CLI, network, installed credential contents, or native tests.

`internal/hostsvc/credentials/coordinated.go:22–35` requires fresh linearizable reads, durable whole-record replacement, prior-record preservation on definite failure, and durable cross-instance/restart quarantine after uncertain replacement. CRUD is explicitly insufficient. The opt-in implementation remains inactive for native stores.

Pinned dependencies: github.com/99designs/keyring v1.2.2 and go-keychain v0.0.0-20191008050251-8e49817e8af4. Source inspected in the existing Go module cache.

- keyring `keychain.go:158–164,196–208` writes one item and uses update on duplicate; `:116–143` updates through go-keychain. It does NOT delete the item before replacement.
- go-keychain `keychain.go:329–343` calls SecItemUpdate and translates status. These wrappers do not establish universal old-value preservation, crash durability, or definite/uncertain outcome classification.
- keyring `keychain.go:45–79` queries afresh, without a Go-side cache, but this is not a documented linearizability guarantee. Empty results are mapped to not-found before other query errors; absence cannot authorize adoption/creation.
- keyring `keyring.go:99–111` exposes CRUD without these stronger contracts.
- Replacement failures leave no durable quarantine state in this adapter. A later Get independently queries the old/new record. A process-local error flag, read-back, rollback, or another uncertain write does not establish the required persistent uncertainty fence.
- Coordinated logout must retain the secret-free logged-out record through replacement. Native Remove physically deletes and is not that operation.
- Preserve service `cercano` and profile/account identity. Canonical versioned bytes fit the native item, but format compatibility is not behavioral qualification.

A direct CRUD wrapper is not qualified as RecordBackend. No alternative journal, relaxed failure semantics, new native identity, migration, or activation was implemented. Further work needs either documented native guarantees plus separately authorized isolated native qualification, or a human-approved recovery mechanism/contract revision. A test pass alone cannot prove crash durability or all ambiguous failure behavior.

Other remaining work is independent: coordinated worker request capabilities, supported discovery/diagnostics, Bedrock identity mapping, cross-host live-provider policy convergence, and full production acceptance are still incomplete. This audit does not mark Phase 3 or the run complete.
