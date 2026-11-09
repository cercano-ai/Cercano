> Sanitized derivative: see `windows-sanitized-provenance.md` in the effort directory. Historical capture hashes/lengths do not authenticate redacted bytes.

# Windows Phase 1 review-finalized checkpoint inventory

## Handoff maintenance — 2026-11-09

The narrative below describes the historical 2026-11-06 checkpoint, not fresh guest execution or a current staging allowlist. The SHA-256 table has been recalculated for the sanitized derivative bytes; it is not original-capture provenance. Current status and evidence/residual mapping are in `windows-phase1-handoff.md`; the existing `plan.md` has been updated only for handoff maintenance. Installer implementation/native acceptance remain incomplete, and the provider-rejected concurrent-launch validation remains unexecuted with no result. No retry/reroute/rephrasing is authorized.

The current bounded pre-push review confirmed attributable machine/account identifiers in committed raw transcripts, including `windows-provision-output.txt:6-10` and `native-go-recovery-transcript.txt`, plus local user-path context in `native-go-host-transcript.json`. Values are not reproduced here. Historically branch push was withheld pending privacy approval. The user subsequently authorized this separate sanitized derivative; earlier marker checks alone were not publication approval. No Windows/VM experiment or host/guest configuration mutation was performed for this maintenance. The three preexisting untracked diagnosis/race artifacts listed in the handoff remain excluded and unexecuted.

## Historical review — 2026-11-06

2026-11-06. Offline review only: no guest invocation, provisioning, transaction rerun, git operation, or spec/plan edit. Original captures remain unmodified in the private feature worktree; these outgoing copies are explicitly sanitized derivatives. Results are narrowed rather than new acceptance assertions invented.

## Review resolutions

- B-metadata cases assert changed executable bytes only, not expected B identity or resource B. The results table now states exactly that.
- PID survival and direct fresh-payload launch remain attributed historical stdout observations, not assertions or public-shim tests. No consolidated stdout/transcript is present; the collector excludes `transcript.txt`. No recovery/recollection was attempted.
- Retry records remain stored but unasserted; expected-B, resource-B, public-shim version, PID ownership/survival and retry gaps are explicitly listed.
- MSI is an untested candidate, not a necessary minimum. Evidence rejects hook-veto reliance, not all portable approaches.
- Amendment scope is actionable running/locked-file failure, no kill/data loss/false activation, not atomic preservation of every payload/shim on any error.

## Validation performed

`python3 release/chocolatey/tests/assert_evidence.py`: PASS, all 19 recorded cases across phase1/extensions/smoke/consolidated. This checks only the documented stored assertions.

Python built-in `compile(..., 'exec')` syntax check: PASS for `assert_evidence.py` and `guest.py`, without importing/executing guest transport or writing bytecode. No PowerShell helper was executed.

## Evidence sensitivity inventory

The 27 payload/helper/document files below were read as UTF-8 and scanned for common password/API-key/token/secret markers, user paths and transcript identity fields. No credential value was identified by that bounded scan; it is not an exhaustive secret detector or publication approval.

- `windows-provision-output.txt:6-10` contains SYSTEM identity, an attributable guest computer name, timestamps and command-line details. Raw historical capture is preserved; public distribution needs privacy review/redaction of a separate copy, not silent alteration of evidence.
- Captures and helpers disclose VM UUIDs, PIDs, local machine paths, OS/build/architecture, timestamps, package IDs and hashes. These are operational identifiers, not authentication secrets, but can correlate a personal workstation/VM. Runner documentation also carries VM inventory context.
- `fake-credential` is a synthetic sentinel name; helper values are `synthetic-only-*`, and collected CSVs contain hashes/paths, not inherited user credentials. This does not establish that real user data was tested or safe.
- No inherited profiles, real conversation contents, auth tokens, credential store exports or environment dumps were collected by this finalization. Historical capture completeness is not guaranteed by a marker scan.

## Explicit scoped file allowlist

Only the paths below plus this inventory are proposed for a scoped checkpoint. The list is a handoff allowlist, **not a claim about the git index** (no git command was run). It excludes spec.md, plan.md, verification.md, unrelated working-tree content, VM images and archives. Every listed payload file decoded as UTF-8 and contained no NUL bytes; none has a binary/archive suffix. `.nupkg`, `.zip`, `.exe`, `.dll` references inside text are paths/hashes only, not included binaries. Do not use a broad directory add. Raw captures remain subject to the privacy caveat above.

SHA-256 values below describe sanitized derivative repository bytes, not historical guest-script identity. This inventory lists itself without a recursive hash.

