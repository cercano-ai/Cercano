# Release operator runbook

This is the authoritative operator procedure for GitHub Releases, Homebrew and
Chocolatey. Platform details: [macOS signing](macos-release-build.md),
[Homebrew definitions](../release/homebrew/README.md),
[Chocolatey publishing](chocolatey-release.md),
[experimental Windows](windows-artifact.md) and [experimental Linux](linux-artifact.md).
Commands below are templates for an authorized operator, **not authorization to
release**. Updating these docs does not authorize a tag, dispatch or publication.

## STOP: the full publication pipeline is currently blocked

**Read this before creating/pushing a version tag or dispatching a workflow.**
The shared acceptance gate in [formula_set.py](../release/homebrew/formula_set.py)
remains **False**, with no CLI/environment override. The integrated workflow now
requires `publication-preflight` before all platform builds: `publish=true`
fails there, before signing or public mutation. `publish=false` permits an
explicitly authorized build/signing rehearsal, not publication acceptance.

The coordinated three-formula updater and private-index local promoter are
implemented but gated before file/credential/network access. The tap-only recovery
workflow requires the original run's complete three-formula artifact, immutable
source/provenance checks and public archive verification; its updater is subject
to the same closed acceptance gate. It is not currently an enabled recovery route.
The isolated historical `coordinated_publish.py` has a separate **False** gate;
its v0.27.0 authorization is historical, not permission for general enablement.

Native/runtime acceptance, real changed-source A/B, live continuity, human checks
and clean-Mac distribution qualification remain incomplete. See the
[integration audit](../release/homebrew/INTEGRATION_AUDIT.md) for bounded evidence.
Do not dispatch publication to probe these gates or manually bypass them. Hold
tag/publication steps until separately authorized acceptance is reviewed and
shipped on remote `main`. No publication is authorized by this local integration.

## 1. Prerequisites and human decisions

- Obtain authorization for the version, exact source SHA, intended channels and
  workflow ref. Record the operator, reviewer and approval evidence. Do not infer
  release permission from a passing check or from a request to edit docs.
- Use an isolated clean release workspace. Preserve unrelated/untracked work;
  do not reset, clean or stash someone else's checkout. Authenticate Git/GitHub
  CLI for repository reads and the separately authorized tag push/dispatch.
- Configure a protected **`release` environment**, required reviewers and trusted
  deployment branch/tag restrictions. Reviewers must inspect both workflow and
  source revisions, inputs, native build results and artifacts before approval.
  Do not remove protection to unblock a run. Windows/Linux build jobs are
  secret-free; the protected macOS job signs and, if enabled, publishes.
  Chocolatey has a separate protected publisher job; do not promise one approval
  covers every deployment. Actual environment rules control approval timing.
