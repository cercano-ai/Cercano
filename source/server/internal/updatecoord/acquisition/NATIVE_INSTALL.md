# Native temporary-install foundation

Run from `source/server` on native macOS arm64:

```sh
CERCANO_NATIVE_INSTALL_TEST=1 go test ./internal/updatecoord/acquisition -run '^TestNativeTemporaryInstall$' -count=1 -v -timeout=15m
```

This opt-in integration test builds the existing agent and CLI packages and the
bundled dedicated updater twice, stamping `0.0.0-fixture.1` and
`0.0.0-fixture.2`. These are **current-source fixture releases**, not published
versions, historical artifacts, or compatibility evidence. Builds use the local
Go toolchain/cache; application execution receives no inherited environment.

The owned temporary root contains build outputs, isolated HOME/config/cache/tmp,
a private `installation/versions` staging parent, TUF caches and downloaded
archives. The existing acquisition fixture generates fresh Ed25519 role keys
and serves a loopback TLS feed. The client trusts that test TLS certificate and
refuses redirects. Both three-binary ZIPs traverse authenticated acquisition,
receipt-bound archive validation and staging. The complete-release caller
requires exactly agent, CLI and updater, without changing generic two-member
layouts. SHA-256 comparisons bind staged executables to the builds; Mach-O
inspection checks native arm64 and exact version output checks the staged images.

Only offline version queries and deliberately refused updater `execute` calls
run. `sandbox-exec` denies network, securityd lookup and host login-keychain file
reads, with isolated HOME/config and no inherited installation locators or
credentials. An unavailable sandbox fails the test, never falls back to normal
execution. No live agent or ordinary startup is attempted. The two version
directories remain present throughout the test; Go removes the entire disposable
fixture after completion. This is logical staged-release retention, not an
installer's finalized immutable-directory/protected-selection contract.

## Next integration seam (not implemented)

Extend this same test after staging: provision protected installation identity,
selection and stable entrypoints, and decouple both launcher and child startup
from updater records before any normal launch. Then independently launch the
exact retained old `bin/cercano-updater` using `updatecoord/launch` and introduce
the bounded owner-only handoff input, updater-owned exclusion and native evidence.
The current updater accepts only exact `version` and `execute`; `execute` always
fails closed without reading inputs or touching installation state. Do not treat
its presence or version output as update readiness.

No drain/publication, normal interaction, installed boot independence, downgrade
policy integration, capability producer/consumer reconciliation, migration safety,
signing/notarization or production feed changes are claimed here. Existing
acquisition/archive rejection suites remain relevant but are not end-to-end
cutover acceptance. Do not enable normal startup merely by removing the sandbox.
