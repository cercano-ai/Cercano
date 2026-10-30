# Focused ordinary dependency evidence — 2026-10-29

Companion to `ordinary-closure-reopen-2026-10-29.md`. No lifecycle redesign or generalized production certification. Findings below come from delegated read-only source inspection and focused offline tests in this review; native secrets, models and existing user databases were not accessed.

## Credentials and routing

Untagged `cmd/cercano/ordinary_generation_defaults.go:1-9` selects `productionCredentialDefaults`; `production_authority.go:44-63` selects the native keychain after credential admission. Ordinary startup attaches writable secrets (`main.go:658-690`). Provider construction validates flavor and credential admission, reads the profile credential, and invokes the builder (`internal/hostsvc/providers/providers.go:428-481`). The tagged rehearsal substitutes these dependencies and does not prove keychain execution.

Focused tests passed for credential refusal before backend access, fake-backend read/write/list/delete, missing-key refusal before provider construction, injected builder selection, and provider flavor construction. No live inference is necessary for these routing assertions.

Remaining boundary: macOS keychain permissions/prompts were not tested. A native smoke test would require separate authorization for a dedicated synthetic keychain entry and cleanup; changing HOME does not isolate the system keychain. This is not evidence of a native-backend defect.

## Runtime

`cmd/cercano/production_runtime.go:35-94` selects ordinary runtime dependencies, permits in-process operation and local fallback on remote failure, warms a successful remote connection and once-guards cleanup. `main.go:945-961` supplies a host launch callback without requesting coexistence protection. The separate connector in `development_host_production.go:46-75` requests protection and prohibits launch/replacement/fallback.

Focused tests passed for fallback/cancellation, independent cleanup, admission ordering, protected-server denials and protected Ensure refusing replacement. These establish the tested control paths, not actual loaded-model operation. Crucially, protected attachment is not the ordinary default: a protected-fixture result cannot certify shared-runtime protection for ordinary A/B hosts. No runtime architecture or configuration policy was changed in this task. Native loaded-model readiness was not exercised and is not required to prove routing selection.

## Existing stores

`internal/conversation/store.go:390-458` ordinary Open performs schema/migration writes. `internal/conversation/coexistence.go:38-108` exclusively prepares a new private store with ownership schema and declaration metadata; `:111-175` preflights without migration and revalidates coexistence opening. `TestDevelopmentStoreRefusesLegacyAndAliases` passed on disposable databases.

Concrete blocker: the current preparation APIs do not adopt an existing ordinary database. Supported acceptance uses a newly prepared private destination. Preserving pre-existing ordinary history would require a separately specified/tested import or adoption operation on a disposable copy; manually stamping metadata or opening the real database is not a safe substitute. No actual user database was inspected or changed. History preservation across A/B within the prepared store is proven separately by the CLI rehearsal, not legacy-history adoption.

## Delegated test record

Commands ran from `source/server`, without fixture tags, through `env -i` with preserved PATH and explicit pre-resolved Go caches, disposable HOME/XDG/TMPDIR under `/tmp/cercano-focused-audit.iE7dlb`, and `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local CERCANO_DISABLE_CREDENTIALS=1`. Tests enabling credentials injected fake backends.

All passed:

```sh
go test ./internal/cloudfactory -count=1 -run 'TestBuild(Messages|ChatCompletions|Responses|Unsupported|Bedrock)'
go test ./internal/modelhost -count=1 -run 'TestProtectedServerDenials|TestProtectionRequestEnforcedAtServer|TestProtectedEnsureNeverReplaces'
go test ./internal/conversation -count=1 -run 'TestDevelopmentStoreRefusesLegacyAndAliases'
go test ./cmd/cercano -count=1 -run 'TestProductionCredential|TestProductionRuntime'
go test ./internal/secrets -count=1 -v -run 'TestCredentialGate|TestMemoryStoreRoundTrip|TestMissingCredentialHasPortableSentinel'
go test ./internal/hostsvc/providers -count=1 -v -run 'TestCloudBuilderInvocationAndCredentialBoundary|TestBuilderCredentialAdmission'
```

No live-provider calls, model loads/downloads, native credential mutation, or real-history migration was performed. Disposable audit artifacts remain for review.
