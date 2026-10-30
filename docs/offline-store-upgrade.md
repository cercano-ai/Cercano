# One-time offline upgrade of existing conversation stores

Generation-aware ordinary launches refuse an unmarked existing-format store. They do not silently migrate it. Fresh prepared development stores already carry the compatible declaration.

## Required maintenance window

1. Stop **ALL legacy Cercano agents**, including background workers, historical binaries, custom launches and agents in other user accounts. Prevent supervisors from restarting them. Back up the stopped store using your normal offline backup procedure.
2. Using the compatible binary, run once:

   ```sh
   cercano upgrade-store --database /absolute/path/conversations.db --all-legacy-agents-stopped
   ```

   The flag is an operator assertion of the prerequisite, not a mechanism that stops agents. The command never creates a missing database.
3. Resume only compatible generation-aware ordinary launches. Never restart legacy writers against the upgraded store. Ordinary A/B publication and attachment thereafter need neither repeated migration nor manual retirement: existing pinned work stays on A, new sessions route to B, and drained A retires automatically.

The command has a bounded, three-second same-user `ps` check before opening the database. It refuses other processes whose reported executable basename is `cercano` or `cercano-agent`, conservatively including processes of unknown compatibility and processes using unrelated stores. Failure to obtain the process snapshot also refuses. The command excludes itself. This is **not universal writer fencing**: renamed/custom executables, other users, launches after the snapshot, and arbitrary direct historical SQLite handles are not detected. Stopping all legacy agents remains mandatory; the scan cannot prove that prerequisite.

## Persistence behavior

The command uses the existing conversation schema initialization and additive task initialization, under one SQLite `BEGIN IMMEDIATE` writer transaction. It rechecks the marker after admission and atomically publishes the compatible declaration, ownership tables and schema version. Errors roll back. History, turns and task statuses (including `running`) remain; no task reconciliation, replay, ownership reset or system import occurs. Existing historical schema migrations still apply as in ordinary initialization.

Repeated upgrade of an already compatible store is a no-op, including its ownership records. Unknown versions or incompatible declarations refuse rather than being overwritten. Compatible legacy `conversation.Open` refuses marked stores: callers must use the generation-aware opener and explicit ownership capabilities. This does not retrofit protection into historical/custom binaries.

## Disposable verification

Targeted tests: `go test ./internal/conversation -run TestOffline -count=1` and `go test ./cmd/cercano -run TestOfflineUpgrade -count=1` from `source/server`.

The compiled fixture harness adds `--offline-upgrade` to `efforts/dev-host-generations/everyday-launcher-compiled.py`. It seeds historical rows, invokes the actual command twice, and extends ordinary A/B checks with original-history resume and a fresh B reply. Run only in an environment satisfying the maintenance prerequisite; it intentionally does not bypass the production process detector. HOME/config/runtime/history and providers are disposable fixture state. The existing closure acceptance tests need not be rerun for this upgrade.
