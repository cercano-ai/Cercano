# Fixed coexistence mutation freeze: required caller wiring

These APIs are opt-in; cmd/server wiring is deliberately outside this change.
Capture `secrets.Options{ReadOnly: coexistence}` once from the validated startup
mode, never from active connections. This is NOT CERCANO_DISABLE_CREDENTIALS:
that existing strict 0/1 facility still rejects reads as well as writes.

Required wiring (before reconciliation or any interactive/network effects):

* Load startup config with `config.LoadWithOptions(path, options)`. For an
  injected snapshot use `cfg.WithSharedReadOnly()` in coexistence before any
  `config.Save`. Clone retains the freeze; YAML does not serialize it. Watcher
  reloads and replacement snapshots must receive the same startup option.
  Raw Save returns `secrets.ErrReadOnly` before validation, mkdir or writes.
* Open with `secrets.OpenKeychainWithOptions(options)`, or wrap injected stores
  with `secrets.Guard(store, options)`; pass options to `credentials.New(store,
  options)` as well. Service options survive ReplaceStore. Never retain raw
  backend aliases for edits. Get/List and fresh subscription tokens still work.
* Direct legacy `anthropicauth.NewSource` / `chatgptauth.NewSource` accept the
  same optional final options value. Guarded stores are also recognized before
  refresh. Expired tokens fail with ErrReadOnly before OAuth transport calls.
* Set `Flow.Options` on directly constructed auth flows; Start rejects before
  listener/device authorization effects and Refresh rejects before transport.
  Service login handlers must call BeginLogin BEFORE browser/listener/provider
  work. Direct auth Save requires a guarded store.
* Keep the existing cfgSvc fixed-mode gate for profile edits, logout and reset.
  CLI/offline reset/login/logout paths that bypass the service or raw Save must
  explicitly return ErrReadOnly at admission, before opening stores, starting
  OAuth, contacting an agent or rewriting YAML. ResetSetupYAML is only a pure
  transformation, not a persistence gate. If a legacy path cannot receive the
  fixed options, reject the entire operation in coexistence; do not silently
  skip persistence after performing provider-side mutation.

No mode is inferred from environment or mutable connection counts by these
APIs. Ordinary zero-option calls retain their previous behavior and file modes.
Do not treat ErrReadOnly as a successful edit or retry it through a raw backend.
Unwired callers remain ordinary-mode callers; this library change alone does
not claim that executable admission is complete.
