# Pinned llama.cpp source build-input audit

## Scope and provenance

This is prerequisite source evidence, not a build recipe, packaging decision, security/policy change, or release qualification. It records read-only inspection of `/tmp/cercano-llama-inspect-dple3nbc/source`, its sibling `acquisition.json`, and this repository's `release/llama/baseline.lock.json`. No network access, downloaded-code execution, configure, build, tests, runtime probes, or live-state inspection was performed for this audit.

**Exact revision: `74976e1aefd8d64845ae5510674b36079c855c0c`.** All upstream citations below are paths relative to that inspected source root, with inclusive line ranges; they are not claims about another revision or current upstream.

Acquisition metadata (`acquisition.json:1–7`) records:

- URL: `https://codeload.github.com/ggml-org/llama.cpp/tar.gz/74976e1aefd8d64845ae5510674b36079c855c0c`
- Archive size: **35,123,325 bytes**.
- SHA-256: **`e5df97a606a8c64d298b3b0536a5bc678650108630dc2e166733988317f07c3d`**.

This digest is **observed acquisition metadata, not independently authenticated**; this audit did not recompute it, authenticate the archive against a separately trusted upstream digest/signature, or independently prove the extracted tree's correspondence to the revision. It must not silently populate the lock's null `source_archive_sha256`.

The lock (`release/llama/baseline.lock.json:1–55`) records a configured local Homebrew baseline, not a source-build qualification: revision evidence is an install receipt; package version is `9890`, short version revision `74976e1ae`, platform `darwin-arm64`, compiler `AppleClang 21`, and the package was built/poured as a bottle. Recorded dependencies are ggml `0.15.3`, OpenSSL `3.6.3`, libomp `22.1.8`, and ca-certificates `2026-05-14` (including their recorded package revisions/rebuilds). `qualified_platforms` is empty; recipe and artifact digest are null. Those observations do not establish equivalent inputs or behavior for a new vendored source build.

## Actual upstream server and Darwin controls

The following are input facts/control points, **not an approved command or a complete release configuration**. No static/shared choice is made here.

| Concern | Actual control and evidence |
| --- | --- |
| Server target | `LLAMA_BUILD_COMMON=ON`, `LLAMA_BUILD_TOOLS=ON`, and `LLAMA_BUILD_SERVER=ON` are needed along this top-level path. Common adds utilities and cpp-httplib; tools is gated by common + tools; tools then gates UI/CLI/server on server. `LLAMA_BUILD_SERVER` alone is insufficient. (`CMakeLists.txt:105–121,206–228`; `tools/CMakeLists.txt:15–40`.) |
| Executable | Target is `llama-server`; it links `llama-server-impl`, which links server-context, llama-ui and cpp-httplib; server-context links common and mtmd. The executable requests C++17. (`tools/server/CMakeLists.txt:5–63`.) Building all tools is broader than building this target. |
| Other top-level artifacts | `LLAMA_BUILD_TESTS`, `LLAMA_BUILD_EXAMPLES`, and `LLAMA_BUILD_APP` are real separate options; standalone defaults enable them. Turning them OFF limits unrelated work but does not itself select a server-only install closure. `LLAMA_BUILD_APP` has its own subdirectory gate. (`CMakeLists.txt:108–121,211–232`.) |
| Vendored ggml | `LLAMA_USE_SYSTEM_GGML=OFF` is the default. ON uses `find_package(ggml REQUIRED)` and an alias; OFF adds `ggml/` if a parent has not already supplied target `ggml`. Thus a standalone vendored build avoids the observed Homebrew ggml dependency, but a parent-supplied target must not be overlooked. (`CMakeLists.txt:34,179–191`.) Vendored ggml declares version **0.15.3** (`ggml/CMakeLists.txt:5–9`); matching that version alone is not binary equivalence. |
| CPU/Metal/Apple math | Actual names are `GGML_CPU`, `GGML_METAL`, `GGML_METAL_EMBED_LIBRARY`, `GGML_ACCELERATE`, `GGML_BLAS`, and `GGML_BLAS_VENDOR`. Apple sets Metal and BLAS defaults ON and BLAS vendor `Apple`; CPU and Accelerate default ON. (`ggml/CMakeLists.txt:96–104,196–206,243–253`.) `LLAMA_METAL` and `LLAMA_METAL_EMBED_LIBRARY` are deprecated forwarders, not preferred names (`CMakeLists.txt:160–168`). |
| CPU portability | `GGML_NATIVE` normally defaults ON; cross-compiling or merely defining environment `SOURCE_DATE_EPOCH` changes its default to OFF. `GGML_CPU_ARM_ARCH` is a separate string control. Explicit `GGML_NATIVE=OFF` avoids relying on that default, but is not by itself an architecture/deployment/reproducibility guarantee. (`ggml/CMakeLists.txt:106–126,187–189`.) |
| OpenMP | `GGML_OPENMP` defaults ON (`ggml/CMakeLists.txt:253`). The lock's libomp observation is not evidence of what a future compiler/discovery configuration resolves. |
| Linkage | `BUILD_SHARED_LIBS`, `GGML_STATIC`, and `GGML_BACKEND_DL` are distinct controls; Darwin's ordinary shared default is not a packaging recommendation. `GGML_BACKEND_DL` defaults OFF and requires shared libraries. (`CMakeLists.txt:36–62`; `ggml/CMakeLists.txt:68–88,122–126`.) This audit leaves linkage and runtime dependency closure unresolved. |

