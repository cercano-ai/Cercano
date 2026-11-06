# Phase 4 attempt — STOPPED during read-only preflight

2026-11-05. **No A/B rehearsal result. No qualification claim.** This attempt did not extend or execute the controller, checkpoint a harness, transfer source, build .903/.904, install packages, run cleanup, or launch any product/service. The previously stopped task is not treated as execution evidence.

## Source and preserved work

- Branch: `feat/native-packages-session-safe`.
- HEAD: `27a8a38d608cc595925d95748291257f6d249d6c`.
- Initial `git status --short --branch`: only `efforts/macos-native-validation-closeout/plan.md` modified. That parent-owned change was left untouched.
- Read the effort spec, plan, native-controller notes, residual-cleanup note, and controller source. The existing controller copies its supplied binary into a synthetic keg; it was **not** used as evidence for actual Homebrew installation.
- SHA-256, obtained with `shasum -a 256`:
  - `source/server/cmd/cercano/native_controller_test.go`: `e9fc79e7d3565026075fdc57b094efa4ebfb9ddfcfc75de6c6c1eff6d513c130`
  - parent-owned plan: `49252d36b1fdcfbe84b06ddd6c98a262cb6b4c271816471919aadf95a6b36ddc`
  - native-controller notes: `b580a4c2a0d235178d2ad6c4c405e4f20cb0784637c8e7180f9df3bee7b788b2`
  - residual-cleanup note: `f1a65500b027e134dcf29dcb9d5bd7c8904b786e4c86ceaa93b02ad16c14d0dd`

## Preflight failure and safety stop

An invocation mistake was detected before any mutating guest command. Passing a multi-command `/bin/sh -c` body as one tool argv element through `prlctl exec` did **not** preserve the expected shell scope. Although its first `id` reported test UID 501, later observations were not reliably inside that user's synthetic environment. Symptoms included `brew --version` reporting a shallow/no-git fallback, a Git dubious-ownership error for a repository subsequently observed to be test-owned, and `brew list` rejecting execution as root. Therefore multi-command observations are not accepted as proof of the authorized execution context.

Stopped rather than extending this invocation pattern to source transfer/build/package changes. No safe.directory override, ownership change, root Homebrew bypass, environment-policy injection, or configuration repair was attempted. Direct single-program sudo/env commands below worked; this does not retroactively validate the earlier shell batches. A subsequent execution must use a separately bounded, UID-asserting guest harness invoked directly, not interpolate shell bodies through this transport.

## Exact guest calls

All calls used `prlctl exec d5633dbc-c8a1-49fc-a420-df6d5d60d952`. The following lists the remaining argv in shell-quoted notation (the execution tool itself supplied an argv array, not a local shell). Each call had a 60-second tool deadline.

Problematic read-only batches:

```sh
/usr/bin/sudo -u test /usr/bin/env HOME=/tmp/cnp.h8sjTc/home XDG_CONFIG_HOME=/tmp/cnp.h8sjTc/config /bin/sh -c 'id; sw_vers; test -d /tmp/cnp.h8sjTc/gomodcache && echo module-cache-present; ls /tmp/cnp-homebrew-evidence /tmp/cnp-macos-update-retention; /opt/homebrew/bin/brew --version'
/usr/bin/sudo -u test /usr/bin/env HOME=/tmp/cnp.h8sjTc/home XDG_CONFIG_HOME=/tmp/cnp.h8sjTc/config /bin/sh -c 'cat /tmp/cnp-macos-update-retention/policy-guest.py | head -100; cat /tmp/cnp.h8sjTc/config/homebrew/brew.env; ls -l /opt/homebrew/bin/go; df -h /; /usr/bin/git -C /opt/homebrew describe --tags --always'
/usr/bin/sudo -u test /usr/bin/env HOME=/tmp/cnp.h8sjTc/home XDG_CONFIG_HOME=/tmp/cnp.h8sjTc/config /bin/sh -c 'ls -l /tmp/cnp-macos-update-retention/policy-guest.py /tmp/cnp-macos-update-retention/policy-continue.py; head -70 /tmp/cnp-macos-update-retention/guest.py; /opt/homebrew/bin/brew list --versions cercano-agent cercano-cli cercano; /opt/homebrew/bin/go version; /usr/bin/stat -f "%Su %u %N" /opt/homebrew /opt/homebrew/.git'
```

Tool exits were respectively 0, 128, 0. In the last call the final exit masked the Homebrew rejection; no assertion relies on that aggregate success. Observations included macOS 26.5.2 build 25F84, Go 1.27.3 darwin/arm64, 62GiB free, and existing evidence directories. These are observations, not a successfully asserted preflight. The read policy file contained `HOMEBREW_NO_CLEANUP_FORMULAE=cercano-agent`; effective loading was not tested.

