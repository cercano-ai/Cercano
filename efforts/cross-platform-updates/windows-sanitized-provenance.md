# Sanitized Windows derivative provenance

## Scope and lineage

This branch, `handoff/windows-replacement-sanitized`, starts directly at shared base `a9ca2fb82831d707863b132ed650e591cded19cf`. Its 96 net Windows paths were copied from local-only feature tip `c9ee1b48104945d350891d16528eeff1d1987c18`, then sanitized, whitespace-normalized and supplemented with this provenance and a derivative checksum manifest. It does not merge or cherry-pick the original private feature commits. No product behavior was changed; the retained scripts are historical test/recovery harnesses, not deployment approval or instructions to execute blocked work.

The original feature ref, raw tracked captures and three untracked diagnosis/race artifacts remain local, untouched and unpushed. The derivative deliberately excludes those untracked artifacts, local blocker/state files, private substitution maps and preparation scripts. No binary/archive payload, credential material, VM image or configuration snapshot is intentionally added.

Privacy review covers **all new outgoing blobs and commit messages** in this branch. Already-published shared ancestry (including preexisting identifiers outside these outgoing versions) and normal repository author attribution are expressly out of scope. Shared history is not rewritten. This is not a claim that the entire repository history is identifier-free.

## Redaction and integrity contract

Consistent placeholders replace discovered host home/worktree/temp context, guest host/account identifiers, VM UUIDs and the blocked operational task identifier. Examples are `/SANITIZED_WORKTREE`, `/SANITIZED_HOST_TEMP`, `SANITIZED_GUEST_HOST`, `SANITIZED_ACCOUNT`, `{SANITIZED_VM_1}`, `{SANITIZED_VM_2}` and `SANITIZED_BLOCKED_TASK`. These are deliberately non-operational. No reverse/private substitution map is published. Generic Windows system accounts and non-personal fixture paths remain technical context.

These files are **derivatives, not byte-exact raw evidence**. Text newline/trailing-whitespace normalization and redaction can change serialized lengths, paths, scripts and transcripts. Historical capture fields such as `sha256`, `length`, `size`, source/script digests, transport byte counts and hash inventories embedded in JSON/text describe the original observed artifacts, not the redacted serialized content. Do not use them to authenticate modified bytes or infer that redacted harnesses are runnable without explicit authorization and requalification. Historical stdout and transcripts are sanitized excerpts/copies, not exact original bytes, even where older narrative calls them full or raw.

Non-sensitive fixture executable/resource/build hashes and technical results are retained. The fixture source identity offline assertion still passes. Two explicit derivative integrity mechanisms supersede historical repository-byte claims:

- `native-go-result.json` was regenerated only by the inspected offline `native-go-audit.py`; its `evidenceIntegrity` map hashes the four current sanitized input files. This does not claim original evidence identity.
- The table in `windows-phase1-checkpoint.md` is recalculated for current derivative bytes. `windows-sanitized-checksums.json` hashes every other outgoing file, excluding itself to avoid recursion. It includes this provenance and the handoff.

## Checks and technical limits

- `assert_evidence.py`: all 19 stored synthetic cases pass. This reads repository captures only, not Windows processes.
- `native-go-audit.py`: six bounded stored checks pass; overall remains `BLOCKED_INCOMPLETE_NOT_ACCEPTANCE_PASS`, with eight `NOT_ESTABLISHED` checks and the historical incomplete-transcript `FAIL`. Recovery and corrected-run reports remain separate later observations, not a retroactive pass of the original run.
- All outgoing files are reviewed as UTF-8 text, with no NULs, executable/archive additions or detected secret material; known private identifiers and broader UUID/SID/personal-path/credential markers are checked across complete outgoing blobs, not just added lines. JSON parsing, Python AST parsing, checksum consistency and staged whitespace checks apply.
- Ancestry checks require this shared base to be the direct parent and all feature-only commits to be absent from the new ancestry. Publication is a normal push of this branch only; upstream/remote SHA and zero ahead/behind are checked afterward.

No VM/guest/process/configuration access, guest script execution, new experiment, product build, live acceptance, cleanup or provider-blocked concurrent probe retry/rerouting was performed. Acceptance, account/permission and race boundaries, actual split product packaging and native amd64 remain outstanding as described in `windows-phase1-handoff.md`. Publishing this branch is not merge, release, package publication or deployment authorization.
