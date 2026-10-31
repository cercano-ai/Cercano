# Recognized-worker database gate follow-up

Scope: disposable processes/state only; no ambient signals, live configuration/database/installation changes, landing, or push. Existing untracked inventory evidence was left untouched.

## Reproduction and correction

A new native regression launched a `cercano agent` host plus a sibling `cercano worker` (the test parent, not the host, owns the worker). Before the production fix, `go test ./internal/brewrestart -run '^TestUpgradeHostWithUnrelatedRecognizedWorker$' -count=1` failed with `upgrade blocker pid=33026: no verified host/worker association`, before shutdown. An initial fixture-path setup mistake was corrected before this reproduction.

Discovery incorrectly required non-writing workers to have a discovered host parent and matching executable. It now excludes recognized worker/model-host roles from the writer gate without granting signal authority. Unknown roles still refuse. Native termination, child ancestry/executable checks, identity revalidation and verified exit remain unchanged.

The production post-stop `knownUpgradeAgents` filter now uses `IsUpgradeNonWriter`, not only model-host exclusion. Intact same-user known executable workers are excluded without ancestry. Missing-image known executables reuse the existing stable kernel lifetime/saved path/strict worker argv/Unix socket/no TCP listener evidence. That classifier already did not require ancestry; it needed no loosening. A missing image without that evidence remains blocking. Missing-image post-stop exclusions additionally require a known saved executable basename; arbitrary aliases are not generalized.

Additional host targeting previously recognized only `agent`. It now explicitly recognizes `development-host` too. Both are tested as additional listeners (not merely the configured endpoint), followed by native termination. Other unknown roles remain blockers. The configured known-name listener retains the pre-existing listener ownership authority. Additional hosts still require a uniquely identified supported listener; no inference from arbitrary argv or names grants signal authority.

## Verification

Final commands (from source/server), all passed:

- `go test ./internal/brewrestart -count=1`
- `go test ./cmd/cercano -run 'TestAutomaticUpgradeStopThenCheck|TestOfflineUpgrade' -count=1`

Also ran the focused native worker/development-host regression and real production worker missing-image tests verbosely. The latter builds the untagged production entrypoint and verifies intact and unlinked worker exclusions, malformed role/socket refusals, then reaps the owned child.

The new integrated disposable regression, for both agent and development-host roles, verifies exactly one discovered target (the host), native host ESRCH before backup/migration, surviving sibling worker identity and continued progress, post-stop non-writer classification, a disposable closed-store backup, real UpgradeOffline migration and compatible reopen. The sibling never enters the shutdown target/descendant set. The fixture host does not itself write the test database: this is a native-stop + real-store sequence regression, not another full historical-writer acceptance journey or a syscall trace of every delivered signal. Existing command tests separately verify stop-before-check ordering and refusal before database access when shutdown/check fails. No full inventory or two-host acceptance journey was repeated.

Limits remain deliberate: bounded same-user known executable detection is not a writer fence against renamed/custom/other-user/newly launched writers; consent must keep legacy writers stopped. Future unknown roles, ambiguous listeners, unreadable/changing identities and insufficient missing-image worker evidence remain refusals. Non-Darwin builds remain unsupported for native shutdown/non-writer classification. This change does not make worker provenance or session idleness a database safety prerequisite, and does not authorize signalling unrelated workers.
