# macOS release build

For the authoritative operator sequence, see the [release runbook](releases.md).
**Full publication is blocked:** a required publication preflight checks the
closed acceptance gate before all builds, signing and public mutations.
Read the runbook STOP gate; tap-only recovery remains acceptance-gated.

`scripts/build-macos-release.sh` produces the signed Apple Silicon release
archive shared by the three Homebrew formula definitions. It builds both binaries from one
tag-derived version, verifies them, signs them through
`scripts/sign-macos-release.sh`, and packages exactly the layout
`release/homebrew/render_formula.py` accepts:

```
cercano-<version>-darwin-arm64/
  bin/cercano
  bin/cercano-cli
  LICENSE
  README.txt
  runtime-inventory.json
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
receives it, rather than trusting the machine that produced it. Set
`EXPECTED_SHA256` from the trusted build record and `NOTARIZATION_EVIDENCE_DIR`
to the accepted evidence directory before running:

```bash
python3 scripts/verify-macos-release.py dist/cercano-1.2.3-darwin-arm64.tar.gz \
    --version 1.2.3 \
    --sha256 "$EXPECTED_SHA256" \
    --notarization-evidence "$NOTARIZATION_EVIDENCE_DIR"
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

### Workflow behavior and provenance

See the [release runbook](releases.md#5-workflow-ref-source-and-tooling-provenance)
for exact source selection, tag immutability, dispatch and approval requirements.
The workflow is manual-only and defaults to `publish=false`. Every platform
checks out the explicit version tag and verifies HEAD matches its commit. The
selected workflow ref supplies the executing workflow; build code, tests,
workflow fixtures and Homebrew tooling remain tag-bound. Chocolatey tools are
separately pinned to `github.workflow_sha`.

A new dispatch from a reviewed ref can select a corrected runner for an unchanged
old source tag; rerunning an old run does not adopt newer workflow changes.
This does not enable newer tag-bound publication tools or justify rebuilding
published artifacts. Never move the tag or use `CERCANO_ALLOW_UNTAGGED` in CI.

After publication preflight, Windows/Linux build first without secrets. The protected macOS job then runs its
test gate, builds/signs once, notarizes, verifies and renders all three formulas.
It destroys its temporary keychain and retains artifacts for 14 days. A build-only
run still needs signing approval. Publication-enabled runs download this run's
platform artifacts, verify their recorded digests and upload immutable GitHub
assets before invoking the coordinated tap updater. This sequence is reachable
only after future acceptance; GitHub and tap updates are not one transaction.
Integrated Chocolatey depends on this job succeeding.
The separate Chocolatey publisher has its own protected deployment gate.

There is no prior-rehearsal promotion input. A later dispatch is a new build, not
promotion of earlier artifacts. Do not rerun the consolidated job to repair
publication. Existing-tag recovery and full publication require the runbook's
channel-specific review and current blocker resolution.

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

## Homebrew publication and recovery are disabled

The renderer creates `cercano.rb`, `cercano-agent.rb` and `cercano-cli.rb` as a
coordinated set. The workflow retains all three; this does not publish them.
`release/homebrew/update_tap.py` and local promotion share the closed
`formula_set.PUBLICATION_ACCEPTED = False` gate before credentials/network access.
The main workflow checks acceptance before builds or public mutations. Tap-only
recovery uses the original complete three-formula artifact and immutable
provenance, but cannot bypass this gate. Neither route is currently enabled.
See [publication mechanics](../release/homebrew/README.md#coordinated-publication-mechanics-acceptance-gate-remains-closed)
and [partial failure handling](releases.md#7-partial-failure-and-recovery).
A valid token cannot substitute for acceptance.
Any future publication identity should be narrowly scoped to tap Contents
read/write access, configured through approved secret management without
bypassing branch protections. Keep GitHub assets and version tags immutable.
