# Everyday launcher regression gate (pre-landing)

Main-agent checks on the integration worktree after permission checkpoint `026cdf9c577a` and preceding source fixes:

- Full server `go test ./... -count=1 -json -timeout=15m`: PASS, 117 packages; 36 test skips.
- Full CLI `go test ./... -count=1 -json -timeout=10m`: PASS, 10 packages; 1 test skip.
- Full race checks for conversation, server, worker, taskrun, hostsvc/tools, hostsvc/persistence, pkg/agentclient: PASS.
- Full CLI UI race checks: PASS.
- Server/CLI builds passed in the preceding gate; later source changes require refreshed build/landed smoke.

Commands, isolated environment and logs: `/private/tmp/el-mcnw2t5f/results-current.json`, `env.json`, `server-current.log`, `cli-current.log`, `server-race-current.log`, `cli-race-current.log`. Tests ran with outer `CERCANO_DISABLE_CREDENTIALS=1`, offline Go module lookup, private HOME/XDG/short TMP. Existing enabled test scopes use injected memory stores/fake providers; the repaired prepared-worker credential test enables only its in-process fake dependency graph.

This is not whole-user-workflow acceptance. The ordinary compiled launcher journey is opt-in and was excluded by its built-in skip; its current result must be reported separately. These are not real-provider/model inference tests. Race flags on parent test executables do not instrument separately compiled children.

Earlier failed gates are retained. They exposed invalid-cloud configuration masking, concurrent preflight lock handling, fixture worker flag parsing, and duplicate prepared-policy metadata. Each failure was reproduced and fixed rather than ignored. An additional reviewed stale permission-result UI fix is in progress and requires fresh CLI checks.

No local landing, installed launcher cutover or native credential/model access is claimed by this note. Installed command remains old pending separately approved replacement.
