# Bounded shutdown caller obligations

2026-10-27 implementation input, not completed verification. User approved bounded incomplete shutdown and deferred disposal; no helper process or forced termination. Read-only inventory at a0c150db71eefbecbb5221122fc32975d3297031 identified the following propagation points. Line numbers are historical as implementation proceeds.

- main.go startup wrappers (~193–220) return func() cleanup; carry explicit outcomes through all wrappers.
- Standalone agent (~2038,2072–2076) unconditionally prints Shutdown complete; must report incomplete and preserve finalization lifetime. Existing second-signal force-exit is not safe-retirement evidence and must not be used to satisfy this contract.
- Embedded MCP (~2108) discards owned cleanup. External-gRPC MCP does not own server teardown.
- main.go startup rollback (~527–532) ignores Server.Shutdown outcome; retain cleanup handle alongside startup error.
- development_host.go (~161–167,191–195) and dev_host.go (~250–251,280–284) defer shutdown/dependency cleanup and return nil on cancellation; audit against incomplete contract.
- development_host_production.go (~169–175,231) dependency close discards store errors and CloseParticipation always returns nil; do not erase incomplete status in retirement.
- server.go Shutdown (~587–611), task-manager/completion drain (~665–697), runtime teardown (~744–770) currently log or discard failures. Readiness is not cancellation.
- coexistence.go RetireCoexistence (~369–385) calls void task-manager close; lifecycle_controls.go (~261–267) reports Ready when no error. Timeout alone cannot report readiness.
- ShutdownAgent/restart request is acceptance, not completion. agentclient/client.go (~1966–1978) only checks Accepted. UI model.go (~3481–3504) and open_runtime_modal.go (~644–675) discard request errors; status/refusal must not masquerade as success.
- Homebrew coordinator waits separately for process disappearance; do not redefine this as proof resources/children drained.

Canonical activation remains disabled. No broad claim of completed caller integration follows from this inventory. Tests must cover admission closed while status remains observable and eventual cleanup exactly once; preserve retained authority until genuine completion without global sibling fencing.

Parent tooling check: `go version` succeeded (go1.26.3 darwin/arm64). Child command failures, if encountered, require parent verification rather than claims of impossible implementation.
