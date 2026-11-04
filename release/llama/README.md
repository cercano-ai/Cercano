# llama.cpp baseline lock (schema 1)

`baseline.lock.json` pins the configured local source baseline for the approved
internal agent runtime: revision `74976e1aefd8d64845ae5510674b36079c855c0c`.
It is **not** a reproducible build recipe, artifact attestation, or a claim that
current release archives contain llama.cpp. No runtime lookup or installer
behavior consumes this lock yet.

## Identity versus provenance

- `source.revision` is the full identity recorded in the installed Homebrew
  llama.cpp 9890 receipt's `source.scm_revision`. The parent inspection supplied
  `--version` = `9890 (74976e1ae)`, AppleClang 21, Darwin arm64. This slice
  independently read the receipt, not an upstream source checkout.
- `observed_build` describes that Homebrew bottle, not Cercano's future build.
  Its runtime dependencies are **receipt declarations**, not current opt-link
  resolution. In particular, OpenSSL was declared as 3.6.3 rebuild 1; the
  inspected opt link now resolves to 3.6.5. See effort verification for closure.
- `9890` is the observed package/build version, not an inferred upstream tag.
  Upstream tag and source archive digest remain null. There is no invented
  bottle or runtime artifact digest. A source commit alone does not identify
  compiler flags, patches, SDK, dependency bytes, backend resources or signing.
- `delivery` records intent (internal agent) separately from current delivery
  (not bundled, no qualified platforms, no recipe or artifact digest).

`baseline.schema.json` is JSON Schema 2020-12. The offline stdlib validator
implements only the keywords used here, rejecting unsupported keywords; it also
checks revision-prefix agreement and unique dependency names. Unknown/missing
fields, bool-as-integer, unsupported schema versions, duplicate JSON keys and
unearned qualification/digest/tag claims fail. Null and false fields are
intentional constraints of this **baseline-only schema**, not permanent limits
on future packaging: extend/version the contract with evidence when actual
build provenance and packaged runtimes exist.

Run from any directory:

```sh
python3 /path/to/repository/scripts/llama_baseline.py
python3 /path/to/repository/scripts/test-llama-baseline.py
```

The existing release workflow test gate validates this metadata before importing
signing credentials. It does not inspect a live installation, assert payload
membership, or bind a built artifact to this source. Standalone archive builders
and `runtime-inventory.json` remain unchanged. Later integration must verify the
actual staged llama identity and complete dependency/resource inventory before
claiming bundled delivery. Updating the source lock requires deliberate review;
validation checks consistency, not upstream authenticity.
