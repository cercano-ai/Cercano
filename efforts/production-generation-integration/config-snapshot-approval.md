# Approved explicit configuration snapshots — 2026-10-28

User approval supersedes the pause recorded in config-save-boundary-blockers.md. Use explicit immutable versioned snapshots with the existing coordinator; do not introduce a stateful writer. All production save callers must propagate the returned snapshot after success. Setup's second save uses the first save's returned snapshot. Copies retain their original baseline and must conflict without automatic retry against a freshly read revision.

Revision checking and publication remain inside the same cross-process boundary. First-time creation is allowed only for genuine absence and must reject competing creators. Existing malformed/unreadable files must remain untouched; report their path and actionable non-secret diagnostics. No new authority, automatic repair/reset, live-user configuration changes, activation, merge, or push is authorized.

Required tests: sequential saves, stale copies, competing writers, concurrent creation, malformed/unreadable existing authority, failed publication, and no proposed live-state application after rejected saves.

## Independent read-only caller checklist

The bounded review identified these integration obligations (not test results):

- UpdateConfig currently has download/broadcast, compaction, retention, watchdog, cloud installation/rebuild, endpoint, restart, and routing effects before or around ignored persistence. Validate the entire isolated draft and publish before activating the proposal. Test valid early settings plus invalid late patches with zero effects.
- UpdateRoutingAssignments currently Mutate/Persist installs live assignments first. Preserve absent draft vs present-empty clearing and distinguish saved-but-apply-warning from save failure.
- Config service Set/Mutate cancel changed logins. A rejected save must not cancel logins or persist another interleaved caller's proposal.
- Setup performs two saves; an external edit between saves must produce conflict. Setup's independent installation/download effects are not rolled back by a config conflict and must not be described as atomic.
- Startup inline-key relocation spans two stores. Preserve key-confirmed-safe-before-YAML-removal ordering; do not claim config/credential atomicity or exercise native credentials.
- Exercise nested-map/slice draft copies, absence observed before competing creation, malformed/nonregular/permission-denied targets, and faults before rename. A successful atomic rename is not a demonstrated power-loss durability guarantee; current publication does not sync the parent directory.

Review task task_c3de3a7c05d3d514f713d4b59ee90b08 performed source inspection only. Implementation task task_eec32ae963cb57c5edf4cf3216616140 is in progress. No implementation result is claimed here.