Corrected direct single-program calls:

```sh
/usr/bin/sudo -u test /usr/bin/env HOME=/tmp/cnp.h8sjTc/home XDG_CONFIG_HOME=/tmp/cnp.h8sjTc/config XDG_CACHE_HOME=/tmp/cnp.h8sjTc/cache XDG_DATA_HOME=/tmp/cnp.h8sjTc/data /opt/homebrew/bin/brew --version
/usr/bin/sudo -u test /usr/bin/env HOME=/tmp/cnp.h8sjTc/home XDG_CONFIG_HOME=/tmp/cnp.h8sjTc/config XDG_CACHE_HOME=/tmp/cnp.h8sjTc/cache XDG_DATA_HOME=/tmp/cnp.h8sjTc/data /opt/homebrew/bin/brew list --versions cercano-agent cercano-cli cercano
```

Both exited 0. Version output: Homebrew 7.0.12; homebrew-core revision `55711ea862c`, last commit 2026-11-04. Package output:

```text
cercano 0.0.902
cercano-agent 0.0.901 0.0.902
cercano-cli 0.0.902
```

## Assertions, teardown, and residuals

None of the requested A/B dispatch, exact persistence, mapped-worker identity/hash, reconnect, coexistence, automatic cleanup, or explicit cleanup retention assertions ran. No source/archive/product artifact hashes exist for this attempt; no host-to-guest archive equivalence is claimed. No new guest evidence directory was created and no prior evidence was overwritten. Tool transcripts are the raw observation record; this document is the bounded summary.

No owned process was created, so no release/retire or signal was needed or attempted. Existing guest packages and persistent exclusion were left in place. Existing source/cache and evidence directories were not modified intentionally. No real model, cloud, or credential access was requested. No host Homebrew/configuration/live-agent action, push, merge, landing, or production edit occurred. External llama retention remains outside the accepted agent-only exclusion; this stopped preflight adds no runtime-safety evidence.


## Continuation — direct script probe passed; source transfer incomplete

2026-11-05. This continuation does not supersede the preflight above. **Phase 4 remains incomplete.**

The direct script invocation was executed successfully (exit 0):

```sh
prlctl exec d5633dbc-c8a1-49fc-a420-df6d5d60d952 sudo -u test env HOME=/tmp/cnp.h8sjTc/home XDG_CONFIG_HOME=/tmp/cnp.h8sjTc/config GOCACHE=/tmp/cnp.h8sjTc/gocache GOMODCACHE=/tmp/cnp.h8sjTc/gomodcache GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local /opt/homebrew/bin/python3 /tmp/cnp.h8sjTc/phase4-guest.py
```

It asserted UID 501, isolated HOME/XDG/Go-cache settings, offline Go environment, cache presence, and the literal persistent exclusion policy line. It reported Go 1.27.3 darwin/arm64 and script SHA-256 `f941bc54a03d360be6062c53d78e0e8efdebc7c224a6b088dcb31c9968ea417f`. This is **not** proof of effective Homebrew policy loading. The earlier shell-scoping failure is not the blocker.

A local transfer helper attempted a tracked-source snapshot at revision `51927714d5f1e5cde3d87dcadcd6c11755959ab7`, selecting Go files, module files, third-party replacement sources, and embedded assets including compaction `real_conversation.json`, with a per-file SHA-256 manifest. The assembled compressed archive was 6,346,750 bytes, SHA-256 `8dbd24bd92673b9c55c42a5071f40d26fa127b71d7f1edf4da2d3ade039f4879`. **No complete guest transfer equivalence was established and this is not an accepted build snapshot.** No logs or credential files were intentionally selected.

Transfer failed with exit 255, `PrlJob_GetResult: Invalid argument. An invalid argument was passed.` Retries with smaller chunks also failed. The last observed guest archive length was 1,206,000 bytes, with its prefix hash checked against the locally generated archive before retry. The partial `/tmp/cnp.h8sjTc/phase4-source.tar.gz` remains; it was not extracted or built. The transfer helper used quoted Python `-c` snippets with UID assertions, rather than the requested absolute guest-script entrypoint. This was a procedural deviation, not accepted rehearsal execution; the failed helper is not checkpointed as a supported harness. No multi-command shell body or subprocess shell execution was used.

No harness extension, offline dependency build, .903/.904 packaging or installation, automatic/explicit cleanup, worker dispatch, persistence assertion, B coexistence, or A reconnect ran. No modelhost or product process was started; authenticated release/retire was consequently not needed, and no PID signals were sent. No package mutation command ran. The .901/.902 inventory above remains historical, not freshly reverified; no new residual package inventory claim is made. No production files, host Homebrew, live state, credentials, or policy were modified. The parent-owned plan remains untouched. This checkpoint preserves only evidence and the read-only guest probe, not a completed native harness.
