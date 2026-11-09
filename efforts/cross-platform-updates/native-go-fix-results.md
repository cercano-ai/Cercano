> Sanitized derivative: see `windows-sanitized-provenance.md` in the effort directory. Historical capture hashes/lengths do not authenticate redacted bytes.

# Native Go serializer recovery and single bounded fixture rerun

## Result — partial, NOT lifecycle acceptance

Exact disposable clone `{SANITIZED_VM_2}` only. Original VM untouched. Historical diagnosis and recovery evidence retained unchanged. No full product build, MSI, service, security-policy change, git/spec/plan edit, host download, forced restore, or fixture kill.

### Authorized old-harness cleanup

`native-go-serializer-cleanup-collected.json` contains before-stop evidence, post-stop query, and final collected transcript. The one-time script `release/chocolatey/tests/native-go-serializer-recover.ps1` opens and retains an OS handle, verifies handle creation time `2026-11-06T20:12:13.5351150Z` against the prior millisecond evidence, exact executable/command line for run `native-go-20261106201208`, and absence of active choco/fixture processes before termination through **that handle** (requested exit 91). Collected status records successful exact-harness termination; subsequent query found no PID 7440. No other process termination was authorized/performed. This is test-harness cleanup, not a package-script policy.

The final collected original transcript ends at `MANAGER closed-upgrade-held exit=0`; forced harness termination did not manufacture a transcript footer or a completed experiment. Original closed package remains installed at 2.0.0.

### Reproduction, fix, and regression

- `native-go-serializer-reproduction-execution.json`: raw `Get-Content` event strings expose `PSPath,PSParentPath,PSChildName,PSDrive,PSProvider,ReadCount,Length`; raw deep conversion did not finish before the bound, exit **124**, elapsed **9.8672 seconds**, stderr starts CLIXML. No completed JSON or readable specific red error was captured. The earlier transport failure remains separately preserved in `native-go-serializer-reproduction.json`.
- `native-go-plain-json.ps1`: JSONL events become explicitly typed plain records. Strings are flattened; only supported primitive/list/dictionary/note-property records enter .NET serialization. Raw provider objects are rejected. Projection bounds: 2 seconds, 100,000 nodes, nesting 16, 8,000,000 string characters; serialization worker join: 5 seconds, output limit 16 MiB. A timeout throws; background serialization cannot hold the process open. This is plain-data projection, **not increasing ConvertTo-Json depth**.
- `Save` writes `SAVING`, then `RUNNING`; failure sets `SAVE_FAILED` and latches against retry of the failed graph. Outer catch records failure and does not reserialize in finally. Each of its two conversions has the bounds above; file I/O is not claimed to have a wall-clock deadline.
- `native-go-serializer-regression.json`: pass, **120 ms**; event version/hash round-trip, accidental decorated-string flattening, raw provider rejection, cycle rejection. The single fixture experiment subsequently serialized its held-operation records normally, crossing the old stall boundary.

### Exactly one fresh-ID fixture experiment

Run `C:\CercanoRunner\phase1\native-go-20261106210423`; package prefix `cercano-native-go-20261106210423-`. Reused existing clone artifacts only after matching known SHA256; source also verified. No new fixture build/download.

| Item | SHA256 |
|---|---|
| fixture Go source | `bea12a2ac5b86f237aa3abfb374f96ab74bed62073959ac39ce609eedf595438` |
| native ARM64 A (1.0.0), 1,955,840 bytes | `DECEF429AEE4CCAB0233D35DB619A85515CED6EC8E2CA689CECF9080DDD41E1E` |
| native ARM64 B (2.0.0), 1,955,840 bytes | `71EB50461447D4B654FF9FE6EEAF98D35D214C0ED8CFD806C99B82961A7E5B45` |
| resource A | `E6FA40966629E22774B8E79E12EDAF0593DE521EC56F986B5C55CCE2B41CA5FE` |
| resource B | `F5718B4F1CC2902DEAE1DC0376488528C00869B400446812F39E9E0269E4F849` |

Evidence: `native-go-fix-run.json`, `native-go-fix-setup.json`, `native-go-fix-execution.json`, and full collected owned text in `native-go-fix-evidence.json` (including final rerun transcript, results, manager logs, hashes, and snapshots). Guest exit **1**, status **FAILED**, error **Cooperative exit assertion failed**, executed probe line 106. The original Python wrapper returned 0 despite this guest failure; wrapper now propagates nonzero guest failure. Do not mistake the transport wrapper exit for a test pass.

| Check | Actual outcome |
|---|---|
| Closed A direct/shim | Exact A/resource output, exit 0 |
| Closed held B direct/shim | Exact B/resource output, exit 0; installed payload SHA matched source B before launch |
| Closed ordinary retry | Exit 0, **upgraded 0/1** (actual no-op, not a demonstrated conflict repair); subsequent direct/shim exact B output, exit 0 |
| Closed cleanup | Uninstall exit 0, payload/shim absence asserted |
| Running A identity | Native A ready record matches PID 3216, exact path, A version/resource; creation `2026-11-06T21:05:12.7314560Z` unchanged and alive before/after held upgrade |
| Running held upgrade | Exit 0, metadata/payload/resource B; live A survived. Chocolatey warned it could not delete the mapped A executable under `lib-bkp`; it nevertheless reported upgrade success. Not an actionable nonzero failure and not proof of recovery |
| Cooperative stop | Stop file written; bounded wait returned; stdout includes A output then `cooperative-stop`; subsequent owned process query empty. **Exact numeric fixture exit was null**, so assertion correctly failed rather than claiming exit 0 |
| Locked executable/resource | **Not reached**; no claim about actionable failures or subsequent retries |
| Running ordinary retry / fresh B activation / cleanup | **Not reached** |
| Synthetic sentinel | All three synthetic file Path/Hash records before vs after failure equal |

### Post-failure bounded harness correction (no second experiment)

The probe now reads `$proc.Handle` immediately after `Start-Process`, retaining a query handle while alive so Windows PowerShell can recover the exit code. `native-go-exit-handle-test.ps1` launches only a minimal PowerShell `exit 0` child, waits at most 5 seconds with no kill fallback, and asserts numeric exit 0. Guest execution of this exact test passed (`native-go-exit-handle-regression.json`, transport exit 0, retainedHandle=true, exit=0). This does **not** retroactively establish the fixture's numeric exit code or turn the failed experiment green. No second native fixture experiment was run.

### Residual ownership / stopping point

Read-only final collection (`native-go-fix-evidence.json`) found **no matching probe, choco, or native fixture processes**. New closed package removed. New running package remains installed at **2.0.0**, with its shim, package metadata and `lib-bkp` A executable evidence retained; no forced manager restore or uninstall after failure. Old `cercano-native-go-20261106201208-closed|2.0.0` remains untouched. Fresh exe-locked/resource-locked IDs were never installed. Both run directories, feeds/packages, synthetic-only sentinels, scripts and evidence remain owned by this test in the disposable clone. No further manager transactions were issued. The locked-case matrix and exact running-fixture exit/retry remain incomplete; do not promote this to product lifecycle acceptance.
