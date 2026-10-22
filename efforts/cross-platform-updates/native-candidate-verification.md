# Native candidate verification — approved direction

The user approved updater-side native process/listener verification rather than adding a private credential/handshake. GetAgentHealth remains observational. Unknown process or endpoint ownership must never authorize health success. No new listener or credentials are authorized.

## Shared inspection foundation

The macOS libproc Inspect and HoldsListener implementations have been moved unchanged into internal/updatecoord/nativeprocess. brewrestart retains forwarding functions and an Identity type alias; its formula-specific ownership policy stays in brewrestart. This avoids making the generic updater depend on Homebrew layout policy and keeps a single native implementation.

A new disposable listener fixture checks current-process identity, owned loopback listening, stale lifetime rejection, foreign identity rejection and invalid endpoints. Existing Homebrew integration tests exercise the forwarding path.

Verification: both package race suites and vet pass locally on macOS. Builds pass with cgo disabled and for Windows/amd64 and Linux/amd64. Windows/Linux native inspection is still explicitly unsupported; those builds are not execution evidence. The existing updatecoord CI glob includes the new package, but no new CI run or push was performed.

## Linux identity slice

Linux Inspect now reads bounded stat/status records and the kernel executable link through an anchored /proc directory, repeating identity observations before returning. Linux start ticks have their own Identity field, distinct from macOS wall-clock start seconds/microseconds. Missing proc data remains uncertainty, never manufactured ESRCH proof. Listener inspection still returns an explicit unsupported error.

Portable parser/lifetime tests and existing macOS race suites pass. Linux vet and test compilation, plus Windows builds, pass. Following the user's push authorization, native CI run 39187460069 at 7e867a5d8e50561dd81e51ad7af4973376ec1bdd completed successfully on ubuntu-24.04, windows-latest and macos-15. The Linux-only identity fixtures now have native execution evidence; they check the actual current process and invalid/absent PIDs. This clears the identity-slice verification blocker, not the still-unsupported Linux listener check or end-to-end health gate.

Run: https://github.com/cercano-ai/Cercano/actions/runs/39187460069

## Remaining boundary

HoldsListener is a per-process observation, explicitly not proof of unique endpoint ownership. A responder's reported PID or a server-side endpoint self-check cannot replace updater-side evidence. Candidate launch lifetime binding, Windows/Linux inspection, competing listener handling, bounded health probing and reconnect evidence remain to be implemented before health may authorize activation completion. No live agent or installed files were changed.

## Linux listener slice — approved scope and implementation

User approval (this session, no later instruction supersedes it): bounded Linux HoldsListener in internal/updatecoord/nativeprocess, using /proc TCP tables matched to exact process socket descriptors, repeated identity checks, no elevation, and fail-closed uncertainty. Scope guardrails honored:

- Target-network-namespace tables (/proc/<pid>/net/tcp{,6}) via an anchored /proc/<pid> root, not the host namespace tables.
- Endpoint restricted to explicit loopback address, nonzero port, no zone. Stale/foreign identity and any user other than the current UID are rejected; identity is re-inspected after the socket observation.
- Exact family/address matches plus family-appropriate wildcard bindings are accepted. A dual-stack IPv6 wildcard socket asked about an IPv4 endpoint is explicitly undecided — /proc does not expose IPV6_V6ONLY — and is reported as uncertainty, not as a match or a miss. Never claims exclusive endpoint ownership or service health.
- Two socket-inode sets bracket the table reads through anchored /proc/<pid>/fd; a changed set, vanished descriptor during enumeration, unreadable input or invalid parsed framing/fields is an error. These repeated observations are not an atomic kernel snapshot and cannot exclude changes that occur and reverse between samples.
- All reads bounded (fd count, read sizes, table bytes, batched directory reads); kernel word endianness handled via native layout; the decimal inode column and hex address/port/state columns are decoded accordingly.
- Pure parsers (listener_parse.go) are platform-independent and covered by portable tests: table parsing for IPv4/IPv6, wildcard matching, dual-stack ambiguity, malformed/truncated/oversized record rejection, socket-link decoding and bounded-read rejection.

Linux-native fixtures (listener_linux_test.go): owned IPv4 listener, IPv6 loopback and wildcard, IPv4 wildcard, closed listener becomes verified false only after fd stability, stale-identity and foreign-UID rejection, invalid endpoints (non-loopback, zero port, zoned), a helper process proving an unrelated same-user process's listener is not ours and is ours when asked for its own identity, vanished-process uncertainty instead of clean absence, and a churn test asserting fd churn can never produce a verified false for a held listener.

Verification: macOS host — go test, go vet and go test -race for updatecoord and brewrestart all pass, full module builds. Linux/amd64 with cgo disabled — go build ./..., go vet and test compilation pass. After the user's push authorization, native CI run 39195494165 at 5fc23221662f5472f5594a400e873ac8e1cbacd8 completed successfully on ubuntu-24.04, windows-latest and macos-15. The Ubuntu native temporary-state/subprocess and race-test steps passed, providing execution evidence for the Linux listener fixtures. Windows nativeprocess inspection remains unsupported; a green Windows build does not imply implementation. Darwin implementation is untouched. No production health wiring or live agent actions.

Run: https://github.com/cercano-ai/Cercano/actions/runs/39195494165

