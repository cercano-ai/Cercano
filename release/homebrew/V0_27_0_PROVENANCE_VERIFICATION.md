# Bounded v0.27.0 two-stage provenance verification

`pinned_provenance.py` is a read-only evidence validator, not a release acceptance
or publication switch. It is deliberately separate from `check_release_run.check`:
that general checker still requires the workflow source to equal the tag source
and successful publication steps. The exceptional historical two-stage run must
not weaken recovery checks for other releases.

## Fixed scope and chain

Only `cercano-ai/Cercano`, `v0.27.0`, source
`c91699b079164138bff083206594f49c56dd5509` is in scope. Workflow implementation
`4b3cd19b4e505239c7a875952a7bb40fd46754e0` is independently pinned by raw workflow
content SHA-256 `c69cdfb23708b02414fc2067f9e6c5cc83560e61cd594ffe41ae1dd5b88a5be8`.
The dispatch workflow is `.github/workflows/release-macos.yml`, run `43655050601`,
attempt **2**. It built the tagged source rather than its own workflow commit.
The validator requires attempt-specific job identities, successful test/build,
signing/notarization/archive-verification/upload steps, and skipped publication
steps, plus source-checkout and source-validation output records from job logs.
It also requires the successful Linux/Windows jobs from attempt 1 and the
attempt-2 Linux/Windows prerequisite results. A latest-jobs view is not used.

Linux executed job `132631632452` (attempt 1) is reused as `132634405095`
(attempt 2); both report `2026-11-14T23:48:25Z`–`2026-11-14T23:50:06Z`.
Windows executed job `132631632597` is reused as `132634438965`; both report
`2026-11-14T23:48:26Z`–`2026-11-14T23:52:02Z`. These are not independent
executions. The validator pins the original execution intervals, requires exact
agreement of steps, interval, workflow SHA, run, name, status and conclusion
between the two attempt-specific responses, and validates logs at the original
executed IDs only. It never invents or requires independently executed alias logs.

The macOS upload log must bind artifact `14139226701` to its name, run, byte
size `45630515`, and outer ZIP SHA-256
`89a85b1902925be5c4fadb67d5088bacc29fb29eb8d248663536cb64ecdc7bf6`.
Artifact metadata must agree and must not indicate expiry. The ZIP digest is
checked before parsing. Only the canonical archive, checksum, and three formula
files are allowed in the ZIP; paths are never extracted.

The inner `cercano-0.27.0-darwin-arm64.tar.gz` must be `45728133` bytes with SHA-256
`18c68ccc69662da6d18b8017a3eba7eaebc67e057065b90bb66deba2838d2aa0`. Its checksum
must be exactly the lowercase digest, two spaces, basename, and one newline.
Existing archive-layout/runtime-inventory validation is reused, additionally
requiring the complete canonical executable and metadata file inventory.
Published release `445713847` and archive asset `749310766` must match their
identities and digest/size metadata. Public archive bytes must equal the retained
inner archive byte-for-byte; its public checksum must match exactly too.

## Transport and failure behavior

Run manually with `python3 release/homebrew/pinned_provenance.py`. Success returns
structured JSON evidence with `publication_authorized: false`. Failure emits only
a generic refusal, not URLs, signed query strings, tokens or response bodies.
Requests are GET-only with finite output caps. Each transport runs in a direct
child process killed and waited on at the **120-second monotonic wall deadline**,
covering DNS, TLS, redirects, blocked reads and slow trickles. This is direct-child
cleanup, not process-tree cleanup. Stdout is consumed through a nonblocking pipe
with backpressure, reads of at most 64 KiB and at most the remaining `limit+1`
allowance, and at most `limit+1` accumulated bytes. Exceeding the cap refuses
immediately, kills/waits the direct child and closes pipe/selector handles. EOF
still requires a successful child exit within the original deadline. There is no
stdout disk spool. Public HTTP additionally has a
20-second socket inactivity timeout (not a wall deadline), at most three
redirects, and deadline checks after opening and after every read, including EOF.
The reviewed `eb59a174fde8` defect was reproduced: a mocked read moved monotonic
time from 0 to 1000 and returned EOF, which the old before-read-only check
accepted. The regression now rejects both late EOF and late nonempty blocks;
a real sleeping child is killed by the parent-deadline test.

