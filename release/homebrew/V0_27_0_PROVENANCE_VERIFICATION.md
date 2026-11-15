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
Requests are bounded GETs, with byte caps, socket timeout, read deadline and at
most three redirects. They do not read credentials, invoke `gh`, use proxies,
send cookies, or reuse publisher credentials. HTTPS redirects permit only exact
GitHub release-storage/Actions-storage hosts, never arbitrary subdomains, login
pages, userinfo, fragments, or downgrades.

**Availability is not assumed:** GitHub may require authentication for Actions
logs or artifact downloads even in a public repository. Missing, inaccessible,
expired, malformed or ambiguous evidence fails closed. No cached local archive,
HTML scraping, operator assertion or substring search substitutes for evidence.
The public archive alone cannot establish the retained-artifact chain. A full
live provenance pass has not been demonstrated here; credential-free access to
all retained job logs and the artifact ZIP is a known prerequisite, not waived.

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

Implementation check (2026-11-15): the full Homebrew Python discovery suite passed
91 tests. A credential-free live invocation of the validator returned the generic
fail-closed refusal, not validated evidence. `git diff --check` passed. No remote
publication or mutation was performed.
