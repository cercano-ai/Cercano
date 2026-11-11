# Bounded fresh-file toolchain hashing

## Scope and contract

Branch `perf/publisher-file-workers`, isolated worktree `.worktrees/publisher-file-workers`, based directly on `bc6e93daa6fd9d0d76752f7539c23fbb08fa3120`. The existing `.worktrees/publisher-three-workers` candidate is the baseline, not the output worktree. No landing or push.

Only toolchain tree hashing and its tests change. Each effective-identity collection owns one three-worker executor shared by enumeration and file batches; there are no nested tree pools. Initial, snapshot and final collections remain separate fresh observations. Every logical alias is read independently, including aliases resolving to the same file. No digest cache, metadata trust or immutability assumption is introduced.

Digest assembly consumes FIFO batches and records in the original globally sorted `Path` order. Regular-file and symlink classification, root handling and encoded record bytes match the frozen legacy oracle. Read/traversal errors propagate; ordered result consumption selects errors deterministically rather than by completion race. Executor shutdown joins outstanding work before returning or propagating failure.

Resource bounds: three active workers; at most five fixed toolchain-root scan futures; at most six outstanding file-batch futures, each containing at most 64 logical paths. A 4 MiB aggregate size hint splits batches further, but is not a content-memory allocation or a hard maximum file size. Files still stream in 1 MiB reads; result records contain names and digests/link text, never file contents. Size stats are scheduling hints only. The existing globally sorted path inventory remains O(number of paths), as required for exact legacy ordering; this is **not** a constant-total-memory traversal. Completed root inventories are released before consuming the next root. Additional hashing queue/content memory is bounded independently of file sizes/tree size.

Source enumeration, snapshot copies, executable modes, environment, fingerprint format/scope, signing and publication code are unchanged. Snapshot equality still precedes either build; final collection still follows both builds, signatures/verifications and pair validation.

## Verification

Final isolated run used `env -i`, private temporary HOME/TMPDIR, inherited executable search PATH and `LC_ALL=en_US.UTF-8`. These are fixture suites: fake Go/signing executables and temporary repositories/configurations, not real publication/build/signing or agent execution. No live credentials/configurations, agents, VMs or downloads were used.

| Exact selected suite | Result |
| --- | --- |
| `python3 source/server/scripts/publish-dev-generation_test.py` | 46 passed, 30.964s |
| `bash source/server/scripts/cercano-launcher_test.sh` | 26 passed, 17.021s |
| `bash source/server/scripts/install-launcher_test.sh` | 5 passed, 0.186s |

`git diff --check` passed. Launcher fixture tests emitted two intentional cleanup-failure warnings; the encompassing private test directory was removed afterward.

Added coverage includes frozen legacy-oracle ordering (component order/Unicode), regular files/large streaming reads, logical symlink branches/aliases/nonregular entries, read/walk failures, FIFO error choice, shared cross-root worker budget, bounded queued work and result records, worker joins on success/failure, and fresh repeated reads despite equal size/restored mtime. The Darwin publication fixture changes real toolchain fixture content persistently before snapshot/final checks, checks refusal and preservation of previous generations, and asserts build/sign/verify/pair-validation temporal barriers. Darwin-specific coverage ran on this host; other platforms were not measured.

## Analytic expectation (recorded before measurement)

Source: `scratch/publisher-build-profile-bc6e93da-57tuipkh/REPORT.md` in the original repository. Baseline effective-toolchain wall time was 15.228s across three checks, or 5.076s/check; build-required publication was 26.700303s. Tree worker-seconds averaged `(5.454 + 13.599 + 14.978) / 3 = 11.344` per collection, yielding an ideal three-worker lower bound of `11.344 / 3 = 3.781s`, before enumeration/overhead. Baseline already parallelizes three trees: this change redistributes imbalance, not an assumed 3x acceleration. Expected collection time was approximately 4–6s. Amdahl estimate, not a publication measurement: `26.70 - 15.228 + 3*T_new` with all other costs unchanged.

