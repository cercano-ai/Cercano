# Windows runner upload investigation — 2026-11-14

## Outcome

The existing `guest.py --system` channel works as **NT AUTHORITY\SYSTEM**. This supersedes the earlier credential-blocked report; it does **not** establish ordinary-user access or ordinary-user ACL acceptance. The bounded transport now successfully uploads, verifies and executes the 124,687-byte native payload. Native validation failed, separately from transport success.

The inherited `PR84_USER` / `PR84_PASSWORD` mechanism is the existing non-SYSTEM mechanism in `guest.py`; availability-only evidence in `windows-runner-fresh-access.json` reports both absent. Existing runner reports and `WINDOWS-FAILURE-INVESTIGATION.md` document environment-supplied authentication; no secret values were exposed, credentials discovered, or settings changed. SYSTEM is the explicitly authorized existing channel, not a newly configured privilege or credential path.

## Reproduction prerequisites and safety limits

This is a retained investigation runner, not a general-purpose Windows test service. It requires the existing Parallels VM `cercano-windows-packaging-20261106-1117-full`, the runner's hard-coded guest upload directory already present, and preinstalled guest Go tooling. `guest.py` and `windows-env.ps1` are required adjacent dependencies. Ordinary-user execution uses environment-supplied `PR84_USER` and `PR84_PASSWORD`; do not put credentials in payloads or reports. The inherited wrapper passes credentials to `prlctl` as command-line arguments, and captured output is saved verbatim without redaction. Review output before sharing artifacts.

Host subprocess timeouts do not guarantee termination of the `prlctl` descendant or guest processes. A timeout means guest completion is unknown: reconcile guest process state before any further execution. The runner never retries execution automatically; only fixed-offset upload chunks are retried. The native payload used separate guest-side test/process bounds. Cleanup exit zero does not establish removal because cleanup suppresses removal errors.

Independent host-only mocked checks passed for exit propagation, chunk retries, no execution retry on exit 255 or timeout 124, hash-failure execution gating, and preservation of the execution exit on cleanup failure. Syntax compilation passed. These checks do not establish ordinary-user native acceptance.

## Diagnosis and bounded repair

Retained exact probe outputs: `windows-runner-size-probes.json`, `windows-runner-size-probes-2.json`, `windows-runner-size-boundary.json`. The boundary sweep observed success at 1,455 PowerShell characters / 3,880 encoded characters and failure at 1,456 / 3,884 with exit 255, `PrlVmGuest_RunProgram: Unable to open new session ...`. Larger commands also failed. This is an observed boundary, **not a proven deterministic Parallels maximum or a complete explanation of the original Invalid argument error**.

Small uploads also intermittently failed with `PrlJob_GetRetCode: Invalid argument. An invalid argument was passed.` Therefore payload size alone does not explain every failure. `windows-upload-smoke-first.transport.json`, `windows-upload-smoke-file.transport.json` and `windows-current-native-first.transport.json` retain failed attempts. They are not passes and did not execute native tests.

`run-guest-bounded.py` now uses:
- Explicit optional `--system`; otherwise unchanged inherited credential channel.
- 300-character gzip/base64 chunks; a cap of 3,200 characters on the **entire** UTF-16LE/base64 command argument.
- Fixed-offset writes instead of append, so an uncertain reply can be retried without duplicating bytes; at most three attempts on exit 255, with pacing.
- Guest SHA-256 verification of both compressed bytes and decompressed script before execution (mismatch exits 91 / 92).
- Bounded subprocess timeouts, retained per-call actual exits and encoded lengths, actual guest execution exit propagation, and upload cleanup in `finally`.

The final smoke payload printed `nt authority\system` and `SMOKE_OK`, and propagated intentional exit **7**. The native upload verified raw SHA-256 `e27c63f88f5dfc9a32e93b4c15059a458bdf940b7fe6ebe48308c6c92e23c7e4`, gzip SHA-256 `0ff01cea75f105c191cc291a41944dc3fdcaffb325386cebe4fcf9a0ba3cd144`. Its 182 transport calls include three failed chunk attempts (255), retained rather than hidden; retries succeeded. Wrapper exit **1** reflects native failures; cleanup exit **0**. See `windows-current-native.transport.json`.

## Native execution and true exits

See `windows-fix-ownership-acl-native.md`. Exactly **8** focused native invocations were consumed: five initial invocations and three regrouped closeout invocations. No more tests were run. The initial PowerShell process-exit accessor emitted blank exits (`TEST_EXIT=1:` etc.); the numbers 1–5 are invocation IDs, **not exit codes**. JSON records establish failures, but initial OS exits are unavailable. The last three commands captured immediate `%errorlevel%` through `cmd.exe`: **6=1, 7=1, 8=1**. Neither initial missing exits nor SYSTEM acceptance are represented as success.

Exact sanitized commands/selectors and outputs are retained in `windows-current-native.ps1`, `windows-current-closeout.ps1`, their `.log` and `.transport.json` files. Closeout command executed:

```text
python3 .review-pr84-evidence/run-guest-bounded.py windows-current-closeout --system --timeout 500
```

Host/guest wrapper exit **1**, cleanup **0**; raw payload SHA-256 `e4d6e340435e466db6dc16834a9c6ff2c7df6c4ab9470014acd47b17c0ed17b6`.

## Isolation and closeout

13 dirty/untracked source files were hash-verified into a copy at `C:\Users\Public\pr84-20261113\ownership-acl-current-20261114\integration`; original integration was not overlaid in place. `windows-current-overlay.json` contains exact file hashes; a final host hash comparison still matched all 13 source files. Native commands used offline Go dependency settings, 60-second Go timeouts and 150-second outer process bounds. No product edits, installs, VM restart, credential changes, or VM/security configuration changes were made by this investigation.

Original integration manifests contain **3,556 files**. Before, after and final manifests were retrieved byte-for-byte into `windows-current-integration-{before,after,final}.sha256`. All three SHA-256 values are:

`c196c947353f68ae7ab26c1dcb683865b8a1af40bd08995dcd011b8ed76b796e`

Final read-only inspection exited **0** and listed no `go`, `conversation.test`, `server.test`, `dispatch.test`, `compile`, or `link` processes. Exact query and result: `windows-current-final-access.json`. This is a targeted process inspection, not a claim that the VM has no other processes. Upload cleanup commands reported exit 0; candidate and test evidence are intentionally retained. The cleanup script suppresses missing-file/removal errors, so its exit alone is not proof that every transient upload file is absent.

Remaining limitations: ordinary-user ACL acceptance is untested; SYSTEM failures cannot adjudicate it. The underlying Parallels intermittent error is mitigated, not fully isolated. Native results are red. No further native invocation budget remains.
