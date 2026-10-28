# Isolated credential slices — 2026-10-28

## Review boundary

Implemented opt-in canonical version-1 records at existing profile identities with active payload or secret-free logged-out/pending state and random revision. No raw fallback, automatic initialization/adoption, general operation journal, native adapter, production wiring or activation. Short commit and separate refresh exclusion reuse filelock; issuer work runs outside commit exclusion. Successful durable logout replacement is the boundary for participating request-start admission. Already dispatched requests and remote revocation are excluded.

RecordBackend is an explicit stronger contract, not a CRUD adapter or native CAS claim: fresh linearizable reads, durable whole-record replacement, definite failure preserving prior data, and uncertain replacement quarantining the identity across instances/restarts until reconciliation. The memory fake exercises coordinator responses to these guarantees; it does not establish native durability. Failed pending publication prevents issuer invocation; uncertain issuer outcomes retain pending and are not replayed. Ambiguous post-mutation replacement is tested through a reopened coordinator view of the fake quarantine.

Service/login snapshots and conditionally commits shared revisions. Hot replacement cannot downgrade/rebind coordinated authority. Config propagates replacement failure. Legacy token-source acquisition refuses coordinated stores until provider integration exists. Unconditional Set is an explicit credential edit, not an API for publishing delayed login/refresh results; those must carry expected revision.

## Verification

Executed from `source/server`, every command with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

- `go test -race ./internal/hostsvc/credentials -run '^TestLegacyTwoServiceResurrectionProbe$' -count=10` — PASS. Retained diagnostic asserts legacy resurrection, not a fix.
- `go test -race ./internal/hostsvc/credentials -count=10` — PASS, including strict decoding, secret-free marker/presence, stale refresh/login, independent logout, separate refresh serialization, request-start exclusion, uncertain issuer no replay, definite failure and uncertain replacement quarantine.
- `go test -race ./internal/hostsvc/config -run '^TestCredentialOwnerSurvivesBackendReplacement$' -count=10` — PASS; neighboring owner/view integration regression.
- `go test ./internal/hostsvc/config ./internal/hostsvc/credentials ./internal/server -run '^$'` — PASS; compilation only, no server tests executed.

Only narrowly scoped Service tests enable credential policy with explicitly injected memory backends/fake resolvers. No child processes, native credential operations or real issuer calls. Tests use temporary lock directories. Two coordinators sharing OS advisory locks in one process are tested; cross-process/restart/crash durability qualification is NOT established. Full server/CLI suites, system acceptance and native qualification were not run.

## Exact remaining seams / activation blockers

See `credential-request-audit.md` for source locations. Provider clients must retain identity/revision and fence **each actual request-start**, including cached keys/tokens, retries, redirects and discovery. `Admit` is tested at its synchronous dispatch callback boundary, not integrated into a network transport; merely queuing work or wrapping a whole blocking HTTP Do does not satisfy both short exclusion and dispatch fencing.

- Responses HTTP Do and authorize; Anthropic HTTP client and subscription auth hook; OpenAI normalizingDoer plus independent reasoning diagnostic dispatch.
- Anthropic/ChatGPT subscription Sources must route rotating refresh through separate serialization/pending state, reread after admission, and conditionally publish without blind retry. Legacy Service acquisition deliberately returns ErrProviderIntegration for coordinated stores.
- Bedrock/AWS default credential chain requires established native-profile identity mapping and coverage of credential-provider traffic, not only Converse calls.
- Cloud-model discovery HTTP requests, application/SDK retries, redirects, retained provider graphs and host-mediated worker admission remain uncovered.
- Qualify real backend replacement/failure/quarantine guarantees, stable private shared lock-root trust/preflight, process contention/restart/crash behavior, and exclusion of incompatible readers/writers before any separately approved adoption. No native adapter is supplied and adoption is disabled.

No entire Phase 3 completion claim; its credential task remains in progress. No push, merge, live migration, installer or launcher change, new service, or production activation.
