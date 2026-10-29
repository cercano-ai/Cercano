# Normal-HOME first-prompt acceptance: stopped before launch

## Concrete blocker

The narrow prepared-worker audit found a reachable ordinary-HOME write before any model tool request:

- `source/server/internal/worker/spawn.go:373-380` launches `worker --socket ... --prepared` with inherited host environment.
- `source/server/cmd/cercano/main.go:2186-2220` selects `worker.NewPrepared()` for that entry; it bypasses ordinary agent startup.
- `source/server/internal/worker/worker.go:303` calls `w.buildDeps` for the turn. At line 562, that function unconditionally calls `failurelog.NewWriter("")`, including prepared workers and no-tool prompts.
- `source/server/internal/failurelog/failurelog.go:22-38` resolves the empty path using `os.UserHomeDir()` to `~/.config/cercano/failures.jsonl`, creates its parent directories, and opens it with create/append/write flags. There is no XDG or environment override in this resolver.

The host's explicit private `FailurePath` (`development_host_production.go:286`) does not redirect this independent worker writer. Private TMP/XDG/cwd/search/anomaly paths therefore cannot qualify the required normal-HOME run. A no-tool instruction does not prevent this dependency-construction write. No permission expansion, source edit, rebuild, or workaround was attempted. Next blocker: an authorized explicit private failure-log authority for prepared workers, implemented and present in an approved generation, would be needed before reconsidering launch. This observation does not qualify the remaining startup graph.

## Evidence and scope

Read AGENTS.md, real-cloud-observation-2026-10-29.md and normal-home-isolation-fix-2026-10-29.md. Inspected only the named startup/worker/config/credential/resource paths and the directly reached failure-log resolver; stopped at the concrete blocker rather than expanding the audit.

Read the existing sanitized credential-only record at `/private/tmp/cercano-interactive-generation-zvdnzsvg/retry-private-rw4ie7or/result.json`: returned, exit 0, OSStatus 0, 4.507 seconds, build `c4a75604-2310-4a22-9c2f-2c5f95e80834`. This is not inference success. The selected generation manifest reports the expected build and server digest `33e6e3f2894841b51376a80370f6c9800a9aa193af8095475ae7a29a84706076`; no new executable or credential query was run, and no fresh binary verification/launch qualification is claimed.

The read-only token source guards mutation before calling the refresh issuer (`internal/chatgptauth/source.go:96-102`), and production development composition guards the store read-only (`development_host_production.go:146-153`). These do not repair the worker filesystem escape.

## Actual execution result

No prepare-development, whole-app launch, PTY, cloud request, credential probe/login/refresh, build, install, runtime operation or process cleanup was performed in this continuation. New acceptance attempts: **0**; no response marker or reported inference usage exists. No fresh timed ledger was needed or created; previous ledgers and artifacts were not modified. This is not a claim of zero project-wide traffic or billing. No processes were created to clean up. The unrelated modified plan.md is preserved and excluded from this document-only checkpoint.
