# Inventoried runtime retention primitive

`Publish(ctx, installedSource, retainedRoot, []Member)` copies only explicitly
required regular-file members. Both roots must be absolute and disjoint;
`retainedRoot` must already exist and belong to the caller, outside manager-owned
files. The caller supplies slash-relative paths, expected SHA-256 digests and an
executable flag. Digests detect changed bytes; they are not package authenticity
or signing verification. Package verification remains the native manager's job.

Each call reserves its own private generation slot, copies into `.staging`,
checks each digest while copying, then reopens and validates the whole required
inventory before renaming `.staging` to `bundle`. Only a successful returned path
is eligible for caller admission. There is no mutable current pointer, package
registry, deduplication, selection transaction or rollback engine. Concurrent
calls publish independent bundles, never replace an existing generation. A
failed or cancelled call returns no path and leaves prior bundles unchanged.
Abandoned stages remain ineligible; **no cleanup** is implemented. Cancellation
is checked during copying and before rename, not an interruptible OS I/O promise.
File contents are synced, but directory/power-loss durability is not promised.

Safety follows the existing development publisher's stage/validate/rename
pattern and worker policy's regular-file/digest checks. Those implementations
are Python or worker-private, and their two-binary manifest contract is not
reusable as an asset inventory. This additive Go package uses `os.Root` to confine
source access, rejects static symlink components, rejects traversal and ambiguous
relative paths, and exclusively creates destinations in fresh private slots.
It does not modify existing application manifests or permission policy. New
fixture copies use private 0700 directories/executables and 0600 data. Callers
must keep the retention root and published bundles unmodified; this is not a
security boundary against a hostile same-user writer. Native Windows ACL and
locked-file behavior have not been verified.

## Explicit limitations / next integration seam

- The caller must inventory **every** required executable, manifest, resource,
  backend and library. Omitting an entry from that contract cannot be detected.
  Omitting a file required by the supplied inventory fails publication. Extra
  source files are not copied. `Validate` rechecks required membership/digests;
  it does not infer closure or interpret application manifest semantics.
- There is no new on-disk inventory schema or restart discovery. The existing
  application manifest is an opaque copied member. A future caller must retain
  or reconstruct the trusted inventory for recovery/revalidation; scanning a
  `bundle` directory is not sufficient admission evidence.
- Symlink-based libraries, loader relocation/rpaths, full dynamic-library closure,
  signed bundle metadata/xattrs and installed artifact inventory production remain
  pending. No symlinks, xattrs or special-file metadata are preserved here.
- Worker integration is **test-only**: a fake manager alias resolves to A/B,
  publication returns a retained directory, and the fixture passes it to
  `NewSpawnPolicy`. The helper executes the generation-specific sibling resource.
  The nonretained characterization remains separate and still fails later use.
- Production must connect installed discovery (`pkg/agentclient/client.go`) to
  inventory/publication and compatibility admission in `cmd/cercano` before
  passing the returned canonical directory/manifest into `SpawnOptions`.
  Host/session/reconnect binding, resource lookup authority beyond this helper,
  asset revalidation at later spawn, live dependency tracking and retirement are
  not implemented. Existing worker policy still pins only binary/manifest bytes.
- Remote-only CLI paths are untouched. Native installed-system acceptance,
  package delivery and shared-data/runtime compatibility activation are separate.

Tests use only temporary fixture roots and copied test helpers, with no package
manager, live installation, agent, model or shared store access.
