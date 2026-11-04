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

## Opt-in fresh Darwin staged-image collection

For a caller-owned **stable** stage, a separate command collects native output
and immediately feeds the same closure checker. The offline `--loader-reports`
command above remains unchanged and does not gain freshness guarantees.

```sh
python3 -B scripts/darwin_staged_reports.py \
  --staged-root /path/to/private-stage \
  --image-inventory /path/to/images.json --architecture arm64
python3 -B scripts/test-darwin-staged-reports.py
```

`images.json` has exactly these fields (paths are stage-relative):

```json
{
  "executable": "bin/llama-server",
  "backends": ["libexec/ggml-cpu"],
  "images": ["bin/llama-server", "lib/libllama.dylib", "libexec/ggml-cpu"]
}
```

This is an explicit caller inventory, not automatic discovery. Include **every**
static dependency and every selected backend root. The sample only illustrates
shape, not a complete usable payload. Missing images/backends, extra unreachable
images, unsafe paths and duplicate physical-image aliases fail through shared
closure validation. Internal aliases remain allowed only under that check;
current release archives still prohibit symlinks.

The collector invokes only the fixed trusted host `/usr/bin/otool`, with `-hv`
for thin Mach-O identity/architecture and `-l` for load commands. It never
executes staged payloads. It accepts only little-endian 64-bit thin Mach-O for
the explicitly selected `arm64` or `x86_64` architecture; the executable must be
`EXECUTE` and other images `DYLIB`/`BUNDLE`. Fat/universal, non-Mach-O, unknown
CPU/type or malformed/unsupported reports fail. Tool calls have a 10-second
per-call timeout, a minimal environment, capped diagnostics, and a 4 MiB accepted
stdout limit; missing tools, nonzero exits and stderr diagnostics fail closed.

SHA-256 snapshots of all supplied images precede inspection; each inspected
image is rehashed afterward, and all images and inventory path resolutions are
rechecked after closure validation. Reports and hashes are ephemeral: there is
no new persistent hash-report schema, report export or source-provenance claim.
The caller must own the stage and keep it stable throughout the operation.
These checks detect ordinary mutation, **not hostile concurrency**, transient
write-and-restore races, or changes after return; they do not lock or freeze the
stage. This opt-in check does not modify archive requirements or qualify native
payloads. Dynamic discovery and resource completeness remain caller obligations
below. Platform-independent collector regressions inject host-tool responses
against inert temporary byte fixtures; they require no native binaries or
installed payload. The separate native integration probe below is opt-in.

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


## Opt-in native Darwin integration probe (2026-11-03)

User direction is **Mac-first**; Windows native validation is deferred, not
qualified. This probe is separate from the platform-independent suites and
release gates:

```sh
CERCANO_DARWIN_NATIVE_TESTS=1 python3 -B scripts/test-darwin-native-staged-reports.py -v
```

Without the environment opt-in, all three tests explicitly skip. With opt-in,
non-Darwin hosts, unsupported host architectures, or missing host xcrun,
otool, clang/SDK explicitly skip. A present toolchain's compilation or inspection
failures remain failures, not skips. Each case authors two minimal C sources in
a fresh temporary directory and invokes trusted host `/usr/bin/xcrun clang`
with a minimal environment. It downloads nothing and **never runs the fixture
executable or loads the dylib**. It uses the default real `/usr/bin/otool`
collector and closure validator, with no mocks or report rewriting.

The analytical contract is a two-image closure: a dylib installed as
`@rpath/libfixture.dylib`, an executable with
`@executable_path/../lib` runpath, and success after renaming the entire stage
(including a space in its new path). The dylib also serves as the explicitly
required backend root; this does not test backend discovery. The negative cases
retain complete image/backend inventories but remove the rpath target by
renaming the library, or embed an absolute non-system install name outside the
stage. They require the corresponding dependency-resolution rejection, not just
any failure.

**Current native result: BLOCKED, not PASS.** Darwin 25.5.0 arm64, Apple clang
21.0.0 (`clang-2100.1.1.101`), SDK 26.5: the command exited 1 with **3 tests,
2 failures and 1 error**. All hit `unexpected report header or diagnostic`
before dependency resolution. Real `otool -l` emits unindented `Section`
markers inside `LC_SEGMENT_64`; the parser currently requires every nonempty
load-command payload line to be indented (`darwin_loader_closure.py:58`). A
single `int main(void) { return 0; }` executable and one isolated real segment
block reproduce the rejection; compiling and collecting that output both exit
0 with no otool stderr. No downloaded or installed payload is involved.

Changing this rule affects the fail-closed header/diagnostic boundary. Per the
user's security-semantics stop condition, production collector/parser policy is
unchanged; the strict native assertions remain failing rather than being
weakened, mocked, skipped or marked expected failures. A reviewed parser grammar
change is needed before the intended native closure outcomes can be evaluated.
The 15 collector and 20 closure platform-independent regressions still pass;
the ungated native invocation explicitly skips all 3. Exact evidence and limits
are recorded in `efforts/cross-platform-updates/verification.md`.

This is not runtime/dyld execution validation, installed-system acceptance,
llama.cpp provenance, signing, universal/per-architecture qualification, resource
completeness or release qualification. No live installation/config/state/VM
changes, signing credentials, network, model startup, push or merge were used.
