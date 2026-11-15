# Coordinated Homebrew preparation — bounded verification

Date: 2026-11-14. Branch: `fix/homebrew-promotion-ready`.
Base: `4b3cd19b4e505239c7a875952a7bb40fd46754e0`.

## Diagnosis and boundary

The renderer and review artifact already supply three sibling formulas, while
both publication workflows still invoke the historical single-formula updater.
The updater and local promoter unconditionally refuse promotion. The release
build guide nevertheless described automatic single-file publication and gave
a recovery dispatch command. That obsolete operational guidance is replaced.

Selectively ported the six-file preparation slice from
`9a06577f5d68c9453edad71c169fa8643f022e81`, not its branch history. No workflow,
recovery provenance code, renderer, formula template or publishing guard changed;
current main's recovery changes are retained. The preparer additionally records
an optional caller-supplied full tap base SHA, validated syntactically but not
authenticated by this offline tool.

`prepare_promotion.py` validates the archive, exact canonical sibling set and a
private tap snapshot before emitting a review-only plan. It records content,
before/after byte hashes and update/no-op decisions for all three paths. A later
separately authorized publisher still needs a genuine single-commit transaction
with base compare-and-swap and public asset/provenance checks. No such live
transaction is enabled. Do not dispatch the release workflow: it has non-tap
publication actions too.

Historical distribution is established: prior API audit found official tap
v0.25.2 monolith and personal tap v0.20.3. The fresh official snapshot below
confirms v0.25.2 owns both binaries and has a post-install restart hook. This is
not proof that any particular user installed it. Installed-user applicability
and native migration/failure recovery remain untested. The README documents
explicit operator-only rehearsal, preserved old kegs/state, partial-failure
rollback and post-convenience snapshot recovery. No automatic migration hook,
unlink, overwrite, removal or expanded runtime guarantee was added.

## Local tests

Original preparation-slice results (historical, before the review fixes below):

| Command | Result |
| --- | --- |
| `python3 -m unittest discover -s release/homebrew -p 'test_*.py'` | 48 tests |
| `ruby release/homebrew/test_formula.rb` | 5 tests, 40 assertions |
| `python3 scripts/test-release-workflow.py` | 51 tests |
| `python3 scripts/test-macos-release-build.py` | 12 tests |
| `python3 scripts/test-macos-release-verify.py` | 23 tests |
| `git diff --check` | clean |

Python covers complete/no-op/partial plans, canonical siblings, symlink refusal,
tamper/digest/downgrade/same-version refusal, base SHA validation, unchanged tap
contents, no partial stdout on failure and continued publication refusal. Ruby
checks ownership and absence of lifecycle/migration hooks. Its handoff/recovery
fixture uses inert temporary files/symlinks: **not Homebrew migration acceptance**.
The macOS build/verify suites use stubs, not a new signed or notarized build.

## Real published archive and read-only tap preparation

Unique evidence directory: `/tmp/cercano-homebrew-promotion-ready.qTjbxK`.

* Fresh read-only `gh api` branch/tree/Contents requests pinned the official tap
  to `caffd060597d6ca9706c9fa4db0c7c042722d7ef`. Tree completeness and the Git
  blob hash of `Formula/cercano.rb` were verified; the two new components are
  absent at that base. Snapshot: `tap/Formula/cercano.rb`; supporting responses:
  `tap-tree.json`, `official-formula-api.json`, `tap-base-sha.txt`.
* Downloaded the public v0.27.0 macOS arm64 archive over HTTPS without forwarding
  API credentials. Its SHA-256 equals the trusted value
  `18c68ccc69662da6d18b8017a3eba7eaebc67e057065b90bb66deba2838d2aa0`, the
  published asset API digest, and the retained archive from run `43655050601`
  attempt 2 in `/tmp/cercano-v0.27.0-verify.WdO7y2`. This verifies digest equality,
  not a new independent provenance/signing acceptance run.
* Rendered `formulas/{cercano,cercano-agent,cercano-cli}.rb` from that archive;
  strict layout/inventory validation succeeded. `promotion-plan.json` records
  all three paths, their exact contents/hashes and reviewed tap base. All three
  decisions are `update` (two are additions). Status is
  `review-only-publication-blocked`, never permission to apply it.
