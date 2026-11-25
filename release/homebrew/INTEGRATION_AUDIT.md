# Local Homebrew integration audit — 2026-11-25

This is an audit of integrated completed work, not a plan or release acceptance.
Baseline local main: `cbd16185c270e6c46f9d5f4669c8aa2a38521fc9` (including its
four local commits after `07ab51ee`). Root untracked work remains outside this
integration; the observed count before landing is 673 files.

## Integrated scope and safety

- Promotion-ready commits `f4229fd2` through `98a4f3f5`, in original order:
  offline preparation, disabled atomic standalone publisher, pinned provenance,
  and bounded native/migration/recovery evidence.
- Generalized publication `bf5bcca4`, then `a2dd1584`: three-formula validation,
  coordinated Git tree mutation, private-index local review commit, and early
  publication preflight before all platform builds/signing/public mutations.
- The four release operator docs from `381b48b9`, reconciled to that pipeline.
- Only historical documentary/evidence material from `9f6a643b` and `d66ff176`;
  no executable gate enablement. Overlapping `9a06577f` was not applied.
- Both `formula_set.PUBLICATION_ACCEPTED` and
  `coordinated_publish.PUBLICATION_ACCEPTED` remain **False**. No override,
  push, tag, release, VM or paid inference is authorized or executed here.
- Current-main formula templates and Chocolatey publishing workflow are retained
  unchanged. Offline preparation's per-file comparison is now local to that
  module, since the generalized updater replaced the old single-file decision.

## Evidence distinctions and remaining blockers

Historical provenance is pinned to source
`c91699b079164138bff083206594f49c56dd5509`, workflow
`4b3cd19b4e505239c7a875952a7bb40fd46754e0`, run `43655050601`, attempt 2.
The signed v0.27.0 archive and native loopback checks are real bounded packaging
and synthetic-provider evidence, not real-model inference. Same-source/version
fixtures do not establish current-source real A/B or live session continuity.
Partial trust checks do not establish full clean-Mac distribution qualification.
All human checks in
[the closeout checklist](../../efforts/macos-native-validation-closeout/human-acceptance.md)
remain pending. External llama/runtime paths must remain unchanged; no broader
cleanup/retention guarantee is supplied.

[Historical publication authorization and evidence](PUBLICATION_AUTHORIZATION.md)
records the prior v0.27.0 standalone operation only. It does not authorize a
current general boolean enablement or constitute current-source acceptance.
Tap-only recovery is implemented with original same-run three-formula artifacts
and immutable source/provenance protections, but its updater still fails the
shared acceptance gate before network/publication. Local promotion also remains
gated. Publication-enabled main runs now fail at preflight, not after uploading
public assets. Build-only runs are not acceptance or publication authorization.

## Verification

Local Python discovery: 114 tests, 113 passed and one skipped because the optional
historical v0.27.0 archive is not in this isolated checkout. The synthetic real
preparer-to-fake-publisher integration still runs. Ruby formula DSL: 5 tests,
40 assertions, no skips. Release workflow contract: 52 tests. Chocolatey workflow
contract: 6 tests. No full Go suite is needed for this Python/workflow/docs scope.
These checks cover closed gates, same-version immutability, same-run artifact
contracts, coordinated contention/reconciliation and controlled local Git
integration; they do not replace pending native/human release acceptance.

## Retained workspaces

All preexisting worktrees/evidence are retained, including sibling
`Cercano-homebrew-promotion-ready`, `Cercano-release-docs`, and root
`.worktrees/homebrew-coordinated`, `.homebrew-promotion-fix`,
`.homebrew-clean-install-diagnostic`. Their state and historical evidence are
outside this integration's cleanup authority. Root evidence, including
`.chocolatey-resubmission-20261119/`, `.ci-investigation/`,
`.combined-production-verification/` and `.review-pr84-evidence/`, is untouched.
Only the newly created `Cercano-homebrew-safe-integration` worktree is a cleanup
candidate after successful landing and containment/cleanliness checks.