Review caught legitimate non-LISTEN TIME_WAIT rows with zero inode being rejected; a failing portable regression demonstrated the issue before correction. LISTEN rows still require nonzero inodes, including off-port rows. Test subprocesses are bounded and reaped on failure paths.

Support limits: both TCP tables must be readable. A host with IPv6 disabled and no tcp6 table returns uncertainty even for IPv4; missing data is not interpreted as absence. The observed listener is in the target's network namespace, which does not establish reachability from the updater's namespace. No exclusivity, exit, or health-success inference is authorized.

## Windows identity slice — native verification

Native CI run [39318042928](https://github.com/cercano-ai/Cercano/actions/runs/39318042928), commit `a8e63ac4e7342f6a88b6f359887991c3e328f33a`, passed on Windows, Ubuntu and macOS, including the full CLI suite. Unix race checks passed; Windows race checks remain skipped. This supersedes the earlier Windows identity implementation/verification limitation, not the remaining listener or health integration boundaries.

The Windows fixture previously compared a kernel-reported short executable path with an expanded long pathname. The correction compares pinned file identity rather than spelling; the alias regression resolves the long form first so an already-short executable pathname does not itself cause a skip. Production identity inspection was not changed by this fixture correction. Windows creation time retains raw unsigned FILETIME representation rather than converted Unix nanoseconds.

These results do not establish exclusive listener ownership, candidate health, descendant exit, or completed update integration. No live agent was stopped or installation modified.

## Windows listener slice — implementation and native verification

The approved scope is bounded GetExtendedTcpTable owner-PID listener observation with same-user process-lifetime checks, no elevation, and no health/activation wiring.

The interrupted delegation left an unverified implementation and premature verification notes. Those notes are superseded here. Review found incorrect row sizes and an incorrect table-class constant. Microsoft documents OWNER_PID_LISTENER as enum value 3; the OWNER_PID row layouts are 24 bytes for IPv4 and 56 bytes for IPv6. The portable parser correction is committed as `131eff034e4c`; its host race suite passed again during integration review. Buffers are capped at 16 MiB and unused port DWORD padding is ignored.

The native adapter is checkpointed as `c162c598bdd4`, using table class 3, a portable injectable bounded query driver, and two order-independent target-PID snapshots; unrelated process traffic does not invalidate the comparison. The native fixture correction is checkpointed as `8318f8010a47`: ephemeral owned listeners, foreign-owner/port checks, independent IPv4 wildcard coverage, IPv6 availability-specific skips, and bounded helper cleanup. An additional host regression reproduced rejection of a four-byte empty count table; the driver now accepts that table while checking nonempty counts against returned bytes.

Combined verification during this integration turn passed: host `go test -race -count=1` and `go vet` for nativeprocess and brewrestart; Windows AMD64 package vet and full test-binary compilation. These are not native Windows execution. Native CI is still required before claiming completed Windows listener verification. Process/listener observations do not establish exclusivity, health, descendant exit, or activation completion. No live agent or installation was changed.

Native verification now supersedes the pending gate above: run [39360914629](https://github.com/cercano-ai/Cercano/actions/runs/39360914629), at `b003a0a90d0a05148e1e2411badc00bd559439c2`, completed successfully on Windows, macOS and Ubuntu. This includes the native updater fixtures, health RPC fixtures and full CLI suite; race checks remain Unix-only. The preceding run passed Windows and Ubuntu but exposed an unrelated macOS safe-stop cancellation fixture timing assumption. Commit `b003a0a90d0a` synchronizes that fixture with actual handler completion and adds a deterministic observation test: an idempotent accepted response does not join the original stop callback. Production shutdown semantics are unchanged.

This clears the Windows listener native verification gate only. Exact candidate lifetime binding, competing endpoint handling, health/reconnect integration and descendant completion remain separate requirements. No live process or installed selection was changed.

## Owned candidate lifetime wrapper — native verification

Run [39375147173](https://github.com/cercano-ai/Cercano/actions/runs/39375147173), at `5f0180c13cc9a456105ba4573ede7fbd14f523dc`, passed on Windows, Ubuntu and macOS. Native updater/subprocess fixtures, stable-launcher checks, health RPC fixtures and the full CLI suite passed; race checks passed on Unix and were skipped on Windows.

The candidate wrapper captures identity while observation of the exact launched child brackets inspection and before its reaper starts. Linux uses a process handle backed by pidfd; Windows uses the owned process handle; macOS uses non-reaping child observation and refuses incompatible child-reaping policies. Unsupported or uncertain observation fails closed. Post-start inspection failure retains the owned Process so completion remains observable. Generic helper Launch behavior is unchanged.

Independent review found no concrete blockers in lifetime capture, exited-child refusal, completion retention or generic-launch compatibility. This clears native verification for the wrapper, not health success: per-process listener observations still do not establish endpoint exclusivity, authenticated response binding, descendant completion, client reconnection or completed activation. No live agent or installation was changed.

## Policy supersession — 2026-10-22

The user now approves exact owned candidate identity/listener plus expected release and readiness within a bounded deadline as the operational health-success gate. Earlier reconnect-evidence requirements above are superseded: terminals reconnect independently and cannot block success. Observation limitations remain; this is not authenticated peer identity. See candidate-health-observation-verification.md for the bounded integration slice. Failed/uncertain health retains previous-version/recovery facts; rollback requires owned-candidate exit proof, not timeout or PID alone.
