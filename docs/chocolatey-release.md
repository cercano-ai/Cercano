# Gated Chocolatey CI publishing

See the authoritative [release runbook](releases.md) before selecting a source,
tagging or dispatching. **Integrated publication is currently blocked:** its
`publish=true` mode is stopped by the required acceptance preflight before
builds or public mutations. Tap-only recovery shares the closed acceptance gate.
that mode as a Chocolatey route or as a test of readiness.

Operator guide for the manual [standalone packaging workflow](../.github/workflows/package-chocolatey.yml)
and [integrated release workflow](../.github/workflows/release-macos.yml).
Windows remains experimental and unsigned. See [installation and package status](installation-windows.md)
and the [packaging implementation/readme](../release/chocolatey/README.md).

**Already submitted:** package version **0.27.1**, containing application **0.27.0**,
was submitted manually. Do not dispatch publication or rerun submission for that
version. Submission is not moderation approval or proof of default-source
availability. This guide does not establish current moderation status.

## Owner setup and approval boundary

Before opting into publication:

1. Configure **`CHOCOLATEY_API_KEY`** for an authorized Chocolatey community-feed
   account through approved GitHub secret management, preferably scoped to the
   protected `release` environment. Never put the key into inputs, command arguments, logs or files.
2. In **Settings → Environments**, configure **`release`** with required reviewers
   and deployment branch/tag restrictions for trusted workflow refs. Do not rely
   on automatic environment creation as an approval gate. Protect those refs:
   a repository secret is not inherently environment-restricted, and malicious
   workflow edits could expose it outside this design.
3. Review the selected workflow commit, requested version, native test results
   and inspected artifacts before approving the `publish-chocolatey` job.
   The integrated workflow also needs the signing/notarization and Homebrew
   configuration described in [the macOS release guide](macos-release-build.md),
   plus resolution of the current publication blockers.

Both workflows are `workflow_dispatch` only and default **`publish=false`**.
Packing jobs have no Chocolatey credentials and no `release` environment. Only
the separate protected publisher step receives `CHOCOLATEY_API_KEY`; its job
uses environment `release`, read-only repository permissions and a 15-minute
timeout. Environment protection rules govern approval of that job; do not assume
an earlier signing approval bypasses this separate deployment gate.

## Choose the workflow and inputs

| Workflow/input | Actual meaning and default |
| --- | --- |
| `package-chocolatey.yml`: `version` | Required existing stable application `X.Y.Z`, without `v`, leading zeroes or prerelease suffix. Selects the published Windows ZIP. |
| `sha256` | Required independently trusted 64-hex SHA-256 of that ZIP. No default or automatic expected-checksum acquisition. |
| `package_version` | Optional; empty defaults to `version`. Only `version` or `version.N` with a positive integer fourth component is accepted. |
| `publish` | Boolean, default **false**. Explicit **true** enables protected submission after successful packaging. |
| `release-macos.yml`: `version` | Required stable application `X.Y.Z`, built from existing tag `vX.Y.Z`. Package version equals application version; no `sha256` or `package_version` input. |
| `publish` | Boolean, default **false**. **true** opts into GitHub Release, Homebrew tap and Chocolatey publication, not Chocolatey alone. |

Use the standalone workflow for an already published ZIP or a packaging-only
correction. Application version controls release URLs, embedded provenance and
binary `--version` checks. For example, hypothetical package `1.2.3.1` contains
application `1.2.3`; all three nuspec versions and convenience-package dependency
minimums become `1.2.3.1`. These dependencies are minimums, not exact locks.
Do not rebuild, retag or mislabel application code to fix package metadata.
Never overwrite a submitted package version; after resolving any unknown feed
state, use a new positive fourth-component revision for a packaging correction.

The selected workflow ref supplies packaging/publishing tools, pinned to immutable
`github.workflow_sha`; standalone packing verifies and logs that checkout SHA.
The integrated workflow keeps application builds/tests tag-bound and checks out
packaging tools separately. Select a trusted reviewed ref containing these tools,
not an older application tag that lacks the gates.

## Dispatch commands (templates, not instructions to republish old versions)

Run from an authenticated repository checkout. Replace every placeholder with a
reviewed value for a **new intended package version**. The following commands
perform real dispatches; do not run them merely to test documentation.

```sh
# Standalone: build/inspect only; existing published application ZIP required.
VERSION='<application-X.Y.Z>'
PACKAGE_VERSION='<X.Y.Z-or-X.Y.Z.N>'
TRUSTED_SHA256='<independently-trusted-64-hex-ZIP-digest>'
WORKFLOW_REF='<trusted-workflow-branch-or-tag>'
gh workflow run package-chocolatey.yml --ref "$WORKFLOW_REF" \
  -f version="$VERSION" -f sha256="$TRUSTED_SHA256" \
  -f package_version="$PACKAGE_VERSION" -f publish=false

# Explicit submission: a NEW run, not promotion of the prior review artifact.
# First check feed/account state and confirm this package version is unsubmitted.
gh workflow run package-chocolatey.yml --ref "$WORKFLOW_REF" \
  -f version="$VERSION" -f sha256="$TRUSTED_SHA256" \
  -f package_version="$PACKAGE_VERSION" -f publish=true

# Integrated release: tag v$VERSION must already exist; all platforms build.
gh workflow run release-macos.yml --ref "$WORKFLOW_REF" \
  -f version="$VERSION" -f publish=false
# No integrated publish=true command: current tap refusal makes full publication
# unsafe. See releases.md for the blocked publication contract.
```

