# Independent-process snapshot coordination evidence

2026-10-28, darwin. Only new test source: `source/server/pkg/config/snapshot_process_test.go`; no production changes made by this task.

Executed from `source/server` (offline):

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local CERCANO_DISABLE_CREDENTIALS=1 \
  go test -race -count=3 -run '^TestSnapshotProcess(Helper|Coordination)$' \
  -timeout=120s -v ./pkg/config
```

Result: PASS, `ok cercano/source/server/pkg/config 9.284s`.
All six contested phases (absent and existing, three repetitions each) reported exactly one winning publication and one `errors.Is(ErrConfigConflict)` outcome. Each winner subsequently saved using its returned baseline; the original copied draft and previous winning baseline remained rejected, with final winner bytes intact.

Each phase launches two copies of the test binary with isolated HOME/XDG/TMPDIR, explicit credential disablement, and no inherited provider keys. Both report their loaded baseline over pipes before publication is released. Before contention, the parent holds the authority's file lock and both independent children must receive `context.DeadlineExceeded` attempting coordinator Save; this rules out process-local mutex-only protection. The parent verifies neither blocked probe changed disk. No assertion sleeps, native credential calls, production application, provider startup, or network calls are used. Deadlines bound pipe waits, saves, and child lifetimes; cleanup targets only owned exec.Cmd children.

The standalone helper test intentionally skips outside subprocess mode. Existing unrelated working-tree edits were not staged.
