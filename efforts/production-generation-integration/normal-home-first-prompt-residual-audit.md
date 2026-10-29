# Normal-HOME first-prompt residual source audit

2026-10-29. Source inspection only: no launch, native credential access, model request, or live-data inspection. This supplements the incomplete delegated residual audit; it is not whole-application isolation certification.

## Resolved bounded paths

- `source/clients/cli/internal/uiconfig/uiconfig.go`: startup `LoadActiveTheme` and `LoadCustomThemes` are reads. `configHome` honors `XDG_CONFIG_HOME`; explicit `CERCANO_UI_CONFIG` and `CERCANO_THEMES_DIR` override it. Set all to private paths in the acceptance environment, rather than inherit overrides.
- `source/clients/cli/internal/ui/tui_perf.go`: performance logging is **enabled by default**, and its default path is under HOME. Set `CERCANO_TUI_PERF=0` and also give `CERCANO_TUI_PERF_LOG` a private destination. Existing override suffices; no code change is required for the bounded run.
- `source/clients/cli/internal/ui/zz_colorprofile_probe.go`: color-profile logging opens its destination for append/create. Set `CERCANO_COLORPROBE_LOG` to a private destination before CLI initialization; path is cached once.
- `source/clients/cli/internal/ui/conversation_search_trace.go`: trace requires explicit `1`, `true`, or `on`. Set `CERCANO_SEARCH_TRACE=0` and its log override to a private path. Do not inherit ambient trace settings.
- `source/server/internal/dispatch/readiness.go`: HOME expansion is for a configured model path followed by `os.Stat`; the inspected helper performs no writes.
- `source/server/internal/llm/openai/stream_trace.go`: excluded unless built with `cercano_streamtrace`. The intended acceptance build uses only `realcloudacceptance`, not that tag.
- `source/server/internal/server/server.go` HOME use at `defaultModelDir` resolves a model-download destination. A cloud-only, no-tool first prompt does not request model download; this is not permission to exercise runtime administration.
- `source/server/internal/chatgptauth/source.go`, `Source.Token`: valid tokens return directly; expired tokens with refresh material encounter both `s.options.CheckMutation` and `secrets.CheckMutation(s.store)` before `s.flow.Refresh`. Preserve the prepared host's read-only service/guard. A refresh requirement is a stop, not authorization to refresh.

## Still required

Prepared worker failure-log and routing-log authority propagation must both be verified, including successful dependency assembly without fake-HOME writes. The routing repair is in progress. A future launch must explicitly construct the bounded environment (including the CLI overrides above), rather than claim private XDG redirects every HOME consumer. Existing acceptance request/time/token limits and stop-on-credential-error requirements remain unchanged. No launch qualification or rebuilt-generation result is claimed by this note.