Choose one route, not both publishers for the same package version. In the Actions
UI, the equivalent is **Run workflow**, select the trusted ref, fill the inputs,
and leave Publish unchecked for build-only. Omit standalone `package_version`
when it should equal `version`. Obtain the expected ZIP digest from an independent
trusted release record; hashing only the newly downloaded ZIP does not establish
trust. The standalone downloader uses bounded HTTPS downloads and rejects a ZIP
that differs from the supplied digest; it never rebuilds the application.

`publish=false` skips submission entirely. The integrated workflow still signs
and notarizes macOS and therefore still requires its signing-environment approval;
it writes neither a GitHub Release nor a tap update. A later dispatch builds and
packs again; it does not promote a previous run's artifacts. With `publish=true`,
integrated Chocolatey submission waits for the main build/publication job and the
Windows job to succeed (the main job itself waits for Windows and Linux). The current closed publication preflight stops the pipeline before builds or
public mutations; later tap failure after future acceptance would still skip
integrated Chocolatey.

## Checksums, artifacts and review

Native Windows checks require `choco` and Windows PowerShell, run isolated tests,
pack all three packages, inspect actual metadata/members/payload bytes, then run
extracted binaries with only `--version` and throwaway user directories. They do
not install packages, start agents, download models or sign Windows executables.
The integrated packer consumes that run's exact smoke-tested ZIP and recorded
SHA-256; standalone packing consumes the independently verified published ZIP.

| Workflow | Actions artifact (14-day retention) |
| --- | --- |
| Standalone | `cercano-<application-version>-chocolatey-review` |
| Integrated | `cercano-<application-version>-chocolatey` |

Each contains exactly three `.nupkg` files and three `.nupkg.sha256` sidecars.
Artifact names use the **application** version even for a package revision;
package filenames use the effective **package** version. These are review
artifacts, not GitHub Release package assets. Review run logs/tooling SHA,
inspection and isolated-version results, and download the current run's artifact
from Actions. Locally, in its extracted directory, `shasum -a 256 -c *.sha256`
checks the sidecars; it does not independently prove artifact provenance.

Before any network submission, the publisher downloads only its named artifact
from **the current run**, requires the exact expected file set, and verifies
**all three** package digests against both the packer's job-output manifest and
sidecars. It retains those exact verified bytes for submission. Review artifacts
are produced even when `publish=false`; no credentials reach the packer.
Portable/fixture tests alone do not demonstrate native CI success or live-feed
compatibility; inspect the actual Windows run before enabling publication.

## Submission order, concurrency and recovery

The publisher submits **`cercano-agent` → `cercano-cli` → `cercano`** (convenience
package last). Both workflows share concurrency key
`chocolatey-publish-<effective-package-version>` with `cancel-in-progress: false`.
The integrated release also has workflow lock `release-macos-<version>` and its
main build uses `cercano-homebrew-tap`. The shared Chocolatey lock serializes
publishers for that version, **not** all versions or an immutable submission
ledger. GitHub can replace older pending jobs; a lock does not make reruns safe.

The publisher uses multipart HTTPS PUT to
`https://push.chocolatey.org/api/v2/package`, with the secret only in the
`X-NuGet-ApiKey` header. It does not persist a key, invoke a child process with
credentials, follow redirects, print raw server responses or retry mutations.
Only **HTTP 201 Created** confirms an individual submission. Other responses
(including 409, redirects or other 2xx), timeouts and transport errors stop the
sequence immediately without submitting later packages or retrying.

On failure or interruption:

1. Preserve the run ID, workflow SHA, artifacts/digests and successful
   `Submitted <filename>` log entries. The final error is intentionally generic;
   a failing run can have already submitted earlier packages.
2. Inspect each package ID/version in the Chocolatey account/feed and moderator
   state. A timeout may mean the server accepted a package before the connection
   failed; a 409 is not proof of a matching successful submission. Public-page
   errors or delayed visibility are not proof of absence.
3. Do **not** blindly rerun the job/workflow, retry all three, delete/re-upload,
   or automatically increment a version to bypass uncertainty. There is no
   resume/skip-existing input and earlier submissions cannot be rolled back by
   this publisher. Resolve the accepted bytes/versions with account or moderator
   evidence before deciding a deliberate recovery plan.
4. For validation failures before any network request, fix the cause and review
   anew, still confirming that no other run/manual operator submitted that version.

A green publisher means **submitted**, not **approved**. Monitor moderation for
each component and the convenience package separately; dependency review and
public/default-source availability may lag. Do not announce approval from a 201
response, a passing CI job or the historical manual 0.27.1 submission.