### Embedded Metal means embedded source, not a precompiled metallib

With `GGML_METAL=ON` and `GGML_METAL_EMBED_LIBRARY=ON`, CMake merges `ggml-common.h`, `ggml-metal.metal`, and `ggml-metal-impl.h`, emits assembly with `.incbin` of the merged **`.metal` source**, and adds that assembly to ggml-metal. The section/symbol names contain “metallib” but do not make the bytes a compiled metallib (`ggml/src/ggml-metal/CMakeLists.txt:26–58`). Runtime reads those bytes as UTF-8 and calls `newLibraryWithSource:options:error:` (`ggml/src/ggml-metal/ggml-metal-device.m:124–130,224–233`). Runtime Metal compiler/OS/driver behavior therefore remains an input and qualification concern.

The non-embedded branch instead copies shader inputs and invokes `xcrun -sdk macosx metal` and `metallib` to create `default.metallib`; minimum Metal macOS version and language-standard flags are applied in that branch (`ggml/src/ggml-metal/CMakeLists.txt:59–117`). Do not describe embedded-source mode as shipping that precompiled output, or assume those branch-specific flags constrain runtime source compilation. Foundation, Metal, and MetalKit are required framework discoveries (`ggml/src/ggml-metal/CMakeLists.txt:1–20`).

## HTTPS discovery is not fail-closed

`LLAMA_OPENSSL=ON` is the actual current HTTPS switch and defaults ON (`CMakeLists.txt:123–125`). `LLAMA_CURL` is deprecated and ignored (`CMakeLists.txt:168–170`). In cpp-httplib the OpenSSL branch calls **`find_package(OpenSSL)` without `REQUIRED`**, checks supported header versions (ordinary OpenSSL >= 3.0), and only then links `OpenSSL::SSL`/`OpenSSL::Crypto` and enables support. Missing discovery emits a warning, not a fatal error; an unsupported version also does not enable support (`vendor/cpp-httplib/CMakeLists.txt:126–157`).

Consequently, requesting ON is not proof that HTTPS was built. A later recipe/qualification must record exact headers/libraries/discovery paths and establish successful support rather than trusting configure success; this document does not implement such enforcement. Enabled support adds `CPPHTTPLIB_OPENSSL_SUPPORT` and requires CoreFoundation/Security on Darwin (`vendor/cpp-httplib/CMakeLists.txt:185–190`). The bottle's OpenSSL and CA package observations do not pin discovery, trust-store behavior, certificate validation, or runtime behavior of a new build.

