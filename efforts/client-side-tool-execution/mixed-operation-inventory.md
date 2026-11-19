# Mixed operation inventory — partial

Phase 1 remains incomplete. Read-only delegated evidence from `task_18bd9c07297a0957ce51ce7457c29398`; findings below are reported inspection results, not implemented routing or verified tests. Paths relative to `source/server/internal/capabilities/builtins/`.

| Capability | Located workspace dependency | Host dependency / required seam |
|---|---|---|
| summarize | `coproc_summarize.go:summarizeCap.Execute`, direct `os.ReadFile` at line 55 for file_path | `runTextAnalysis` in `coproc.go` calls host Dispatch with project context; separate file/context reads from inference |
| extract | `coproc_extract.go:extractCap.Execute`, file read at line 59 | Same host text-analysis dispatcher; supplied text needs no file read |
| classify | `coproc_classify.go:classifyCap.Execute`, file read at line 55 | Same split |
| explain | `coproc_explain.go:explainCap.Execute`, file read at line 53 | Same split |
| review | `review.go:reviewCap.Execute`, tool-enabled branch uses Agentic dispatch and `call.WorkDir`; text-only branch OneShot | Host inference remains host-side; delegated tools and automatic context must inherit execution binding. This function alone does not enforce its description's read-only restriction; downstream enforcement not yet checked |
| research | `web_research.go`, output_dir persists resumable research state; explicit write-permission check | Actual Python runner and output-path handling still need inspection |
| deep_research | `web_deep_research.go`, output_dir writes reports/findings; explicit write-permission check | Actual runner and filesystem access still need inspection |
| local | `local_offload.go` located | Contents not inspected by this slice |
| deepinfra_infer | `deepinfra_infer.go` located | Attachment loading, result artifacts and host-secret boundary not inspected by this slice |

For client-bound operation, route workspace pieces independently while retaining model/service calls and credentials on the host, or reject the unsupported variant explicitly. Never send the entire mixed capability to a client merely because it accepts a path. This is the approved host-orchestration boundary applied to identified paths, not a new routing implementation.
