# macOS release build

`scripts/build-macos-release.sh` produces the signed Apple Silicon release
archive that the Homebrew formula refers to. It builds both binaries from one
tag-derived version, verifies them, signs them through
`scripts/sign-macos-release.sh`, and packages exactly the layout
`release/homebrew/render_formula.py` accepts:

```
cercano-<version>-darwin-arm64/
  bin/cercano
  bin/cercano-cli
  LICENSE
  README.txt
```

## Usage

```bash
export CERCANO_CODESIGN_ID='Developer ID Application: Your Name (YOURTEAMID)'
bash scripts/build-macos-release.sh 1.2.3 dist
```

This writes `dist/cercano-1.2.3-darwin-arm64.tar.gz` and a `.sha256` file
beside it.

## What it refuses

The script fails closed, and a refused build leaves no archive behind:

- **Unsigned builds.** `CERCANO_CODESIGN_ID` must name a Developer ID
  Application identity; `none` and `-` are rejected. There is no unsigned
  escape hatch — use `scripts/build-macos-unsigned.sh` for rehearsals.
- **Unstable or untagged versions.** Only `X.Y.Z` without a leading `v` or
  leading zeros, and a matching `v<version>` tag must exist. Set
  `CERCANO_ALLOW_UNTAGGED=1` for a local dry run only.
- **Wrong architecture or a raised deployment floor.** Both binaries must be
  arm64-only with a macOS 12.0 build target. This is checked before either
  binary is signed.
- **Version mismatch.** Each binary must report the version being built, so an
  archive cannot claim a version its contents do not.
- **Signing or post-staging verification failure.** Signatures are re-verified
  on the exact bytes that get archived.
- **Existing artifacts.** It will not overwrite an archive or checksum that is
  already present, so a published artifact cannot be silently replaced.

## What it does not do

The script **does not notarize**. A built archive is signed but not yet
distributable: run `scripts/notarize-macos-local.py` (or the release workflow)
against the staged `bin` directory and verify the finished archive before
promoting anything to the tap. A separate clean-Mac rehearsal is optional.
It also does not push, publish, tag, or
touch the tap, and it bundles no models, runtimes, or credentials.

## Verifying a finished archive

`scripts/verify-macos-release.py` inspects an archive the way a consumer
receives it, rather than trusting the machine that produced it:

```bash
python3 scripts/verify-macos-release.py dist/cercano-1.2.3-darwin-arm64.tar.gz \
    --version 1.2.3 \
    --sha256 <expected-64-hex-digest>
```

It checks the archive layout, the SHA-256 digest, arm64-only architecture, the
macOS 12.0 deployment target, a Developer ID Application signature with
hardened runtime and a secure timestamp, executable permissions, the required
`LICENSE` and `README.txt`, and that each binary reports the expected version.
Any failure exits non-zero and says not to publish.

**How notarization is checked.** Notarization evidence from
`scripts/notarize-macos-local.py` is **required**, passed with
`--notarization-evidence <dir>`. Each binary's code directory hash (cdhash) is
compared against the cdhashes in Apple's Accepted notarization ticket. A match
proves those exact bytes were notarized. Offline, pass `--skip-gatekeeper`; the
run then reports notarization as `UNVERIFIED` rather than implying success.

Two other approaches were tried against a real notarized build and rejected:

- **`spctl --assess`** only evaluates app bundles. It rejects a correctly signed
  and notarized command-line binary with *"does not seem to be an app"*.
- **`codesign --test-requirement==notarized`** is unreliable for loose Mach-O
  files. Two freshly notarized probe binaries both failed it even though Apple
  returned Accepted and their cdhashes appeared in the ticket, and the failure
  persisted after waiting. A bare binary carries no stapled ticket — `xcrun
  stapler` handles only bundles, disk images and packages — so that lookup
  cannot be depended on.

The cdhash comparison is deterministic and needs no network call at
verification time, since Apple's ticket contents are captured at submission.

## Release workflow (GitHub Actions)