## UI fetch suppression requires both switches OFF

Use the actual controls **`LLAMA_BUILD_UI=OFF` and `LLAMA_USE_PREBUILT_UI=OFF`** to suppress the two automatic UI provisioning routes. They default ON (`CMakeLists.txt:115–116`). The UI target passes them separately as `BUILD_UI` and `HF_ENABLED` to an assets script run on every build (`tools/ui/CMakeLists.txt:72–94`).

- `BUILD_UI` gates npm build; npm provisioning can execute `npm install` and `npm run build` (`scripts/ui-assets.cmake:83–158,304–316`). Turning only prebuilt OFF still permits npm/network dependency acquisition.
- If npm did not provision assets, `HF_ENABLED` independently gates HF download (`scripts/ui-assets.cmake:319–351`). Turning only build OFF still permits this fetch, despite the top-level option description's “requires LLAMA_BUILD_UI=ON” wording.
- HF candidates include the requested version **and `latest`**. The script downloads both archive and adjacent checksum from the same service, compares SHA-256, and extracts; this is not an independently authenticated immutable source pin (`scripts/ui-assets.cmake:178–247`). Environment `HF_UI_VERSION` (and deprecated `HF_WEBUI_VERSION`), build number, bucket cache input and `HF_TOKEN` can affect provisioning (`tools/ui/CMakeLists.txt:3,20–34`; `scripts/ui-assets.cmake:184–204`).
- Both OFF does **not** mean the UI target or embedding helper disappears. A supplied `tools/ui/dist/index.html` wins before these gates, and fallback can embed stale binary-directory assets or emit no embedded UI (`scripts/ui-assets.cmake:295–302,354–369`). Clean, controlled source/build directories remain inputs. Deprecated `LLAMA_BUILD_WEBUI`/`LLAMA_USE_PREBUILT_WEBUI` can override their newer counterparts (`tools/ui/CMakeLists.txt:6–18`).

These switches address UI provisioning, not a universal no-network guarantee or runtime downloads.

## Other optional acquisition paths

Keep optional fetching features explicitly controlled in any future offline input inventory; do not infer that UI flags disable all downloads:

- `LLAMA_BUILD_BORINGSSL=OFF` and `LLAMA_BUILD_LIBRESSL=OFF` keep the alternative TLS fetch branches inactive. These are tested variables in this file (not top-level `option()` declarations); BoringSSL fetches a git repository/tag, LibreSSL fetches a release URL, and both take precedence over the normal OpenSSL branch (`vendor/cpp-httplib/CMakeLists.txt:40–126`).
- `LLAMA_LLGUIDANCE=OFF` (the default) avoids an ExternalProject git acquisition and Cargo build. The enabled branch pins a git commit but adds a Rust dependency/toolchain closure (`CMakeLists.txt:125`; `common/CMakeLists.txt:138–167`).
- `GGML_CPU_KLEIDIAI=OFF` (default) avoids an ARM-relevant FetchContent archive; enabled code fetches `v1.24.0` with an MD5 URL hash (`ggml/CMakeLists.txt:157`; `ggml/src/ggml-cpu/CMakeLists.txt:578–621`). That checksum is not a substitute for a separately authenticated release input.
- Non-Darwin-oriented optional backends also contain acquisition machinery: CUDA FetchContent (`ggml/src/ggml-cuda/CMakeLists.txt:63–72`), Vulkan ExternalProject (`ggml/src/ggml-vulkan/CMakeLists.txt:158–165`), ZenDNN git ExternalProject (`ggml/src/ggml-zendnn/CMakeLists.txt:21–51`), and VirtGPU header download (`ggml/src/ggml-virtgpu/CMakeLists.txt:4–18`). Their switches include `GGML_CUDA`, `GGML_VULKAN`, `GGML_ZENDNN`, `GGML_VIRTGPU`, and `GGML_VIRTGPU_BACKEND`, default OFF (`ggml/CMakeLists.txt:208,232,241–242,273`). Enabling a different backend requires a fresh dependency/fetch audit; this is not an exhaustive offline certification of every optional path.