* `evidence-summary.json`, `published-archive.sha256`, `published-release.json`,
  `test-results.json` and the five named test logs retain the evidence. No auth
  headers/tokens or signed redirect URLs were recorded; API access used existing
  gh authentication only for read-only requests, never tap write authority.

No release dispatch/action, push, tag, tap mutation, real Homebrew install/unlink/
cleanup, live agent/keychain access, signing or destructive migration was used.
Successful preparation does not authorize any of those actions.

## Remaining acceptance blockers

Keep the worktree for review; do not land or publish yet. Still required:

* Installed-user applicability of the established historical monoliths and actual
  native migration, ordinary link-conflict behavior and failure/snapshot recovery.
* Three-formula native install/upgrade/uninstall, partial sets and repository lag.
* Human acceptance, changed-source A/B compatibility, real inference after idle,
  later worker launch and restart with **all external runtime paths unchanged**.
  Same-source synthetic native evidence is not acceptance of these scenarios.
* Final signed/notarized clean-Mac smoke and supported platform qualification.
* Separately authorized coherent publisher implementation/review: one commit,
  reviewed-base compare-and-swap, dependency availability and preserved downgrade,
  drift, public-asset provenance/checksum and transport guards.

No live transaction is enabled in this slice; legacy sequential Contents API helpers
are not a fallback. Tap base and all inputs must be revalidated before any future
application. This is bounded preparation, not release readiness.


## Independent-review corrections and current verification

The preparer previously accepted CRLF desired formulas after universal-newline
normalization and hashed LF-normalized old text rather than snapshot bytes.
New regressions were run before the fix: all three CRLF desired cases failed to
refuse, and the CRLF before hash differed from SHA-256 of the actual bytes.
Malformed version/path/archive cases also reproduced raw sentinel-bearing
validation/path diagnostics and raw tar errors (8 failing subcases total).
The fix uses byte reads, strict UTF-8 decoding without newline normalization,
canonical encoded-byte comparisons and actual-byte hashes. CLI refusals now use
fixed bounded diagnostics, retaining validation exit 1, usage exit 2 and success
exit 0. Tests also cover invalid UTF-8 and no partial stdout/tap mutation.

The isolated coordinated publisher core remains disabled, with no CLI or workflow
integration. Current self-contained fake-API regressions cover expectedHeadOid,
stale advance/rewind and boundary races, counterfeit legacy refusal, and sanitized
transport diagnostics. The new integration regression calls the real preparer
with an inert archive and a monolith-shaped snapshot, then proves the resulting
review plan is refused by the test-enabled publisher without mutation. This is
not a historical production formula or native migration execution. Eligibility
remains absent or exact canonical split sets only; monolith migration and live
asset/provenance/native acceptance are not supplied by this core.

Current post-fix checks (2026-11-14), all passed with no skips:

| Command | Result |
| --- | --- |
| `python3 -m unittest discover -s release/homebrew -p 'test_*.py'` | 70 tests |
| `ruby release/homebrew/test_formula.rb` | 5 tests, 40 assertions |
| `python3 scripts/test-release-workflow.py` | 51 tests |
| `python3 scripts/test-macos-release-build.py` | 12 tests |
| `python3 scripts/test-macos-release-verify.py` | 23 tests |
| `git diff --check` | clean |

### Historical candidate evidence attribution

Removed `OldCandidateRegressions`, which depended on a developer's external
`homebrew-coordinated` worktree and otherwise skipped. Its embedded harness
imported that checkout's `formula_set` and `update_tap`, test-enabled its guard,
and injected snapshot/REST functions: it asserted a non-force ancestor rewind
was accepted, a commented legacy-install substring was classified as legacy,
and an injected OSError sentinel escaped. These are historical candidate harness
claims preserved from the removed test, **not rerun or independently verified in
this review pass**, not real GitHub production execution, and not a model of old
code presented as production evidence. No old candidate was vendored. Current
correctness is tested solely against this worktree, without that checkout or a
skip. Earlier public archive/tap evidence above is likewise historical; this pass
performs no download, publication, install or VM activity.