Only logs for executed jobs `132631632452`, `132631632597`, `132634405380` and
artifact `14139226701`'s ZIP use existing authenticated `gh api --hostname
github.com --method GET` at fixed repository/ID endpoints. No token is extracted,
printed or written; diagnostics are suppressed. `gh` scopes authentication to
GitHub API hosts, and its Go HTTP redirect handling strips Authorization on
unrelated signed-storage domains. Public metadata, workflow and release downloads
use the separate credential/cookie/proxy-free urllib transport, never `gh` or
publisher credentials. Its HTTPS redirects permit only exact release/Actions
storage hosts and rebuild requests without Authorization/Cookie headers; login
pages, arbitrary subdomains, userinfo, fragments and downgrades refuse.

**Exact prior refusal:** credential-free Actions job-log requests returned HTTP
403 ("Must have admin rights to Repository."), and the artifact ZIP API returned
HTTP 401 ("Requires authentication"). Metadata/public release assets returned
200 and the public archive digest was correct. The old generic refusal was
therefore an unavailable authenticated CI-evidence transport, **not** a public
archive digest mismatch or proof of failed CI. The old implementation also
incorrectly required logs at reused alias IDs, rejected actual upload-path
continuation records and expected `Artifact ID:` where the actual finalized
record says `Artifact ID` without a colon. Actual-response fixtures preserve
these distinctions without accepting arbitrary untimestamped text.

Missing, inaccessible, expired, malformed, changed or ambiguous evidence still
fails closed. No local cache, operator assertion, HTML scraping or substring
search replaces live evidence. The public archive alone cannot establish the
retained-artifact chain.

## Trust and limits

This is a GitHub CI attestation trust model: trust GitHub's repository/run/job
metadata, protected signing workflow and its successful steps, immutable pinned
workflow/source content and digest collision resistance. It is not an independent
rebuild or an independent cryptographic verification of signing/notarization.
The Apple ticket-comparison evidence was **not retained** with this release, so
that comparison cannot be independently repeated. The validator reports this
explicitly; it does not reconstruct an Apple ticket from CI step success.
There is **no clean-Mac qualification claim**, nor any claim that a renderer's
layout checks establish signatures, architecture or runtime behavior.

The coordinated publisher requires this validator after plan validation and
before token validation, tap reads or mutations if its internal path is reached.
The ordinary public/CLI entry points still refuse immediately because
`PUBLICATION_ACCEPTED = False`. The existing historical transition, branch,
plan, credential, atomicity and reconciliation guards are retained. Evidence is
never publication authority.

## Offline verification

`python3 -m unittest discover -s release/homebrew -p 'test_*.py'`

Fake-network tests generate inert archives on every run (no local-archive skip),
substituting only synthetic byte pins to exercise the positive chain. Negative
cases cover identity/attempt/evidence changes, missing/duplicate/failed steps,
non-record/ambiguous logs, digest/workflow/public-byte changes, checksum grammar,
ZIP paths, inaccessible evidence, expiry, and unsafe/excessive redirects. Publisher
tests mock only the provenance boundary for legacy state-machine scenarios and
verify that a provenance refusal precedes any tap call without leaking secrets.
These tests establish fail-closed implementation behavior, not actual historical
provenance acceptance or clean-host qualification.

Implementation check (2026-11-15):

- Re-fetched exact attempt-1/2 metadata and all three original executed logs
  through authenticated read-only `gh` GETs. The sanitized metadata projections
  matched exactly; each log fixture matched an ordered excerpt of its live log.
- Full Homebrew Python discovery suite: **98 tests passed**, including publisher
  guards, actual-response fixtures, changed/reused evidence, bounded subprocesses,
  late EOF/nonempty reads, credential-free public routing and header stripping.
- Actual full `python3 release/homebrew/pinned_provenance.py`: **exit 0**, live
  public metadata/workflow/release bytes plus authenticated CI logs/ZIP; no cached
  evidence supplied. Result: `ci_evidence_validated: true`,
  `publication_authorized: false`, `clean_mac_qualified: false`,
  `apple_ticket_independently_repeated: false`. Run `43655050601`, attempt 2,
  release `445713847`, artifact `14139226701`, asset `749310766`; outer SHA-256
  `89a85b1902925be5c4fadb67d5088bacc29fb29eb8d248663536cb64ecdc7bf6`,
  inner/public SHA-256
  `18c68ccc69662da6d18b8017a3eba7eaebc67e057065b90bb66deba2838d2aa0`.
- `git diff --check` passed. Publication/general-checker policy was not changed;
  no tap mutation, enablement, push, VM action or credential disclosure occurred.


Output-boundedness correction verification (2026-11-15):

- The reviewed tempfile/polling implementation could spool 8 MiB before refusing
  a 4-byte cap. It has been replaced with bounded pipe consumption, without
  changing the exact authenticated endpoint whitelist or public transport policy.
- A real Python subprocess attempts to emit 8 MiB at a 4-byte cap: the parent
  reads exactly 5 bytes before refusal, kills/waits the direct child, and closes
  stdout. Additional real-child regressions cover empty/exact-cap EOF, multi-chunk
  success, nonzero exit, blocked-read timeout, EOF without exit and slow trickles.
  Tests check bounded read sizes, accumulated byte counts and direct-child cleanup.
- Full offline Homebrew Python suite: **103 tests passed**. Ruby formula suite:
  **5 tests, 40 assertions passed**, no skips.
- Exact read-only `python3 release/homebrew/pinned_provenance.py`: **exit 0**
  against live evidence, with `ci_evidence_validated: true` and
  `publication_authorized: false`; clean-Mac qualification and independent Apple
  ticket repetition remain false. No cache or synthetic pins supplied.
- Publication remains disabled. No remote writes, push, VM actions or credential
  output were performed. The correction makes no process-tree cleanup claim.