## Final read-only measurement

`measurements.json` retains the final four observations, ordered baseline/candidate/candidate/baseline, each in a fresh Python subprocess. Baseline source was extracted with `git show bc6e93daa6fd9d0d76752f7539c23fbb08fa3120:source/server/scripts/publish-dev-generation.py`; its SHA-256 is `3597a7ab4720f10a5fb4a21f82a0582383c1647c5472b58bf8d0bafafdc90539`. Measured candidate publisher SHA-256: `83e9c671da184eec96d8b26de0eaa11075ccd97616404b9e7be108aaea0cbc10`.

The harness invokes only `build_environment()` and `effective_identity()`: actual installed Go/Xcode/SDK/compiler hashing and read-only tool version/environment queries. No builds, signing, credential discovery, cache flushing or downloads. GOPROXY=off; GOENV/GOWORK disabled and GOTOOLCHAIN=local through the unchanged publisher environment policy. Final run used `/opt/homebrew/bin/python3`, a private HOME/TMPDIR and sanitized PATH/environment; an earlier rerun attempt under a different interpreter failed closed on inherited `CPATH` and produced no timing comparison.

| Run | Baseline seconds | Candidate seconds | Baseline max RSS MiB | Candidate max RSS MiB |
| --- | ---: | ---: | ---: | ---: |
| First per mode | 5.572608 | 4.851709 | 218.25 | 213.17 |
| Second per mode | 4.563966 | 4.653896 | 215.80 | 211.92 |
| Mean | 5.068287 | 4.752802 | — | — |

All four normalized effective identity hashes equal `bca31109d0d75dd5fd49553b7c036fdc47949d3d8be29da963506d27793fef26`; all recorded compiler/tree identities also equal. Darwin process `ru_maxrss` includes interpreter/inventory allocations, not filesystem-cache memory. Final candidate mean is 0.3155s (6.2%) lower, but the last warm baseline is faster than either candidate. With only two observations per mode and uncontrolled OS cache/load, this is **not evidence of a reliable speedup or proof against a small warm-cache regression**. No end-to-end publication speedup was measured or claimed.

Earlier small-batch measurements regressed: eight-path sequential-scan baseline/candidate means 4.796/4.998s; parallel-scan means 4.748/4.899s. Investigation targeted task/queue overhead and imbalance: batches now permit 64 small files but split at a 4 MiB size hint so large files/aliases get independent reads; enumeration shares the worker budget, and completed inventories are released. Intermediate 64-path results averaged 5.117/4.972s; before final inventory release 5.023/4.696s. These are exploratory, differently configured noisy measurements, not pooled into the final comparison. Historical exploration exceeded the protocol's 3–5 initial-verification-run recommendation (22 retained observations across configurations); this is a process limitation, not statistical evidence. No further parameter sweep was run. Final results remain in the analytic 4–6s range. Independent review should treat the functional resource-bound improvement separately from uncertain wall-time benefit.

## Reproduction (read-only hashing only)

Run from this isolated worktree on Darwin with installed local Go/Xcode, not from a live application environment:

```sh
git show bc6e93daa6fd9d0d76752f7539c23fbb08fa3120:source/server/scripts/publish-dev-generation.py > efforts/publisher-file-hashing/baseline.py
state=$(mktemp -d /tmp/publisher-file-hash-bench.XXXXXX)
mkdir -p "$state/home" "$state/tmp"
env -i PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin \
  HOME="$state/home" TMPDIR="$state/tmp/" LC_ALL=en_US.UTF-8 \
  /opt/homebrew/bin/python3 efforts/publisher-file-hashing/benchmark.py
rm -rf "$state"
rm efforts/publisher-file-hashing/baseline.py
```

`benchmark.py` writes a fresh `results.json`; checked-in `measurements.json` remains the reported observation. Effective identity differs if toolchain/environment changes; the comparison requires equality within each run, not equality to this machine's historical hash. No production runtime, build, signing or download should be added to this benchmark.
