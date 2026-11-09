> Sanitized derivative: see `windows-sanitized-provenance.md` in the effort directory. Historical capture hashes/lengths do not authenticate redacted bytes.

# Windows Chocolatey transaction audit (source only)

Date: 2026-11-06. **Native acceptance remains unverified.** This is a narrow source trace, not a Windows test report, package implementation, or replacement execution plan.

## Scope and provenance

Read the [approved Windows amendment](spec.md#approved-windows-scope-amendment--2026-11-06): ordinary machine-wide Chocolatey replacement; affected components must exit; no forced termination, retained Windows generations, or privileged user-agent launch. Running/locked-file failures must be actionable, user data must survive, and failed replacement must not be reported as successful activation. An unaffected disconnected CLI during agent-only upgrade is a validation case, not a support guarantee.

Independently retrieved official `chocolatey/choco` source over HTTPS at commit **`194ac6262ac75571c44aa0a431c3ab6f36fa73be`**. The [official tag API](https://api.github.com/repos/chocolatey/choco/git/ref/tags/2.7.5) returned `refs/tags/2.7.5`, object type `commit`, and that exact SHA. Source was read as text, including line-numbered extraction from the raw files; no downloaded code was executed, no git commands were used, and no native installation was changed.

## Verified trace

All source links below pin that SHA; line numbers refer to the original source, not rendered/extracted text.

| Claim | Exact source and finding | Safety implication |
|---|---|---|
| Before-modify errors do not reliably veto modification | [`NugetService.cs` L2974–3023][before]: when an install location and callback exist, the callback receives a copied result (L2982–2991). Its error messages are copied back as warnings (L2993–3010). The enclosing catch logs exceptions without rethrowing (L3018–3022). Backup follows the callback (L3013–3015); the catch covers both. | A failed `chocolateyBeforeModify.ps1` is not a fail-closed transaction gate in this path. Throwing or returning an error there alone cannot establish safe refusal before mutation. A thrown callback can also skip backup while the caller continues. |
| Upgrade removes old files before installing new NuGet package | [`NugetService.cs` L1773–1836][upgrade]: calls `BackupAndRunBeforeModify` at L1775, then `RemoveInstallationFilesUnsafe` at L1789, explicitly removes old nupkg at L1809, downloads and validates the new package (L1822–1828), then calls `InstallPackageAsync` (L1832–1836). Removal exceptions add an error, invoke the continuation if present, and skip that iteration (L1791–1804). | This is not an atomic swap or proof of an untouched old installation on failure. A new install script is too late to protect all old-file removal. Removal errors are not simply ignored in this caller, but failure reporting does not undo preceding mutations. |
| Unsafe removal can partially delete before reporting locks/errors | [`NugetService.cs` L3087–3144][remove]: under the existing-directory/snapshot guard, iterates files, skips files absent from the snapshot, records a locked-checksum error and continues (L3104–3109), deletes checksum-matching files (L3112–3125), then throws collected exceptions after iteration (L3130–3136). | Some files can be deleted while others remain locked. This establishes a possible partial-delete path, not that every running executable is detected or every failure has this outcome. |
| Windows uninstall removes shims before package uninstall script | [`ChocolateyPackageService.cs` L1507–1543][uninstall]: `_shimgenService.Uninstall` at L1512 precedes `_powershellService.Uninstall` at L1516 (unless the provider is skipped); auto-uninstaller and cleanup are success-conditional. Failure requests rollback at L1542. | A refusal from the uninstall script is not proof that command access remains unchanged: shim removal has already been invoked. This trace establishes invocation order, not that every shim deletion succeeds. |
| Rollback here is package-file recovery, not application-state transaction rollback | [`ChocolateyPackageService.cs` L1773–1813][failed] sets a nonzero exit code if needed and conditionally invokes restoration. [`L1840–1916`][rollback] requires usable location/version/backup, may prompt explicitly for “package files only” (L1896), and moves backup to the package directory at L1911–1912. [`NugetService.cs` L2382–2401][backup] backs up the package directory and warns on a reported access error. | This routine does not replay installation scripts or restore running processes, user state, external installer effects, or command shims as a separate step. Backup/restoration is conditional, not guaranteed recovery. Do not generalize this narrow trace into a claim that every Chocolatey provider or external installer has identical rollback behavior. |

## Consequence for Windows acceptance

The reported narrow claims are confirmed, with the conditions above. Chocolatey's before-modify hook, locked-file detection, and package-file rollback **do not by themselves establish the amendment's safety guarantees**. Closing the affected application is a documented prerequisite, not evidence of race-free replacement. A clear nonzero manager failure still requires checking whether files/commands were left partially changed; manager success is not proof of a new running version. No architecture change or production fix is proposed by this audit. The required behavior remains an acceptance gate for a separately reviewed replacement plan.

## Minimal native running / locked / relaunch matrix (not executed)

Run only in an isolated Windows VM with local test packages A and B, pinned Chocolatey 2.7.5, and disposable user accounts/data; never the developer's live installation. Exercise both externally initiated manager commands and any supported application-guided path. All rows below are **pending native evidence**.

| Case | Operation / state | Required observation |
|---|---|---|
| Closed baseline | Install, A→B upgrade, uninstall: standalone CLI, standalone agent, and convenience package; affected processes closed, shell terminals left open | Expected commands and exclusive ownership; remote-only CLI needs no local agent; successful replacement followed by fresh launches runs B; uninstall preserves configuration, conversations, credentials. |
| Running affected CLI | CLI upgrade and uninstall with CLI active | Clear failure or request to exit; no forced kill or misleading success. Record any partial file/shim changes; after exit, manager-owned recovery/retry yields usable commands and correct version. |
| Running agent / workers | Agent upgrade and uninstall with agent active, then with an active worker | Same refusal/data requirements; no promise of preserving an in-flight turn, and no forced termination. Verify all affected processes, not just parent agent. |
| Locked payload | Hold a package executable/resource open with deletion denied while other snapshot files are removable; upgrade and uninstall | Actionable failure; capture exit status, logs, file inventory/hashes and shim state to expose partial mutation. Unlock/retry must not falsely activate an incomplete installation. |
| Concurrent launch / relaunch | Attempt affected launches during replacement; retry after success and after induced failure/interruption | No automatic restart or elevated user-agent launch; no successful activation claim for partial files. After successful replacement, new launches use B, not a stale command/path. |
| Agent-only coordination | Keep unaffected CLI disconnected while replacing agent; trigger reconnect during and after replacement | Reconnect must not relaunch A during replacement. Later reconnection requires supported protocol compatibility. Remains unsupported until demonstrated. |

Cross-cut these cases with partial CLI/agent upgrades, spaces/Unicode paths, elevated same-user and alternate-admin/SYSTEM contexts. Record account identity, process/version evidence, exit codes, logs, before/after package and shim inventories, and disposable user-data checks. Verify no other account's state is touched. Include an interrupted replacement and verify recovery through Chocolatey rather than custom repair. This minimum transaction matrix does not waive the amendment's broader signing, integrity, publication, or native acceptance obligations.

[before]: https://github.com/chocolatey/choco/blob/194ac6262ac75571c44aa0a431c3ab6f36fa73be/src/chocolatey/infrastructure.app/services/NugetService.cs#L2974-L3023
[upgrade]: https://github.com/chocolatey/choco/blob/194ac6262ac75571c44aa0a431c3ab6f36fa73be/src/chocolatey/infrastructure.app/services/NugetService.cs#L1773-L1836
[remove]: https://github.com/chocolatey/choco/blob/194ac6262ac75571c44aa0a431c3ab6f36fa73be/src/chocolatey/infrastructure.app/services/NugetService.cs#L3087-L3144
[uninstall]: https://github.com/chocolatey/choco/blob/194ac6262ac75571c44aa0a431c3ab6f36fa73be/src/chocolatey/infrastructure.app/services/ChocolateyPackageService.cs#L1507-L1543
[failed]: https://github.com/chocolatey/choco/blob/194ac6262ac75571c44aa0a431c3ab6f36fa73be/src/chocolatey/infrastructure.app/services/ChocolateyPackageService.cs#L1773-L1813
[rollback]: https://github.com/chocolatey/choco/blob/194ac6262ac75571c44aa0a431c3ab6f36fa73be/src/chocolatey/infrastructure.app/services/ChocolateyPackageService.cs#L1840-L1916
[backup]: https://github.com/chocolatey/choco/blob/194ac6262ac75571c44aa0a431c3ab6f36fa73be/src/chocolatey/infrastructure.app/services/NugetService.cs#L2382-L2401
