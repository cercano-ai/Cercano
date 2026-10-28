# Shared service registration evidence (2026-10-27)

Scope: test-only repair in `source/server/cmd/cercano/production_composition_test.go`.
No production credential boundary changes or credential enabling.

## Probe before repair

With `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`, from
`source/server`:

```
go test ./cmd/cercano -run '^TestProductionCompositionServiceParity$' -count=1
```

Failed at the former line 158: `open read-only credentials: credentials disabled
by CERCANO_DISABLE_CREDENTIALS`. The production opener correctly refuses before
calling the injected credential factory; the former test's successful-opener
assumption was stale.

## Replacement evidence

- `TestProductionCompositionSharedServiceRegistrationParity` constructs the
  prepared graph through `server.NewPreparedServer` using a temporary prepared
  conversation store, explicit memory config/secrets and synthetic routing.
  Both graphs call the shared `composeProductionServices`. It checks nonempty
  identical listed tool names and preservation/isolation of caller stores.
  Prepared MCP factory calls remain zero.
- `TestProductionCompositionDisabledAdmissionRefusal` calls the actual production
  prepared opener with credentials disabled and requires wrapped
  `secrets.ErrDisabled`, no server/cleanup, and zero credential, runtime,
  provider and MCP factory calls.
- Existing policy-before-assembly and credential-opener zero-count assertions
  remain intact.

This is lower-level shared service registration evidence, **not** full production
opener, provider, worker execution, lifecycle, transport or runtime parity. No
native credentials, real provider/network calls or child processes were used.
The fixture writes a dummy generation executable but does not execute it.

## Focused verification

Both commands passed offline with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off
GOSUMDB=off`:

```
go test ./cmd/cercano -run '^TestProductionComposition' -count=1
go test -race ./cmd/cercano -run '^TestProductionComposition' -count=1
```

The first race attempt encountered a concurrent unrelated build edit:
`vet: cmd/cercano/main.go:492:4: undefined: cancelProducers`. A subsequent identical
race command passed (package reported 1.765s). No changes to that file were made
for this repair. No full-suite claim is made.
