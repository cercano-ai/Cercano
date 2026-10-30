# Ordinary generation startup: bounded verification

The validated generation launcher now calls the ordinary startup pipeline. That pipeline attaches the generation lifecycle using its exact store and installs the captured worker spawn policy without constructing a prepared server. Ordinary configuration, credentials, runtime initialization, watchdog, and ownership-aware scheduling remain composed. Prepared-only service restrictions remain for prepared construction, not ordinary attachment. Ownership admission covers ordinary service mutations, profile changes, rollover, and manual context regeneration.

Retention behavior was not changed: a shared-database sweep alone was not treated as evidence requiring a new ownership mechanism. No live database adoption, installation replacement, automatic-exit policy change, or handover rewrite was performed.

## Independently executed verification

All execution used disposable HOME/config/runtime state and synthetic inference/memory credential backends where needed.

- `go -C source/server test ./internal/server ./internal/conversation ./internal/broker ./internal/hostsvc/persistence -count=1`: PASS.
- `go -C source/server test ./cmd/cercano ./internal/worker -run 'Test(OrdinaryGeneration|ProductionRuntime|DevHostCompiledExecution|Prepared|WorkerPostTurn)' -count=1`: PASS.
- `python3 efforts/dev-host-generations/everyday-launcher-compiled.py --repo <integration-checkout> --evidence /tmp/cercano-ordinary-parent-final-ab-offline`: PASS (65.6 seconds including dependency preparation).

The final compiled rehearsal ran with GOPROXY/GOSUMDB disabled, after public CLI dependencies were populated into the isolated build cache. An earlier attempt failed before A launched because those dependencies were absent; a direct online retry was refused by the harness's offline guard. Neither failed attempt is acceptance evidence.

The final rehearsal exercised shell launcher, publisher, compiled host through ordinary service composition, compiled terminal client, and actual workers. It recorded B publication without stopping A, a newly born A worker handling fresh A foreground work after B became available, ordinary settings mutation/restoration, busy reopen preserving independent work, explicit reopen on B, explicit A retirement, and watchdog enablement with offline audit on both hosts. The evidence JSON contains process identities and executable/manifest hashes.

## Limits

This establishes bounded ordinary-entrypoint executable pinning and focused regression coverage, not full production parity. The store was freshly prepared compatible disposable state; adoption of an ordinary legacy database was not exercised or authorized. Native credentials, real provider inference, loaded model runtimes, and live cutover were not tested. Unbound canonical generation activation remains refused; activation requires the validated launch identity and compatibility admission. Explicit retirement in the rehearsal does not claim implementation of automatic old-generation exit. Evidence remains outside Git under `/tmp/cercano-ordinary-parent-final-ab-offline`.
