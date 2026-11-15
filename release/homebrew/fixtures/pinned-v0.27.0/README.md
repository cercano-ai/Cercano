# Sanitized live response fixtures (2026-11-15)

Obtained with existing authenticated `gh api --method GET` against
`repos/cercano-ai/Cercano/actions/runs/43655050601/attempts/{1,2}/jobs?per_page=100`
and `actions/jobs/{132631632452,132631632597,132634405380}/logs`.
Metadata projects only validator-consumed job fields (including exact steps).
Logs retain original timestamped checkout, source/workflow, upload output and
multiline upload-path records, omitting unrelated build output. No tokens,
signed storage URLs, credentials or local machine paths are included.
These excerpts test real response syntax, not a replacement for full live logs.
