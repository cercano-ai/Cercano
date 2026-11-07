# Enterprise physical-source usage rollout

Status: implementation candidate only. No production rollout has occurred.

The signed `route_id` selects a model route. The opaque
`source_connection_id` identifies the **actual physical billing connection**
used for one provider attempt. It is captured before the adapter call and
cannot be changed by a later usage observation. A custom adapter's backend
label is not the signed API provider used for policy matching or pricing.

## Required order

1. Deploy an Enterprise server that accepts `cercano.usage-report.v2` and
   retains v1 compatibility. Verify authentication and the PostgreSQL source
   registry without publishing a source-aware policy yet.
2. Upgrade the client host and every attached worker together to the build
   carrying accounting protobuf fields 23–32 and the durable v2 outbox.
   Do not leave an old worker attached to a managed host: old observations
   lack report scope, and an old host can discard new fields. This pair is
   not a supported mixed-version metering configuration.
3. Publish a policy version with registered physical source IDs and matching
   per-route allowlists only after the host and workers are upgraded. Set
   `minimum_client_version` above every old client build; the release build
   must expose a truthful version greater than the existing installed version.
   Configure the corresponding non-secret ID on each saved credential
   profile. For a local runtime, set this host's
   `local_source_connection_id` to an allowed ID. The client never guesses
   it from a policy shared by multiple hosts.
4. Verify one real permitted attempt per physical connection, its outbox
   acknowledgement, and the cost coverage status before broadening use.
   Compare a sample to provider invoices separately; estimated client
   reports are not invoice guarantees.

## Compatibility behavior

- A v1-only server rejects a v2 upload. The client retains the exact durable
  events and retries later; it never downgrades to route-only v1 pricing.
- Legacy attempt rows and old worker observations have unknown source.
  They are not retroactively assigned to a connection. A new managed host
  marks accounting coverage incomplete when it receives an old worker batch,
  while preserving its local attempt. An old worker must still be prevented
  by the coordinated upgrade step above; an old host cannot detect this gap.
- Invalid managed conversion does not roll back the local attempt. It leaves
  a fixed `conversion_failed` outbox row excluded from automatic upload.
- A missing physical ID is retained as unclassified and unpriced. A reported
  ID outside the signed route registry is rejected. The server binds the
  organization and host from the native session, not from report JSON.
- Provider SDK retries below a physical adapter boundary remain a coverage
  limitation unless that SDK path emits separate attempt IDs. No subscription
  fee or API-rate proxy charge is inferred from subscription token volume.
