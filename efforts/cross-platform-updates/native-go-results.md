> Sanitized derivative: see `windows-sanitized-provenance.md` in the effort directory. Historical capture hashes/lengths do not authenticate redacted bytes.

# Native Go Phase 1 probe — blocked partial result

## Verdict

**BLOCKED / INCOMPLETE — not an acceptance pass.** This report audits retained artifacts from isolated run `native-go-20261106201208`, targeting only approved clone `{SANITIZED_VM_2}`. It does not rerun the experiment. The last read-only inspection returned Parallels exit 255: `Unable to open new session in this virtual machine`. Earlier runner status remains unresolved. No further guest execution, concurrent manager mutation, process termination, restore, or cleanup was attempted during final offline audit. Package/feed/evidence state is retained, not claimed clean.

The spec amendment requires actionable running/locked failure, no forced termination/data loss, and truthful activation. Partial mutation alone is not a violation. No native running/locked outcome is established here; neither safety nor a violation is inferred.

## Recorded build

Synthetic stdlib-only source and complete build metadata are in `native-go-build.json`. Compiler: `go1.26.3 darwin/arm64`. Isolated output: `/SANITIZED_HOST_TEMP/native-go-build`. Environment: `CGO_ENABLED=0 GOOS=windows GOARCH=arm64 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off`. Source SHA-256: `bea12a2ac5b86f237aa3abfb374f96ab74bed62073959ac39ce609eedf595438`.

| Artifact | Bytes | SHA-256 |
|---|---:|---|
| A / 1.0.0 | 1955840 | `DECEF429AEE4CCAB0233D35DB619A85515CED6EC8E2CA689CECF9080DDD41E1E` |
| B / 2.0.0 | 1955840 | `71EB50461447D4B654FF9FE6EEAF98D35D214C0ED8CFD806C99B82961A7E5B45` |

Fixture supports ready-file PID/executable/version/resource identity, cooperative stop file, and 300-second maximum lifetime. These capabilities are source facts, not proof the held-process scenario executed. No product build is evidenced or claimed. ARM64 execution is not amd64 production qualification.

## Exact observed scope

Guest root: `C:\CercanoRunner\phase1\native-go-20261106201208`; isolated feed: `ROOT\feed`. Captured package: `cercano-native-go-20261106201208-closed`. Exact command arrays, output and exits are in `native-go-evidence.json` under `native-go-commands.json`; all four captured operations (pack A, pack B, install A, closed upgrade B) returned 0.

| Check | Result |
|---|---|
| Source identity against recorded SHA-256 | PASS |
| Closed A fresh direct and public-shim launch | PASS: exit 0, exact `native-go-fixture version=1.0.0 resource=resource-1.0.0` |
| Closed B installed executable and resource identities | PASS: before-release snapshot matches known B source hashes; metadata lists 2.0.0 |
| Audited old-file removal before B script | PASS, closed case only: log removal attempts for executable/resource at lines 171/174, B script invocation at line 379 |
| Fresh B direct and public-shim launch | NOT ESTABLISHED |
| Running installed A, exact PID survival before release | NOT ESTABLISHED |
| Stopped locked executable and resource refusal/actionability | NOT ESTABLISHED |
| Ordinary exact-B closed retry and fresh launch assertions | NOT ESTABLISHED |
| Resolved shim target | NOT ESTABLISHED; embedded-target search false is not proof of broken routing |
| Final data hashes, feed hashes, cooperative teardown, cleanup | NOT ESTABLISHED |
| Full transcript | FAIL: unavailable/incomplete |

The closed upgrade log records success and B bytes, not a running-executable replacement result. Lock failure before B script and actionable refusal remain untested. Optional new-install-script process-check backstop was not attempted; the running baseline did not complete. No hooks kill or restore processes and no MSI/custom updater/architecture change was introduced by the probe source.

## Evidence and capture limitation

`native-go-result.json` is the structured offline verdict, including source evidence hashes, captured command stdout, exact launch records and remaining matrix. `native-go-evidence.json` preserves captured file contents and manager logs. `native-go-host-transcript.json` preserves available host transport stdout/stderr, but contains no completed probe invocation record. Its collection output ends with a sharing/read error for the active guest transcript. `native-go-final-inspection.json` preserves the failed session attempt; exit 255 means inspection did not execute. **A full transcript cannot be reconstructed, and this requirement is explicitly failed rather than silently omitted.**

Offline checker: `python3 release/chocolatey/tests/native-go-audit.py` completed successfully after handling null entries in recorded source-match lists. Its exit 0 means the partial-result report was generated, not acceptance success. Only separate `native-go*` additions are used; existing checkpoint/evidence/spec/plan files were not edited by this final audit.

## Remaining matrix

All unresolved rows above remain open. Also unqualified: native amd64 and production PE/resource closure; relaunch races and inaccessible-process/unknown-inspection handling; real agent/worker/reconnect coordination; cross-user, standard-user and alternate-admin boundaries; real user data; standalone/convenience/partial component lifecycle and uninstall locks; installed paths with spaces/Unicode; interruption/power-loss/permission/disk failure; signing and runtime delivery. Synthetic sentinel creation does not establish preservation or real account boundaries. No cross-user or relaunch coverage is implicit.