| File | SHA-256 |
|---|---|
| `efforts/cross-platform-updates/windows-architecture-output.txt` | `c7e9d9b4781add852a7ef707ee68bc8653a6935ef6884cee48a68bc239d90871` |
| `efforts/cross-platform-updates/windows-barrier-evidence.json` | `34cf37993331f52f427718144d0d85a9777d3c9b77f03cd5440ee080ae0bd712` |
| `efforts/cross-platform-updates/windows-barrier-output.txt` | `a19b2fc6c01902b1e6bc7cd606e6945e00daae7ebb30094dc5ef4e18ebb0edfd` |
| `efforts/cross-platform-updates/windows-checkpoint-output.txt` | `c54d9c5bc86a3d3270a5590b23fa590fab83874dbd3f668d22caccd9cab3b8f2` |
| `efforts/cross-platform-updates/windows-consolidated-evidence.json` | `bba01ad38893ca6d01f713d68c66ad085c50ea3e40415e1f78df30dbb4c72f8f` |
| `efforts/cross-platform-updates/windows-extensions-evidence.json` | `136080a906f11cb02054f7f1f3af8e98a283bda89a7c86fdcd8a601c19df3768` |
| `efforts/cross-platform-updates/windows-extensions-output.txt` | `27be0758a651da4d88c33e4524b188766f7a6c2e65d550288d0d888bcabc1a22` |
| `efforts/cross-platform-updates/windows-final-output.txt` | `e27d98564d1270e5e8265c495aab0770a6b93cdbcc42e3c745f773670a38e794` |
| `efforts/cross-platform-updates/windows-harness-smoke-output.txt` | `e71abf8b5078d5424820c0af8ff82f572a946e5835dc2b6802952c8565c364be` |
| `efforts/cross-platform-updates/windows-phase1-evidence.json` | `1295bb691b2e96bfbf86ca433c9dea0f216783b4617eef1d656cd80f2c735473` |
| `efforts/cross-platform-updates/windows-phase1-results.md` | `fd7514b279e44e1a105993f280168d71723093d93838aeb0526d7f402ed372a3` |
| `efforts/cross-platform-updates/windows-policy-final-output.txt` | `7625efb284f2eda8eb7d1fc197f8b6a1bf2e422185683372006463255c95d3bc` |
| `efforts/cross-platform-updates/windows-provision-output.txt` | `f0c0c7aaabe5c20de2f726321f815d7f03699cdfbfaf14db13d6b30b0cdf2ae2` |
| `efforts/cross-platform-updates/windows-runner.md` | `0e70f28fbc75327389b5b3592104a9598fad6117f497611c55f80d954fd46982` |
| `efforts/cross-platform-updates/windows-smoke-evidence.json` | `4d5284446198aefcfd4f95cfccd765f8f6ef407ab9cb9ce32f7e62e902d2f830` |
| `efforts/cross-platform-updates/windows-transaction-audit.md` | `91dfeb2959e23d7782ed4a5869cbc3ee695007667e89a6638bb04616bfbd6ab9` |
| `efforts/cross-platform-updates/windows-transactions-output.txt` | `9cbd1d8310bb8a759c18e26f6889f68a0828bba60b7a87a40e4e1217889578a4` |
| `release/chocolatey/tests/architecture.ps1` | `b04c00f6bfb8a68a4a3517cf63115d3cab852d853ace03d6f53231ea746bc536` |
| `release/chocolatey/tests/assert_evidence.py` | `ef9d52a285744b03a5b6668b3eacb211a8e8f742f02dda1ce57497d3dce7a447` |
| `release/chocolatey/tests/barrier.ps1` | `ee158f18b195b855a204cfe0a353ed31b33d96d828aecd23b385c0ddb5929733` |
| `release/chocolatey/tests/collect.ps1` | `85293ba74b5ba65d7b119b600c588eef5f8fc0e94421c9ec36df18c7616798be` |
| `release/chocolatey/tests/extensions.ps1` | `29aa2334018b6d549c767fffb6161317194e62151a6d6b2bf73b1246db6fa89f` |
| `release/chocolatey/tests/final-inspect.ps1` | `1226751ff65dd1efdb398f8c7fda5b6e6ec1a1c59079ed19a3b15b956993f977` |
| `release/chocolatey/tests/guest.py` | `e4f40bfa41e3f29bf40607554352ea22fb2dce1f5625bc68758d2f35a073c443` |
| `release/chocolatey/tests/inspect.ps1` | `b8539c94ad026b879d0631e44175bbccf074dc001d0e70205079b1fd37243238` |
| `release/chocolatey/tests/provision.ps1` | `2441576bd9b9f5632801124234f6850a08d7c049d823605176d2415bfdd77ad8` |
| `release/chocolatey/tests/transactions.ps1` | `8054f84ebb6fc50beb9c1de10e19116651f33845c8d6d131942698cfc8b0cb6f` |
| `efforts/cross-platform-updates/windows-phase1-checkpoint.md` | self (not hashed) |
