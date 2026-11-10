# Recovery cleanup verification — Phase 2

## Authorization and baseline

Phase 2 only, under the user-approved plan. Read root AGENTS.md and CLAUDE.md, spec.md, plan.md and Phase 1 verification.md. The spec's proposed-status language is historical; the explicit execution request authorizes this phase. Baseline HEAD: `d8f7fad9ff238988f1cd4d12cb57cffc4925b10f`. Initial scoped-checkpoint status showed only this phase's first semantic plan-status edit; no unrelated modifications were present. Semantic task transitions used throughout. Go: `go1.26.3 darwin/arm64`.

Repository-wide search for `reaffirmResumeWithSync|ReserveTakeover` reconfirmed the Phase 1 map: reaffirmation had only its definition/comment in resume.go; takeover had only its definition/comment and calls in its own five top-level tests. Other matches were historical effort prose and the plan, not executable consumers. No production consumers, interface adapters or generated consumers were found. Scoped pre-commit baseline inspection `git grep -n -E 'reaffirmResumeWithSync|ReserveTakeover' HEAD -- source` independently confirmed that exact source consumer map. After removal, the same search under `source` returned no matches. Historical prose is preserved.

## Assertion inventory and deletion boundary

- `TestReserveTakeoverOwnedAndRetry`: source unusable/unlocked, exact reserved row, stale writer refusal and source-qualified claim are relevant; existing active reservation/transition tests retain row, epoch, stale-write and claim coverage. Added source unusable/unlocked and nonmutating rejection assertions in `TestOwnershipReservationRejectsReusedOperation`. Exact takeover retry/same-ID destination supersession behavior belonged only to the deleted primitive, not the active reservation API, so was not imposed on it.
- `TestReserveTakeoverReservedAndCompetingClaim` and `TestReserveTakeoverConcurrentOldClaim`: tested the deleted primitive superseding a pending reservation versus an old claim, including nonblocking reacquisition and durable stale checks. Those supersession/race outcomes have no active API equivalent and were removed, not translated into new production behavior. Active `TestOwnershipTransitionsReservationAndAbandonment` retains stable-lock exclusion, immutable rejected claims, epoch transition, stale mutation refusal, receipt-only retry and abandonment checks. `TestOwnershipSourceQualifiedClaim` retains exact source host/epoch refusal with unchanged reservation and historical receipt refusal after release.
- `TestReserveTakeoverWaitsForWriter`: unique useful committed-writer ordering assertion migrated essentially verbatim to `TestOwnershipReservationWaitsForWriter` using active `ReserveHandover`: reservation must not pass an open writer transaction, must complete after commit, must reject the stale writer, and must preserve the committed title.
- `TestReserveTakeoverInvalidAndHistory`: active explicit-reservation tests already reject missing operation and self-destination. Added pending operation collision across conversations, completed operation reuse on both another conversation and the original conversation at a fresh epoch, stale-capability reuse refusal, and complete before/after row equality after refusal. Deleted primitive-specific nil/malformed receiver, closed receiver, and reserved-to-reserved retry tests rather than expand active API contracts.
- No tests invoked the unused reaffirmation helper. No journal durability assertion was deleted. Existing selected resume tests retain uncertain-publication identity, acceptance-retirement, authentication/owner-race, interrupted retry, current readiness and exact completed-operation checks.

Production diff is deletion only: removed the 59-line unused reaffirmation helper; deleted ownership_takeover.go (109 lines). Deleted its obsolete test file (169 lines) after adding 68 lines of relevant active-API tests to ownership_reservation_test.go. `saveResume`, `saveResumeWithSync`, `syncResumePublication`, active cross-host durability barriers, ownership implementations, lifecycle dispatch/consent, stop interfaces, protobuf schema and generated wire/API files are untouched. No behavior change to any active path is intended or introduced.

## Exact checks and results

All Go commands below ran from `source/server` unless specified. Focused commands are anchored by a nonempty observed selection, not just exit status. A Python JSON-event summary counted completed test events and top-level tests; no skip/fail events occurred.

```sh
go test ./internal/conversation ./internal/server -run 'Test(Ownership|DestinationResume|Resume|Original|LiveRecovery|LiveSourceRecovery)' -count=1 -json -timeout=180s
go test ./internal/conversation ./internal/server -run 'Test(Ownership|DestinationResume|Resume|Original|LiveRecovery|LiveSourceRecovery)' -race -count=1 -json -timeout=180s
```

Both exit 0. Each run: conversation **16 top-level tests, 122 pass events including children**; server **18 top-level tests, 67 pass events including children**; zero skip/fail events. Normal package times: 0.476s / 2.296s; race: 6.574s / 7.131s. Combined command elapsed 16.254s. This includes both added reservation tests, source-qualified claim, transitions/abandonment, mutation fences and path aliases; server includes LiveSourceRecovery, consent mutation, lookup failure, all DestinationResume tests, new-destination interrupted transfer, ordinary-before-contained and ready retry identity.

A preliminary race run used the same selection without `|LiveSourceRecovery`: exit 0 (14.205s command), 122 conversation / 62 server pass events. Final expanded normal/race selection above is the phase evidence. Earlier execution history records a failed command launch because `/opt/homebrew/bin/go` was unavailable in that command environment, followed by a successful retry; all final commands here use absolute in-worktree cwd. No failed test was masked.

```sh
# source/server: compile all default-tag packages and test files, no selected tests
go test ./... -run '^$' -count=1
# source/clients/cli: same compile-only check against local server module
go test ./... -run '^$' -count=1
```

Both exit 0 (46.739s server; 4.782s CLI). Server compile emitted macOS Security framework deprecation warnings in cmd/keychain-read-diagnostic; no errors. These are explicitly compilation checks, not full-suite passes or keychain diagnostic execution.

From repository root:

- `gofmt -w source/server/internal/conversation/ownership_reservation_test.go source/server/internal/server/resume.go`, followed by `gofmt -l` on both: no outstanding formatting.
- Source reference search `reaffirmResumeWithSync|ReserveTakeover`: no matches.
- `git diff --check`: exit 0.
- Scoped diff review confirms resume.go only loses the dead helper; two takeover files deleted; only active test additions otherwise. No active durability barrier or wire/API behavior edit. Plan changes limited to Phase 2.

## Limitations and checkpoint scope

No full test suite, tagged historical/native acceptance, native process containment runtime qualification, live data/host, user credentials, VM, deployment, merge or push. Test stores are disposable fixtures; server tests use fixture transports/adapters. Race testing does not prove all schedules, and migrated writer timing test retains its original bounded 50ms/5s checks. Deleted takeover-specific supersession race coverage is not claimed as active API coverage. Compilation does not qualify deployment or platform runtime behavior.

Scoped checkpoint includes only plan.md Phase 2 statuses, this record, resume.go, ownership_reservation_test.go, and deletion of ownership_takeover.go / ownership_takeover_test.go. All later phases remain pending. Checkpoint SHA is reported after commit rather than self-referenced here.