- Signing requires an authorized Developer ID Application certificate in
  `MACOS_CERTIFICATE_P12` (base64), optional `MACOS_CERTIFICATE_PASSWORD`, and
  `APPLE_ID`, `APPLE_TEAM_ID`, `APPLE_APP_SPECIFIC_PASSWORD` for notarization.
  See the [macOS configuration details](macos-release-build.md#required-configuration).
- GitHub publication uses the workflow's scoped `contents: write` token.
  Future tap publication requires `HOMEBREW_TAP_TOKEN` with narrowly scoped
  Contents read/write access to `cercano-ai/homebrew-cercano`, subject to its
  branch protections. Having a token does **not** enable the disabled updater.
  Chocolatey submission requires `CHOCOLATEY_API_KEY` for an authorized account;
  prefer protected environment storage. Review actual repository/environment
  configuration privately; secret names in docs are prerequisites, not a live
  inventory or proof that credentials exist, work or are unexpired.
- Never paste values into chat, docs, inputs, command arguments or logs. Enter
  credentials through approved secret management. The optional
  [publishing-access workflow](../.github/workflows/verify-publishing-access.yml)
  reads reported repository permissions for tap **and plugin** tokens; it does
  not test Apple/Chocolatey credentials, prove a write will pass branch rules, or
  prove pipeline readiness. It is not a required release acceptance gate.

## 2. Select and accept one exact remote-main commit

Run these read-only/ref-fetch checks from the isolated workspace. Replace the
version example with the authorized stable `X.Y.Z` (no `v`, prerelease or leading
zeroes). Do not use local `HEAD` or assume local `main` is already published.

```bash
set -euo pipefail
REPO=cercano-ai/Cercano
VERSION=1.2.3
TAG="v$VERSION"
[[ "$VERSION" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]
git remote get-url origin  # confirm this is the intended repository
git fetch origin refs/heads/main:refs/remotes/origin/main
SOURCE_SHA="$(git rev-parse --verify 'refs/remotes/origin/main^{commit}')"
REMOTE_MAIN="$(git ls-remote --exit-code origin refs/heads/main | cut -f1)"
[[ "$SOURCE_SHA" == "$REMOTE_MAIN" ]]  # if it moved, stop and review again
git rev-list --left-right --count main...refs/remotes/origin/main
git log --oneline refs/remotes/origin/main..main
printf 'Candidate source: %s\n' "$SOURCE_SHA"
gh api "repos/$REPO/commits/$SOURCE_SHA/check-runs" --paginate
gh api "repos/$REPO/commits/$SOURCE_SHA/status"
gh run list --repo "$REPO" --commit "$SOURCE_SHA" --limit 100
```

Record the full SHA, not just a moving branch name. Inspect existing runs and
check results for **that SHA**, their event, workflow revision, attempts, logs
and conclusions (`gh run view <run-id> --repo "$REPO"`). Pagination/listing
limits must not hide relevant evidence. A PR check for a different merge SHA or
an older commit does not accept this candidate. A successful API request with no
checks is not a pass.

The checked-in [CI workflow](../.github/workflows/ci.yml) runs on pushes/PRs to
`main`, with `test` and `cli-e2e` jobs (tests, launcher fixtures, builds,
cross-compilation and terminal E2E). Inspect the other applicable workflows and
repository branch/ruleset-required checks too; this guide does not invent a
fixed list of required check names. Require applicable required checks and the
release owner's acceptance of existing CI on the exact candidate. Pending,
failed, cancelled or missing required evidence blocks release; resolve it or
choose and review a new candidate. Do not rerun the full Go suite merely for
documentation changes, and do not weaken checks to manufacture acceptance.

Review release workflow/scripts at the selected source and workflow revisions,
not only this guide. If remote main moves before tagging, stop to reconfirm the
exact accepted SHA and authorization rather than silently tagging the new tip.

## 3. Check version availability (absence is normal)

Before mutation, enumerate successfully with adequate access:

```bash
# Empty output is normal for a new version; a transport/auth error is not.
if git show-ref --verify --quiet "refs/tags/$TAG"; then
  echo 'Local version tag already exists; stop.' >&2
  exit 1
else
  test "$?" -eq 1  # other failures are not evidence of absence
fi
EXISTING_TAGS="$(git ls-remote origin "refs/tags/$TAG" "refs/tags/$TAG^{}")"
[[ -z "$EXISTING_TAGS" ]]  # existing remote tag blocks a new release
gh api --paginate "repos/$REPO/releases?per_page=100" \
  --jq '.[] | {tag_name, draft, prerelease, assets: [.assets[].name]}'
```

Check local and remote tags, published **and draft** releases/assets, the tap's
three formula versions/digests, and all three Chocolatey package IDs at the
intended package version (including account/unlisted/pending moderation state).
For a new version, **no tag/release/package is the expected starting state**, not
a defect to fix by choosing an old tag. A 404 or empty public Chocolatey page
alone is not authoritative absence; verify access and account state. Inventory
unknown/partial state before proceeding. Do not reuse any published/submitted
version or overwrite immutable assets. Existing version tags belong to recovery,
not this new-release sequence.

## 4. Create an immutable annotated tag — only after the STOP gate is cleared

These commands mutate local/remote tags and require explicit release approval.
Recheck absence immediately beforehand and stop on any race or mismatch. Use the
previously recorded, accepted full `SOURCE_SHA`, **never implicit HEAD**:

```bash
git tag -a "$TAG" "$SOURCE_SHA" -m "Release $TAG"
[[ "$(git cat-file -t "refs/tags/$TAG")" == tag ]]
[[ "$(git rev-parse "refs/tags/$TAG^{commit}")" == "$SOURCE_SHA" ]]
TAG_OBJECT="$(git rev-parse "refs/tags/$TAG")"
git show --no-patch "$TAG"
git push origin "refs/tags/$TAG:refs/tags/$TAG"
REMOTE_TAGS="$(git ls-remote origin "refs/tags/$TAG" "refs/tags/$TAG^{}")"
printf '%s\n' "$REMOTE_TAGS"
[[ "$(printf '%s\n' "$REMOTE_TAGS" | awk -v r="refs/tags/$TAG" '$2 == r {print $1}')" == "$TAG_OBJECT" ]]
[[ "$(printf '%s\n' "$REMOTE_TAGS" | awk -v r="refs/tags/$TAG^{}" '$2 == r {print $1}')" == "$SOURCE_SHA" ]]
```

Require both the remote annotated-tag object and peeled commit to match. Never
force-push, move, delete/recreate or convert a version tag; stop and investigate
any mismatch. Tag push does **not** start the manual release workflow.

## 5. Workflow ref, source and tooling provenance

`release-macos.yml` is `workflow_dispatch` only, with `version` and `publish`
(default **false**). Choose a trusted reviewed **branch or tag** for the workflow
ref; record its full resolved revision and verify the resulting run's workflow
revision. A branch may move between review and dispatch: stop if the actual run
differs. Retain the run ID, attempt, inputs, source SHA and tooling SHA.

- Every platform validates `version`, checks out explicit `refs/tags/v<version>`,
  and verifies HEAD equals the tag commit. Build scripts, application tests,
  workflow fixtures read by those tests, Linux packaging and Homebrew
  rendering/publication tooling come from **the source tag**.
- The selected ref supplies the **executing workflow**. Chocolatey packaging
  tools are a separate checkout pinned to immutable `github.workflow_sha`;
  Chocolatey publishing tools use that same revision, not the application tag.
- Source logs print `Release source` and `GITHUB_WORKFLOW_REF`/`GITHUB_SHA`.
  Record the resolved workflow revision as well as the distinct tagged source;
  do not treat the Actions run's `head_sha` as proof of built application source
  when dispatching from a different ref.
- Rerunning an old run uses its old workflow, not fixes on `main`. A newly
  dispatched reviewed workflow ref can build an unchanged old tag (for example
  to select the supported `macos-15` arm64 runner), but tag-bound tooling still
  comes from that old tag. It does not ship a newer updater. Never set
  `CERCANO_ALLOW_UNTAGGED` in a release or move a tag to repair tooling.

After separate authorization, a **build-only** template is:

```bash
WORKFLOW_REF='<reviewed-branch-or-tag>'
gh workflow run release-macos.yml --repo "$REPO" --ref "$WORKFLOW_REF" \
  -f version="$VERSION" -f publish=false
```

This still signs/notarizes and requires protected approval. It does not write a
GitHub release, tap or Chocolatey submission. Artifacts are retained 14 days.
A subsequent dispatch builds again; there is **no promote-prior-dry-run input**.
Do not rebuild an already published version as a repair strategy.

**Publication remains blocked:** only after shipped remediation and explicit
channel approval would the same dispatch use `publish=true`. Do not run that
mode with the current updater. Reviewers should reject publication approval
while these blockers remain, even if unsigned platform jobs have passed.

## 6. Pipeline and channel verification

Current ordering is Windows/Linux secret-free builds and smoke checks → protected
macOS test/sign/notarize/verify/render → retained artifacts → optional GitHub
publication → tap tests/updater → separate protected Chocolatey publisher.
Publication downloads **this run's** three platform artifacts, checks recorded
archive digests and refuses existing target assets; `overwrite_files: false` is
also set. It uploads three archives and three checksum sidecars, not review
packages. It does not rebuild between verification and upload. macOS is signed
and notarized; Windows/Linux are explicitly unsigned and experimental. Linux
native package artifacts are review-only, not an APT/RPM publication pipeline.

Record and verify each channel separately; a job conclusion is not the ledger:

1. **GitHub:** confirm tag object/peeled SHA again, release is public/non-draft,
   expected six asset names, and downloaded archive/sidecar digests match the
   build outputs from the authorized run. Check verification/notarization
   evidence for the exact macOS bytes, not just signing success. See the
   [macOS verifier](macos-release-build.md#verifying-a-finished-archive).
2. **Homebrew:** review all **three** `Formula/cercano-agent.rb`,
   `Formula/cercano-cli.rb`, `Formula/cercano.rb` at the tap commit, coordinated
   versions, component archive URLs/digests and convenience dependencies.
   `render_formula.py --output .../cercano.rb` creates all three siblings;
   rendering/upload is **not promotion**. The agent owns `cercano`, CLI owns
   `cercano-cli`, convenience formula owns neither. Require separately accepted
   native clean install, component/partial upgrade, old-monolith migration,
   runtime closure and user-state preservation evidence. Bare dependencies do
   not ensure atomic/exact-version upgrades. Current updater cannot do this;
   an existing tap version does not prove a new release reached it.
3. **Chocolatey:** verify three inspected `.nupkg` files plus sidecars and the
   packer's digest manifest from this run. Submission is dependency order
   `cercano-agent` → `cercano-cli` → `cercano`. Only HTTP 201 confirms each
   submission. Then track **each** package's moderation and public/default-source
   availability independently. CI success/201 is **submitted**, not approved or
   installable from the default feed. Use the [Chocolatey guide](chocolatey-release.md)
   for native checks and package-only revisions; do not announce moderation from
   a successful upload.

## 7. Partial failure and recovery

Freeze mutations and preserve run/attempt IDs, workflow/source/tooling revisions,
step conclusions, archive/package digests, tag object, public assets, tap commit
and Chocolatey account/moderation evidence. Keep evidence private where needed;
never retain credential values. Download retained artifacts before their 14-day
expiry; expiry does not authorize rebuilding immutable published bytes.

| Failure boundary | Safe next action / limitation |
| --- | --- |
| Before publication | Determine whether any external mutation occurred; inspect logs and remote state before an explicitly authorized retry. Check notarization history/info/log after a timeout rather than resubmitting blindly. |
| GitHub upload fails partway | Inventory every asset and digest. Do not delete/recreate the release, use clobber/overwrite, or rerun the consolidated build. No general resume/asset-completion input exists; obtain a reviewed recovery plan preserving original verified bytes. |
| GitHub succeeds, tap fails | If this occurs after future acceptance, GitHub assets remain immutable and Chocolatey is skipped. Use the gated tap-only recovery only after explicit authorization, with the original complete artifact and provenance. A token cannot bypass acceptance. Current preflight prevents reaching this stage with publication enabled. |
| Chocolatey partly submits / times out | Stop; reconcile each ID/version and accepted bytes through account/moderator evidence. No resume/skip-existing input or automatic mutation retry exists. Never blindly rerun all three. A conflict/public-page absence does not establish submission identity or absence. |
| Application defect after publication | Use a new application version/tag after normal acceptance, not replacement artifacts or tag movement. |
| Package metadata defect only | Standalone Chocolatey packaging can use a deliberate new `X.Y.Z.N` package revision around the original trusted ZIP, after resolving existing feed state. It is a new reviewed package set, not an application rebuild or generic partial-submit resume. |

The tap recovery provenance checker currently also requires run `head_sha` to
match the tag commit, so a run dispatched from a different workflow commit can
fail that check even with correct tagged source. Disabled recovery is not a
promise of cross-ref recovery. Do not change provenance or approval gates during
an incident simply to make them pass. Announce channel-specific partial status
honestly; completion requires accepted evidence for all intended channels.
