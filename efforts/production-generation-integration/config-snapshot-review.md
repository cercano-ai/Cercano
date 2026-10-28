# Snapshot review follow-up — 2026-10-28

Read-only review task task_ea7909949c1155078e5777c2df951b1d inspected current source after 8fa0fc6989bf; it ran no tests. Findings need reproductions before fixes:

1. Load uses yaml.Unmarshal, whereas coordinator requires exactly one mapping via configDocument. Multi-document or comment-only files may load defaults/first document yet reject save. Align admission and provide path-bearing nonsecret actionable error for invalid authority. Preserve file bytes.
2. UpdateConfig SaveAndSet and subsequent runtime activation are not serialized as one same-host operation. A saves A then pauses; B saves/applies B; A applies old endpoints/providers. Add deterministic barrier test and ordered apply serialization using existing service/server boundaries. No new stateful writer object.
3. Mutate then Persist still leaves rejected draft live. Known existing API behavior; all actual production callers must migrate. Current TestCoordinatedTwoStoreStalePersist asserts disk only, not live rollback. SaveAndSet is the intended proposal boundary, not rollback of arbitrary prior intentional live mutations.
4. Full SaveSnapshot retains legacy struct serialization: unknown YAML fields can be dropped and effective env/default values persisted. This is not established as a new regression; existing precise Change preserves stored fields. Do not silently change full-save semantics or claim document-preserving behavior. Follow approved environment/unknown-field constraints when integrating precise mutations.

Review found baseline captured from same bytes as decoded, cloned baseline data, returned-value advancement, target identity validation, locked absence comparison, and setup sequential propagation correct by inspection.

Parent reran focused race command (credentials disabled, dependency network off): go test -race ./pkg/config ./internal/hostsvc/config ./internal/server ./cmd/cercano -run 'TestSaveBoundary|TestSaveAndSetBoundary|TestUpdateConfig|TestUpdateRouting|TestSetupConfigBoundary|TestCoordinatedTwoStore' -count=1. All four packages passed. This does not cover newly identified interleaving or cross-process acceptance.

Current follow-up implementation task task_f89e13d5a73924aa705332e83051697e addresses remaining reset/cloud/profile/runtime callers. Neither this review nor prior focused results establish completed Phase 3.