The signing job pins the standard `macos-15` Apple Silicon runner. GitHub's
[hosted runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners#standard-github-hosted-runners-for-public-repositories)
lists this label as arm64 (M1); `macos-15-intel` is not interchangeable.
The host OS is independent of the archive's macOS **12.0** deployment floor:
`build-macos-release.sh` still pins that floor in the environment and CGO flags
and builds/verifies `darwin-arm64` binaries.

**Existing-tag runner recovery:** rerunning an old run or dispatching its old
tag uses the old workflow, not a runner fix on another branch. In particular,
`v0.27.0` contains the unsupported `macos-14` runner. Dispatch this corrected
workflow from a reviewed recovery branch with `version=0.27.0` and
`publish=false` to build the unchanged tag without moving it. Every platform
validates the version before deriving an explicit `refs/tags/v<version>`
checkout, then verifies that HEAD is that tag's commit. Logs identify both the
source commit and the workflow ref/commit. All build scripts, tests, workflow
fixtures read by those tests, and publication/tap tooling come from the tag;
the executing workflow and separately pinned Chocolatey packaging/publishing
tools come from the selected workflow revision. Thus the old source's
runner assertions still read its own matching workflow fixture. Never move the
tag, bypass validation, or set `CERCANO_ALLOW_UNTAGGED` for recovery. Protected
environment approval is still required for signing, even with `publish=false`.

`.github/workflows/release-macos.yml` is a **single-run release**. Dispatch it
from a trusted, reviewed workflow ref, with the stable version without `v`.
The release source is always the exact commit tagged `v<version>`.
Explicitly enable **publish** for a release (default **false**). This also opts
into protected Chocolatey submission; see the [Chocolatey operator guide](chocolatey-release.md).
No prior rehearsal run or second dispatch is required.

1. The secret-free Windows and Linux jobs build and smoke-test their archives.
2. Once both succeed, approve the protected `release` environment **once**.
   This approval authorizes signing **and publication**, not just a rehearsal.
3. The protected Apple Silicon job runs its test gate, builds/signs once,
   notarizes, verifies the finished archive, renders the formula, and destroys
   its temporary signing keychain. It retains the verified artifacts.
4. In that same job, publication automatically downloads this run's three
   artifacts, checks their SHA-256 digests against the recorded build outputs,
   refuses existing release assets, and publishes those exact bytes. It does
   not rebuild. The tap update follows successful publication, using the same
   archive digest and environment-protected tap token. The separate Chocolatey
   publisher then downloads this run's inspected packages;
   its own `release` environment gate is subject to configured protection rules.

**Optional dry run:** set `publish=false` (uncheck publish). All builds,
signing, notarization, verification and artifact retention still run, but no
release, tap or Chocolatey submission is written. This is not a prerequisite
for a release. A later new dispatch is a new build, not promotion of a previous dry run's artifacts.

**Manual-only:** no push, tag or pull-request trigger exists. The selected
workflow ref may differ from the tagged source; validation rejects any source
checkout that does not match the tag. Creating/pushing the tag and configuring
the protected environment are separate operator actions. Review both refs before
approving; keep required reviewers and environment deployment restrictions.
The removed automatic, unsigned `release.yml` must not be restored.

### Required configuration

Create a **protected `release` environment** in the repository settings with
required reviewers, so a run cannot reach Apple credentials without human
approval. Add these secrets to that environment:

| Secret | Contents |
|---|---|
| `MACOS_CERTIFICATE_P12` | Base64-encoded Developer ID Application certificate (`.p12`) |
| `MACOS_CERTIFICATE_PASSWORD` | **Optional** password for that `.p12`. Leave unset for passwordless export. |
| `APPLE_ID` | Apple ID for app-specific password authentication |
| `APPLE_TEAM_ID` | Apple Developer Team ID |
| `APPLE_APP_SPECIFIC_PASSWORD` | App-specific password for Apple ID authentication |

The workflow creates a temporary keychain, resolves the signing identity from
it (failing if absent or ambiguous rather than trusting a configured string),
and **destroys the keychain and all decoded key material on every path**,
including failure and cancellation. Default permissions are read-only; write
access is scoped to the single protected release job (GitHub permissions are
job-scoped, not step-scoped); unsigned platform jobs stay read-only and
secret-free. Checkout does not persist the release job token. Publication
refuses to replace an asset already attached to the tag and re-checks the artifact digest before
uploading.

**Passwordless certificates:** If your Developer ID certificate was exported
without a password, leave `MACOS_CERTIFICATE_PASSWORD` unset or empty. The
workflow will notice and proceed with passwordless import. Base64 encoding is
not encryption — GitHub secrets remain private even when unset.

### Triaging a failed notarization

When the `Notarize` step fails, the run uploads a
`cercano-<version>-notarization-diagnostics` artifact containing **only**
`diagnostic-report.json`, the sanitized summary generated by
`scripts/notarize-macos-local.py`. It holds the well-known notarytool fields
(`id`, `status`, `message`), return codes and timeout flags for the submit and
log calls, with credential-like values redacted before anything leaves the
runner. The same summary is printed to the job log. A failure is therefore
diagnosable from the artifact alone — previously the generic "Submission did
not return a successful Accepted result" plus runner-only capture files left
nothing to investigate after the runner was recycled.

The raw `submit-*`/`log-*` stdout/stderr captures, the submission ZIP and the
`submission-id.txt` file stay on the ephemeral runner; the keychain,
certificates (`.p12`, `.p8`) and authentication parameters are never uploaded.
Apple's `message` may reference the submitting Apple ID or file paths, but the
app-specific password exists only inside the temporary keychain, which the
unconditional cleanup step destroys on every path.

When reporting a failed notarization, attach the sanitized report and the job
log — never paste certificates, keychain contents, app-specific passwords or
the `xcrun notarytool store-credentials` command line. The script never
resubmits; recover the submission ID from `xcrun notarytool history` and check
`info`/`log` before any manual retry, because a timeout may mean Apple is still
processing the original submission.

## Tests

```bash
python3 scripts/test-macos-release-build.py
python3 scripts/test-macos-release-verify.py
python3 scripts/test-release-workflow.py
```

The build and verify suites drive the scripts with stub `go`, `lipo`, `otool`,
`codesign`, `spctl` and signing executables on `PATH`. They prove refusal
behavior, archive layout and permissions, agreement with the formula renderer,
and that every verification check actually fails when its property is violated.

`test-release-workflow.py` checks the workflow's structure: manual-only
trigger, least-privilege permissions, protected environment, step ordering,
keychain cleanup on every path, verification before publication, and that no
workflow publishes on push or tag.

None of these prove that a real toolchain, a real Developer ID signature, real
notarization, or an actual Actions run succeeds; those need the real build, a
networked notarization, and optionally a clean-Mac rehearsal.

## Homebrew coordinated preparation — publication blocked

Publication-enabled runs finish with **Publish verified formula to tap** in
the same protected job, only after publication succeeds; dry runs (`publish=false`)
never write to the tap. The native/runtime acceptance code gate is still closed
(`release/homebrew/formula_set.py`); do not dispatch either workflow yet. Once
accepted, one commit updates `Formula/cercano-agent.rb`, `Formula/cercano-cli.rb`
and dependency-only `Formula/cercano.rb` on
`cercano-ai/homebrew-cercano`'s `main`, with no pull request or approval prompt.

Before writing, the updater checks the CI archive and regenerated formula,
then downloads the publicly published macOS archive and its checksum sidecar.
Both must match the digest recorded by the build. Nothing is extracted or run.
An identical complete formula set is a successful no-op. A newer version, or a
same-version change of digest/formula, is refused. Concurrent edits use the
Git Data API's non-force branch advancement; a conflict causes up to three fresh
head/tree/set validations, never a force update. One tree and commit contain all
three formulas; unrelated tap files are preserved. Ambiguous ref responses are
reconciled by rereading the exact complete set, not blind mutation retries.
Recovery requires all three formulas from the original reviewed artifact; legacy
single-formula artifacts cannot publish the split. See `release/homebrew/README.md`
for the supported explicit manual monolith migration and local private-index
review-commit preparation. Neither implies crash-atomic package installation.

Use the [Homebrew preparation guide](../release/homebrew/README.md) to validate
all three rendered formulas against the local archive and produce a read-only
review plan from a private tap snapshot. This does not update a checkout or tap,
create commits, download release assets, or grant release approval. Missing old
single-formula artifacts cannot be promoted by bypassing sibling validation.

The approved migration policy is **explicit one-time operator migration**, not
automatic unlink, overwrite or removal. Its instructions are for a separately
authorized disposable rehearsal only. Historical tap distribution is established
(official v0.25.2 monolith; personal v0.20.3); installed-user applicability, real
migration/failure recovery, native changed-source A/B behavior, real local
inference after idle/restart with unchanged external runtime paths, human
acceptance, and final signed/notarized-package smoke remain gates. Existing
same-source synthetic native evidence does not satisfy them; see the
[human acceptance handoff](../efforts/macos-native-validation-closeout/human-acceptance.md).

Future separately authorized live promotion must review the entire set, make all
three dependency definitions available in one commit, and compare-and-swap the
reviewed tap base without force updates. It must preserve downgrade, same-version
drift, published-asset checksum/provenance, and least-privilege transport guards.
There is no enabled implementation of that transaction in this slice; the old
single-file API helpers are not a safe fallback. Do not provision credentials,
retry recovery dispatch, or remove guards to work around this boundary.