## Build identity and reproducibility limits

Top-level CMake accepts `LLAMA_BUILD_NUMBER` and `LLAMA_BUILD_COMMIT` overrides when defined, otherwise uses discovered build info, and derives install version `0.0.<number>` (`CMakeLists.txt:129–138`). `cmake/build-info.cmake:1–47` starts with `0`/`unknown`, attempts git short-HEAD/count discovery, and records compiler ID/version plus platform/processor. The generated common build-info embeds those strings (`common/CMakeLists.txt:9–40`; `common/build-info.cpp.in:6–9`). An archive build must not assume automatic git identity equals the acquisition pin. The bottle's `9890`/`74976e1ae` is observed identity, not newly established revision-count evidence for this archive.

Vendored ggml independently attempts git short-HEAD and dirty-state discovery and can replace a parent-supplied commit with `unknown`/a detected value (`ggml/CMakeLists.txt:13–40`; parent assignment at `CMakeLists.txt:186–189`). Later identity handling must account for both paths; merely passing top-level identity overrides is not proof of all emitted metadata.

The minimum CMake declaration is `3.14...3.28`, C/C++ are project languages, and non-Xcode/non-MSVC builds default to Release when unspecified (`CMakeLists.txt:1–13`); the UI script separately requires CMake 3.18 (`scripts/ui-assets.cmake:8`). These are compatibility declarations, not exact toolchain pins. `GGML_CCACHE` defaults ON (`ggml/CMakeLists.txt:126`). Cross-compiling UI discovers a host compiler or accepts `HOST_CXX_COMPILER`, adding another tool input (`tools/ui/CMakeLists.txt:39–69`).

No exact Xcode/Command Line Tools build, compiler binary, SDK identity, macOS deployment target, architecture set, CMake/generator/linker versions, environment, dependency artifacts, paths, timestamps, signing/notarization inputs, or full configure cache is established here. `AppleClang 21` in the baseline is insufficient to recreate them. `SOURCE_DATE_EPOCH` affecting a native-optimization default is not a reproducible-build guarantee. Neither bit-for-bit reproducibility nor baseline performance/behavior equivalence has been demonstrated; embedded Metal additionally retains runtime compilation variability.

## License obligations and remaining inventory

Upstream `LICENSE:1–21` is MIT, copyright 2023–2026 The ggml authors: retain the copyright and permission notice in all copies or substantial portions, including binary distributions as applicable, and preserve the disclaimer. Vendored cpp-httplib is separately MIT, copyright 2017 yhirose, with the same notice condition (`vendor/cpp-httplib/LICENSE:1–21`). Do not assume one project-level notice exhausts third-party obligations.

CMake registers the root license and cpp-httplib license, and the alternative TLS branches register their own license files (`CMakeLists.txt:173`; `vendor/cpp-httplib/CMakeLists.txt:2,78,121`). Aggregation helpers read registered files and can generate an embedded license array, but missing files merely warn; registration alone does not prove the server artifact includes every required notice (`cmake/license.cmake:6–40`). Before distribution, inventory the actual vendored/linked components, optional kernels/UI assets and TLS/runtime dependencies, retain their applicable notices/licenses, and verify the delivered notice bundle against the chosen artifact closure. This audit is neither a completed third-party license inventory nor legal approval.

## Evidence boundary

This document adds durable pinned-source evidence only. It does not choose static/shared packaging, change the lock, establish security policy, supply a verified recipe, or qualify any build/runtime/platform. All unresolved discovery, provenance, toolchain, dependency, licensing and behavioral checks remain prerequisites to a later release decision.
