# Acceptance integration update

Foreground no-local-provider root cause fixed in ec7378d11c0a: worker wire snapshot now carries explicit llama_server.enabled. Added snapshot/protobuf/provider-selection regression failed before, passed after. Rebuild tagged binaries/source snapshot to verify full launcher journey; old artifacts still reproduce old code.

Prepared process policy fixed in 877afb4e92ad: pinned worker command includes --prepared and strict constructor rejects missing/invalid metadata before dependency assembly. Helpers must accept this flag, not strip it. Production fake-provider injection remains via host graph, no native opener.

Dispatch failclosed finalization fixed in 648e2455b07f. Independent taskrun->engine error/panic/sibling tests passing. Existing worker path-length fixtures corrected. Combined full server gate had unrelated invalid cloud config classification and concurrent read-only SQLite BUSY failures; fixes underway, so do not claim final gate yet.
