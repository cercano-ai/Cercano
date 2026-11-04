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

## Opt-in offline Darwin staged loader check

This is a separate command, **not** a mandatory gate for current release archives
(which contain no llama payload):

```sh
python3 -B scripts/darwin_loader_closure.py \
  --staged-root /path/to/private-stage --loader-reports /path/to/reports.json
python3 -B scripts/test-darwin-loader-closure.py
```

Proposed layout: `bin/llama-server`, `lib/<libraries>`,
`libexec/<selected ggml backend modules>`. The caller supplies JSON with exactly:

- `executable`: stage-relative executable path;
- `backends`: **nonempty explicit list** of stage-relative backend root paths;
- `reports`: object mapping every closure image's stage-relative path to its
  complete, single-image thin `otool -l` text. Do not supply `otool -L`: install
  IDs are not dependency edges. Report headers are informational, not identity.

The command reads reports and checks existing staged regular-file targets; it
never invokes otool, executes staged code, copies or rewrites binaries. The
caller must produce fresh reports from the actual staged images after relocation
and bind evidence to their bytes separately. Reports alone cannot prove Mach-O
identity, architecture, provenance, freshness or completeness. The fixture JSON
under `fixtures/` illustrates the format but is **synthesized text**, not raw
native output or evidence of relocated bytes. Its dependency spellings model the
previously recorded Homebrew graph (including opt paths and the OpenSSL 3.6.5
Cellar edge); staged names/IDs and other load-command fields are illustrative.

Resolution supports `@loader_path`, `@executable_path`, and `@rpath` using local
then inherited loader-chain runpaths. Backend roots use executable runpaths as
initial context. More than one distinct existing rpath target fails rather than
assuming dyld search order proves safe packaging. Plain relative paths, unknown
tokens, external absolute paths, missing images/reports, external runpaths (even
unused), lexical/root or symlink escapes and unaccounted report images fail.
Only exact OS library/framework paths in the reviewed `SYSTEM` constant are
allowlisted; arbitrary `/usr/lib` or framework-prefix matches are not allowed.
Weak/lazy/reexport/upward loads are required too (conservative packaging policy).
Install IDs are parsed but not treated as load edges or qualified for relocation.
Safe internal aliases resolve to one physical image; this does **not** change the
archive inventory's existing symlink prohibition. Reports must key each physical
image only once. Only a thin image is supported; universal binaries require a
separately reviewed per-slice contract. Environment injection and unsupported
loader commands fail; this is not a complete dyld emulator.

### Caller inventory obligations

A caller must determine the backends actually required by the build and its
supported hardware. The observed candidates were BLAS, CPU apple_m1,
apple_m2_m3, apple_m4 and Metal. List each selected module explicitly even if no
static edge reaches it. Omitting one from both the stage and inventory cannot be
detected here: **dynamic backend discovery is not complete or verified**. Verify
discovery/search directories and CPU selection separately; the validator does
not scan libexec and infer requirements. Declare non-Mach-O resources in the
runtime inventory and establish their use independently: Metal resources,
OpenSSL providers/configuration/CA trust, licenses, and any further dlopen
modules. This loader report format does not validate those resources. A passing
check is only supplied-report closure, not a bundled-payload release gate,
runtime qualification, model support, or authorization to build/sign/install.

### Parser validation verification (2026-11-03)

Before the fix, running the two smallest new regressions with
`python3 -B scripts/test-darwin-loader-closure.py ClosureTests.test_unknown_command_rejected ClosureTests.test_missing_cmdsize_rejected`
exited 1: **2 tests, 2 failures**, both `Failure not raised`. The reports were
`file:\nLoad command 0\n cmd LC_BOGUS\n` and the same envelope with `LC_UUID`:
unknown commands and missing `cmdsize` incorrectly produced empty edges.

The parser now explicitly recognizes reviewed dependency/runpath and
nondependency commands, rejecting other names with an instruction to review
loader semantics. Each block requires exactly one well-formed `cmd` and decimal
`cmdsize` (at least the 8-byte load-command header, aligned to 4 bytes).
Existing required dependency name/runpath fields remain checked. This validates
the common command envelope, not every Mach-O command payload or binary layout;
the offline supplied-report contract and all limitations above remain unchanged.

Post-fix offline verification:

| Command (`python3 -B scripts/…`) | Tests | Result |
| --- | ---: | --- |
| `test-darwin-loader-closure.py` | 20 | PASS |
| `test-llama-baseline.py` | 6 | PASS |
| `test-runtime-inventory.py` | 3 | PASS |
| `test-macos-release-verify.py` | 23 | PASS |
| `test-release-workflow.py` | 47 | PASS |

**99 tests passed**, no skips. Loader regressions cover a valid `LC_UUID` with
`cmdsize`, unknown commands with/without `cmdsize`, missing sizes (including
otherwise populated dependency/runpath commands), duplicate/malformed commands
and sizes, invalid sizes, and missing/ambiguous dependency fields. Tests use
synthesized reports, inert staged files, and stub tools only; no native payload,
network, signing, installation, or runtime qualification was performed.
