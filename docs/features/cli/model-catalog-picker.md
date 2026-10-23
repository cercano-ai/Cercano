# Model catalog picker

Cloud account model choices use a searchable results-and-details popover instead of placing all metadata in a dropdown label. The same component powers the read-only Mistral catalog across accounts.

## Using the picker

Open a quality-tier or image-model control in **Config → Cloud**. Search is focused immediately. Exact model IDs/names rank before prefix and substring matches; aliases are searchable. The current selection is marked separately from the highlighted result.

- **Up/Down:** navigate results; when details are focused, scroll details.
- **Tab / Shift+Tab:** move among search, filters, results and details.
- **Enter:** select a model/action, or toggle the focused filter. Browse mode does not change configuration.
- **Ctrl+F:** return to search. **Ctrl+U:** clear text before the search cursor.
- **Ctrl+R:** refresh the catalog; duplicate pending requests are coalesced.
- **PageUp/PageDown:** scroll the active region.
- **Escape:** return from compact details, then close without selecting.
- In provider-wide browse mode, focus filters and use **Left/Right** to choose an account. The list remains one row per model; details retain each account's own metadata.

Wide terminals show results and details side by side. Smaller terminals stack them, and compact terminals use a Details/Back view. Full IDs remain available in scrollable details even when the closed selection control truncates its label.

Selections change only the account draft. **Save** is still required to persist them. Inheritance, no image override and custom model IDs remain explicit, separate actions. An ID missing from the latest catalog is retained with an access-unconfirmed label, not silently removed. Unsupported or unconfirmed image capabilities cannot bypass server validation.

## Credentials and discovery

For key-backed providers that require credentials, an account without a saved key displays **“Add an API key to load available models.”** The action focuses the existing masked editor; it does not save anything. An edited key or account identity instead prompts **“Save this account to load its available models.”** New subscription accounts use the existing sign-in flow. Public discovery and custom endpoints are not universally forced behind a key requirement.

After a successful save/sign-in, the visible account's catalog is refreshed asynchronously. Failed saves retain the draft. Loading, refreshing, successful empty catalogs, search misses, discovery failure and partial account failure have distinct messages. Existing curated fallback data is labeled static/unverified in details. Unknown capability, price and context values remain Unknown.

Current discovery RPCs do not consistently provide typed authentication/rate-limit errors. The UI uses a generic discovery error when it cannot establish a more specific cause; it does not guess by parsing arbitrary provider strings.

## Component contract

`source/clients/cli/internal/overlay/catalog.go` implements `Catalog`, a presentation-only component:

- `NewCatalog(title, scope, browse)` creates a selection or browse view.
- `SetEntries` installs typed `CatalogEntry` values and preserves highlighted identity where possible.
- `CatalogEntry` keeps exact ID/account, aliases, current/unconfirmed/disabled state and structured detail rows separate from labels. Browse entries can list multiple accounts with account-scoped detail rows.
- `CatalogState` carries discovery copy, a primary action and busy state.
- `Update` returns a command and typed `CatalogOutcome`; the host decides whether and how to mutate drafts.
- Model selection, inheritance, no override, custom entry, cancel, refresh and credential focus are distinct `CatalogAction` values.
- `Resize`, `View` and `Paste` support terminal layout and input without owning an API client or any credentials.

`internal/ui/cloud_catalog_picker.go` owns RPC scheduling and maps agent metadata into the component. Result messages carry page identity, saved scope and request generation. Account/endpoint/key/lifecycle changes invalidate pending work. Replies cannot cross accounts or update a replacement page. Async results defer form reconstruction while an editor has uncommitted input, so a catalog reply cannot discard a partially typed credential.

Runtime/download management and conversation-model selection have not been migrated. Their eventual reuse must provide their own selection semantics and discovery adapters; this component does not download models, change routing or bypass permissions.

## Verification coverage

Focused and full affected CLI suites cover selection/cancel, exact account identity, capability/unknown metadata, inheritance, custom IDs, account filters, credential focus, failed/successful save, automatic post-save discovery, stale replies and active-editor preservation. Render fixtures cover six states at 120x36, 100x24, 80x24 and 60x18 across built-in themes.

For fixture exports, set `CERCANO_CATALOG_RENDER_DIR` and run `go test ./internal/overlay -run TestCatalogRenderStates -count=1` from `source/clients/cli`.

`TestCatalogTerminal` is an opt-in real Bubble Tea terminal harness (`CERCANO_CATALOG_TTY=1`) using deterministic model data, not an agent connection. The original feature verification recorded a pseudo-terminal driver exercising search/select and resize/details/cancel; that run is historical, not a result of the current documentation audit. This is not a claim of manual end-to-end testing against a user's live configuration or provider account. No paid inference is needed for UI verification.
