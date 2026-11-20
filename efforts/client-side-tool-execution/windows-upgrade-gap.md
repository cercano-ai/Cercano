# Windows upgrade compatibility gap — historical investigation

**Current status (2026-11-20):** the backup-publication gap and bounded manual CLI prerequisite are resolved in `d73b699e`, `eeef482f` and `69c3d877`. Native Windows ARM64 selected tests passed with two existing skips. Manual support is limited to absent/empty retained-host inventory and local NTFS/private-path safeguards; retained/unknown entries, uncertain processes, automatic shutdown and development lifecycle still refuse. No power-loss qualification is claimed. See [durable evidence](windows-upgrade-evidence.md). The original investigation below is retained as history, not a current request for VM access or a remaining blanket Windows blocker.

The macOS-tested upgrade currently refuses backup-requiring migrations on Windows. This is a known blocker, not completed cross-platform support. No native Windows runner was available to the implementation/review tasks in this session.

Bounded investigation task `task_50b56669716cf0ee490f82fd1c72aa7c` recommends a same-directory staging snapshot, flushed/closed before publication using `MoveFileExW` with only `MOVEFILE_WRITE_THROUGH` (no replacement/cross-volume flags). Existing final backups must never be overwritten. Preserve the current failed-attempt inspection/retry barrier; random staging names alone are insufficient. Publication must succeed before source SQL changes. Do not suppress directory-flush errors or fall back to ordinary rename.

Sources inspected by that child:
- https://raw.githubusercontent.com/MicrosoftDocs/sdk-api/docs/sdk-api-src/content/winbase/nf-winbase-movefileexw.md documents that WRITE_THROUGH does not return until the file is moved on disk.
- https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-flushfilebuffers does not establish a directory-publication durability contract.
- https://raw.githubusercontent.com/golang/sys/master/windows/syscall_windows.go exposes MoveFileEx; pinned module availability remains to verify.

This recommendation is not implementation or native verification. Need an isolated Windows test environment before enabling the Windows transition. Tests must cover committed WAL snapshot integrity, existing/concurrently created destinations, sharing/access failures, publication/sync failure before source mutation, Unicode paths, standard-user operation and actual dependency build. Crash/restart testing should distinguish process termination from storage/power-loss durability; no claim for untested filesystems or network shares.

No credential, endpoint, or Windows execution access has been assumed. Ask for a Windows runner/VM, or explicit permission to narrow this slice to macOS while keeping Windows upgrades disabled. Do not push to run CI without authorization.
