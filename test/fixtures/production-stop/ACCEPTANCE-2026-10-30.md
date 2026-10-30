# Bounded ordinary-launcher/native-stop acceptance

Test-only integration connects the ordinary launcher acceptance stop hook to the existing fixture-scoped native stop implementation. No production behavior change is intended.

Expected combined order/counts: confirmation, one check-only shutdown helper call, verified owned writer exits, one backup, one migration helper call, supported generation startup/history checks. Compatible A/B expects zero upgrade-store calls and zero upgrade backups. Assertions are in `generation.py:50-78` and `efforts/dev-host-generations/everyday-launcher-compiled.py:593-597`.

## Verification in the final continuation

`python3 source/server/scripts/test-ordinary-upgrade.py`: exit 0, 13 tests in 9.791s, OK. Python emitted unclosed SQLite ResourceWarnings. `git diff --check`: exit 0.

The supplied execution history reports successful combined native-stop/generation and compiled A/B runs, retained failure rehearsals, and empty owned-process cleanup reports. Those executions were not repeated in this final continuation. The compatible evidence inspected during this continuation reports `passed: true`, zero shutdown/migration calls and an empty upgrade call list; source checkout `e37ffbad20f7e46a815227bb088deba01de85ab3`, snapshot `f7ddc336c984c2f8de465f6dbd2b13a6c81b7780`, source tree `ec79e30021ec0b908d43ee5a47e31eb51e9592dc`, fixture SHA256 `2c3d86ee877012318241ccc47747e86eb3c5abaae070d14ec1fe7178cae23de4`.

Observed local binary SHA256: production fixture host `5b98c29b3c0e6c2d7a8c1098a14f8eb2c3efa9f82ca30330ce465d3aae59b655`; native helper `3b985e188912ea210f3088e16b84d779a47aef68490dbb09f1cab42a56fb7bfe`. These hashes alone do not establish build provenance.

## Scope and limitations

The existing fixtures substitute synthetic providers, credentials and disposable configuration/SQLite state. Discovery is fixture-scoped rather than ambient. Native shutdown is intended to target the explicitly owned disposable host, not global inventory. Exited zombies are distinguished from live writers; shared model-host cleanup is separate from the writer fence. Historical reports of host exit -9 must not be described as graceful shutdown.

This evidence is not live configuration/DB migration, ambient discovery, external-provider acceptance or permission to activate/install/push. No live inventory preflight was repeated. Full combined acceptance and cleanup results in the supplied history should be treated as historical evidence, not fresh execution claims in this continuation.
