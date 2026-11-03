# Phase 1 bounded baseline verification

## Directly observed — 2026-11-03

- Read Phase 1 of `plan.md` and `docs/agent/self-dev.md`. The effort directory contained only `plan.md` and `spec.md`; no previous verification file or saved baseline artifacts were available. No broader audit was performed.
- Read all Go tests and implementation in `source/server/pkg/update` before execution. Tests use temporary cache/install fixtures and local HTTP servers (plus localhost:1 for failure). Successful release checks call `DetectInstallMethod`, which may execute read-only `brew list cercano`: this suite is not fully hermetic, but contains no package installation, live binary replacement or agent launch actions.
- Ran exactly one focused test command, from `source/server`:

  ```text
  go test ./pkg/update -count=1 -timeout=60s
  ok  cercano/source/server/pkg/update  1.297s
  exit 0
  ```

- This directly establishes the update package's existing unit-test baseline, including advisory upgrade guidance and unchanged temporary fixture binaries/symlinks. It does not establish survival of native-manager upgrades, package cleanup, installed host/session continuity or automatic retirement. No failing probe was observed in this run.
- No separate builds, installs, live-agent actions or manual git inspection were performed. Source and specification were not changed.

## Prior reported trace — not independently revalidated here

The recovery task reports that installed executable/resource paths remain replaceable. Development-generation retention is a reuse candidate, not proof that installed discovery, reconnect and later worker starts use complete retained bundles after native-manager replacement or cleanup. These are reported baseline findings, not results of the test above; no saved detailed trace was available in this effort directory to substantiate additional path/line claims.

Reuse boundary: preserve native-manager package ownership; consider development-generation retention and session binding only as narrowly tested implementation candidates. Do not import the old custom-updater branch wholesale. Installed resource completeness, shared-runtime nonreplacement, shared-store admission and safe retirement still need their own evidence.

## Acceptance blockers and remaining scope

Disposable native runners have not been provided/established for Homebrew/macOS, Debian/APT Linux, or Chocolatey/Windows. Missing resources are disposable machines/VMs, fixture native package sources/artifacts and isolated user/session state; Windows locked-file and Unicode/space-path coverage also requires a native Windows runner. No runner provisioning or credential search was attempted. This is an installed-system acceptance blocker, not a platform pass; production signing/publication remains separately unauthorized.

The bounded recovery is complete, but Phase 1 as a whole is not. Installed-discovery, pinned-session, worker-generation, modelhost nonreplacement and formula-rendering baselines were not rerun. Trace and broader baseline tasks remain in progress. Only the disposable-native-environment task is changed to blocked; no existing completion or branch/base identity claim is newly verified here.
