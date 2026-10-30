# Temporary automatic-upgrade safety containment

Status: **safe refusal, incomplete implementation; not completion of the automatic-upgrade requirement.** No migration success acceptance is claimed.

Known reproduction (reported and confirmed before this containment; not rerun here): native automatic upgrade shutdown can return success while a late-spawned, differently named child remains alive. A process snapshot and known-name detector cannot prove that worker spawning has stopped or all workers have drained. There is currently no qualified worker-spawn/drain shutdown barrier on any supported host.

Containment keeps the exact y/Y confirmation and compatible unattended path. After affirmative consent the launcher refuses before helper build, shutdown, backup, permission changes, or migration. The native `--stop-hosts-config` path independently refuses before config loading, shutdown, discovery, or database access. Both say: `host lacks verified worker-spawn/drain shutdown control; upgrade not performed`. No STOPPED/manual prerequisite or detector bypass was added. Existing offline primitives remain lower-level diagnostics, not authorization for automatic migration. The native shutdown test is explicitly labeled diagnostic, not safety acceptance.

Focused validation performed for this containment (disposable fixtures only; no live-state discovery):

```sh
# source/server; GOPROXY=off GOSUMDB=off CERCANO_DISABLE_CREDENTIALS=1
python3 scripts/test-ordinary-upgrade.py
python3 scripts/test-launcher-safety.py
go test ./cmd/cercano -run 'Test(AutomaticUpgrade|OfflineUpgrade)' -count=1 -timeout=60s
```

Results: 12 ordinary containment tests PASS; 20 launcher safety tests PASS (expected cleanup-warning fixtures); focused Go tests PASS. Combined command bounded to 180 seconds, completed in 21 seconds. Tests assert affirmative refusal with bytes/modes unchanged, no backup/helper/startup, and native refusal before shutdown/discovery with a missing config path. Lower-level native shutdown diagnostic was not rerun. No push, land, install, or live discovery performed.

## Historical validation — superseded, not migration safety acceptance

The following records predate containment. Prior migration success observations do **not** establish safe automatic shutdown and are not current acceptance claims. Supported-success assertions have been replaced by refusal assertions.


All executions used disposable synthetic stores; migration discovery was the explicit acceptance-tag injection. Production discovery is unchanged. No native inference claim.

Baseline reproduction: imported `scripts/test-ordinary-upgrade.py`, called `OrdinaryUpgrade.setUp()`, replaced only its disposable copied launcher with `git show 672c088a:source/server/scripts/cercano-launcher.py`, moved authority into an owned 0755 `nested` directory, and launched with STOPPED. Result: exit 78, retained backup and completed migration, followed by `directory must be private`. The new nested-authority regression exits 0 without manual chmod.

Commands from `source/server`:

```sh
gofmt -w cmd/cercano/upgrade_discovery_acceptance.go pkg/agentclient/launcher_python_lock_test.go
d=$(mktemp -d); trap 'rm -rf "$d"' EXIT
go build -tags ordinary_upgrade_acceptance -o "$d/cercano" ./cmd/cercano/
CERCANO_TEST_UPGRADE_BINARY="$d/cercano" python3 scripts/test-ordinary-upgrade.py
python3 scripts/test-launcher-safety.py
go test ./pkg/agentclient -run TestPythonLauncherSDKLockThroughReadiness -count=1
go test ./cmd/cercano -run 'Test.*(Upgrade|Ordinary)' -count=1
go test ./internal/conversation -run 'Test.*(Upgrade|Offline)' -count=1
```

Results: ordinary upgrade 11 tests PASS; launcher safety 20 tests PASS; all three focused Go commands PASS. Initial launcher-safety runs failed because synthetic markers omitted singleton and obsolete assertions expected an upgrade offer for unsupported stores; fixtures/assertions corrected and rerun. Expected cleanup-warning fixtures still print warnings.

SDK interoperability uses the real SDK lock against Python `main`, in both directions. Startup is injected: SDK exclusion is asserted before and after a synthetic readiness boundary, and release afterward. This is not a real host-readiness integration test.

From repository root:

```sh
d=$(mktemp -d)
TMPDIR="$d" CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off python3 efforts/dev-host-generations/everyday-launcher-compiled.py --repo "$PWD" --evidence "$d/evidence"
echo "EVIDENCE=$d/evidence"
```

Result: PASS bounded compiled A/B rehearsal through validated launcher startup (115.887 seconds). Evidence: `/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/tmp.RfqaN1gGFt/evidence`. Disposable compiled copy: `/private/tmp/ca-84g8jxa8`. Observed A survives B build/startup failures and publication; new sessions choose B; newly born A-pinned worker runs after B; history resumes on B; A retires after transfer/release. Compatible stores complete unattended startup, consistent with no upgrade confirmation. Synthetic providers and memory credentials only; bounded fixture builds, not native inference or full production parity. No offline-upgrade harness flag was used: this run exercises compatible stores, while the separate ordinary-upgrade suite covers migration with injected discovery.
