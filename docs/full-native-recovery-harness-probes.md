# Full native recovery acceptance: baseline probes

Status: investigation only. No integrated acceptance test has been implemented;
no new coverage or CI qualification is claimed by this note.

## Local baseline, 2026-11-13

Isolated worktree `wt-full-recovery-acceptance`, branch
`test/full-native-recovery-acceptance`, created from local `main`.
The commands below passed on Darwin using `/opt/homebrew/bin/go`:

From `source/server`:

```sh
/opt/homebrew/bin/go test ./internal/brewrestart -run 'Test(SignalIdentity|PreparedGenerationInventory|PreparedAbsenceClassification)' -count=1 -timeout=120s
/opt/homebrew/bin/go test ./cmd/cercano -run '^TestDevelopmentResumeTimeoutConfirmedContinuation$' -count=1 -timeout=120s
/opt/homebrew/bin/go test -c -o /tmp/cercano-full-recovery-baseline-server.test ./cmd/cercano
```

From `source/clients/cli`:

```sh
CERCANO_RESUME_PROBE_BINARY=/tmp/cercano-full-recovery-baseline-server.test /opt/homebrew/bin/go test ./internal/ui -run '^(TestDestinationResumeAfterActualProcessExit|TestReopenProductionReleasedAndDisconnected)$' -count=1 -timeout=120s
```

These are existing component/bridge tests, not the requested new journey. No
race repetition, full server suite, production build, independent review or
hosted CI execution was performed in this investigation.

## Concrete integration gaps

- `source/clients/cli/internal/ui/destination_process_test.go` invokes
  `reopenDevHost` only after the source has exited. Its bridge's source uses
  `os.Exit(0)` and its recovery guard substitutes the verifier with the exact
  child's `ProcessState.Exited`. It does not reach native containment or consent.
- `source/clients/cli/internal/ui/dev_host_reopen_production_test.go` uses
  production source startup but lifecycle-only destination construction. Its
  own header explicitly excludes destination UI/history hydration.
- `source/server/cmd/cercano/development_resume_observation_test.go` has private
  records and token-authenticated identity but substitutes inspection, stop,
  absence and destination selection, and injects an immediate observation
  deadline. Its success is not native termination evidence.
- `development_containment.go` requires the native source executable to equal
  `<recorded-generation>/cercano`; the registry must also include the matching
  `.starting` intent and launch lock. A generic test executable at an arbitrary
  path cannot simply be plugged into this adapter.
- `internal/brewrestart/legacy_darwin.go` additionally requires a same-user,
  non-self process owning the exact loopback listener. `verifyLegacyHostArg`
  requires exactly one matching `--host` argument and revalidates incarnation.
  A test child must meet these production requirements, not replace them.
- `VerifyPreparedWritersAbsent` excludes all remaining processes executing that
  generation's canonical executable. Source and destination therefore need
  distinct canonical generation directories; keeping the test coordinator in
  the source generation would correctly prevent recovery.
- `contained_recovery.go` already implements actual CLI confirmation including
  default Enter/No refusal, pinned destination validation, authenticated resume,
  fresh-epoch response validation and authenticated session dialing. An
  integrated UI test must drive this handler and the resulting hydration path,
  not set the wire confirmation flag directly and label that CLI consent.
- `.github/workflows/ci.yml` currently labels `recovery-darwin-components`
  accurately as components only. It must not be relabeled full acceptance until
  the missing journey exists and is run by that job.

The existing tagged terminal harness (`dev_host_terminal_test.go`) demonstrates
private seeded stores, explicit synthetic modelhost infrastructure and standalone
CLI processes. It is a useful production-host construction reference, not proof
that recovery is already integrated. No change to production identity authority
or native containment is justified by the probes above.
