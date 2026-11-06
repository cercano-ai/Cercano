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


## Continuation — verified transferred archive; guest upgrade reached B, controller failed

2026-11-05. This section supersedes the earlier transfer-blocked status, not its
preserved observations. **Phase 4 remains incomplete.** This continuation read the
actual preserved guest logs, checked complete archive hashes and current guest
inventory; it did not rerun the build or claim a passing rehearsal.

### Transfer/source and command provenance

All guest commands used `prlctl exec d5633dbc-c8a1-49fc-a420-df6d5d60d952`.
Read-only `prlctl list -i` reports host folders and host-defined sharing off;
`/bin/ls -la /Volumes` shows only Macintosh HD -> /. No sharing was enabled.
No transfer was repeated and no listener was opened in this continuation.

Host hashlib SHA-256 of `.phase4-d33af63ef86c9c6bf37a4110bfa09eef2bcb40396f29469f740156d5ecb01ee4.tar.gz`
and guest `/usr/bin/shasum -a 256 /tmp/cnp.h8sjTc/phase4-d33af63ef86c9c6bf37a4110bfa09eef2bcb40396f29469f740156d5ecb01ee4.tar.gz`
both returned `d33af63ef86c9c6bf37a4110bfa09eef2bcb40396f29469f740156d5ecb01ee4`.
This is a fresh complete-byte equivalence check (guest checksum read as root),
not a claim that this continuation performed extraction or a UID501 build.
The partial `phase4-source.tar.gz` was not used. Host archive inspection found
1,925 manifest files, revision `6b1083073baa9f25736194d324f1a3d78420a7a8`,
and the required `source/server/internal/compaction/testdata/real_conversation.json`.
Git history confirms harness checkpoints `3bf23e41` and `6b108307` precede this snapshot.

Preserved evidence directory: `/tmp/cnp.h8sjTc/phase4-6b1083073baa/native-evidence`.
Individual `/bin/cat` reads of `controller.log`, `upgrade-A.log`, `upgrade-B.log`,
`explicit-agent-cleanup.log`, both formula files, and residual logs succeeded.
Commands recorded in those logs include actual `brew upgrade --formula
fixture/cercano/cercano-agent` for .902 -> .903 and .903 -> .904, and
`brew cleanup fixture/cercano/cercano-agent`.
Controller invocation was `controller.test
-test.run=^TestNative(ProductionController|ControllerAssertions)$ -test.v
-test.timeout=360s`.

Fresh `/usr/bin/shasum -a 256` of preserved built binaries:

| Binary | SHA-256 |
| --- | --- |
| cercano-0.0.903 | bf8d600c8d2a88ac2037e75107a24b6ce47fc73f4a16a494589cd342efc3e123 |
| cercano-0.0.904 | 90a53dfe91c4e5aa59edc2dba869e72eaff84cc0372dfe7ca6974af3fd96d26d |

Preserved formula archive digests (read from formulas, not freshly hashed archives):
A `97a8ad5d4636c67ea8f99550c5641cc450779cbfdb8271fce2d71c0f32e31400`;
B `dedd9c2b8eb20d50de578ddb7ffde41099ac847f08a0488abf7876f725ab2666`.
SHA-256 of retrieved controller.log bytes:
`0ccf50519b297597a8f254f87925ee5eb67d6bbb29c1c40b17f933449842599e`.

### Actual failure and bounded correction

The controller logged A host PID 34641 before its first turn and the matching
A binary digest. Effective `brew config` in the hook reported
`HOMEBREW_NO_CLEANUP_FORMULAE: cercano-agent`, without a blanket cleanup disable.
Upgrade installed B. Explicit cleanup printed
`Error: Refusing to clean cercano-agent because it is listed in HOMEBREW_NO_CLEANUP_FORMULAE!`
but returned **0**. The harness incorrectly expected **1**, so the hook failed
and the controller stopped before normal fresh-worker dispatch. Assertion helper
subtests passed, but they are not native continuity acceptance.

Corrected the harness to require status 0 and retain its refusal-text and
A device/inode/hash assertions. Added a pre-build guard rejecting already-installed
B: a no-op upgrade must never masquerade as the required transition. Python AST
syntax check passed; corrected harness SHA-256:
`1c085a2a6f9cbb5384990ac36cd6f940b831ae9deb173bfee6e02b69371fd55c`.
This correction has not been executed in the guest.

### Teardown, residuals, and precise blocker

The preserved controller log reports `owned process cleanup complete (no signals)`
and preserved failed fixture `/private/tmp/nc-2173132018`. The current read-only
`/bin/ps -axo pid=,command=` inventory contained no cercano or that fixture path.
No signal fallback, new service, production change, package reset, host install,
configuration change, external llama action, push or merge occurred here.

Fresh UID501 inventory command:
`/usr/bin/sudo -u test /usr/bin/env HOME=/tmp/cnp.h8sjTc/home
XDG_CONFIG_HOME=/tmp/cnp.h8sjTc/config HOMEBREW_NO_AUTO_UPDATE=1
/opt/homebrew/bin/brew list --versions cercano cercano-agent cercano-cli`
returned:

```
cercano 0.0.902
cercano-agent 0.0.901 0.0.902 0.0.903 0.0.904
cercano-cli 0.0.902
```

The precise remaining execution prerequisite is a disposable package baseline
without already-installed B. Ordinary `brew upgrade` cannot recreate .903 -> .904
with .904 already installed. This task authorizes that deliberate upgrade, not
manual manager-owned file/link rewriting, rollback, uninstall of retained kegs,
or a different VM. None was silently substituted. Existing archives, packages,
synthetic exclusion, caches, fixture state and logs remain preserved. A clean
baseline/reset decision is required before repeating the actual transition.

Still unverified: post-explicit-cleanup A inode/hash assertion, FIRST normal
fresh A worker turn and mapped executable, exact final/history/completion counts,
eligible B session while A lives, authenticated A reconnect and later turn, and
successful-scenario authenticated teardown. The failed run's cleanup result does
not establish those successes. Parent-owned plan was not edited.
