# Bounded README audit: `/rename` and `/resume`

Scope: only the two slash-command table entries in [agent README](agent/README.md#slash-commands), lines 172–173. Read root `AGENTS.md`; compared spelling, arguments, and directly observable handlers in [history.go](../source/clients/cli/internal/slash/history.go). No additional source file was needed.

## Findings and corrections

- `/resume`: spelling matches `Name: "resume"`. The old `/resume <id>` entry omitted the no-argument picker behavior. Document `/resume [id]`: no arguments returns `ResultOpenHistoryPicker`; otherwise the handler returns `ResultResumeConversation` with only `args[0]`, ignoring additional arguments. It does not fetch or validate a transcript here. Exact evidence: `source/clients/cli/internal/slash/history.go:26–37`.
- `/rename`: spelling and the current-conversation form were correct, but the old entry omitted the explicit-ID form. Document both `/rename <title>` and `/rename <id> <title>`. The explicit-ID form is selected only when there are at least two arguments and the first is exactly 24 hexadecimal characters; remaining arguments are joined with spaces. Otherwise all arguments form the title of the current conversation. Exact evidence: `source/clients/cli/internal/slash/history.go:40–62`, with hexadecimal recognition at `74–84`.
- Directly observable rename outcomes: no arguments returns usage (`44–45`); a missing current-conversation callback or empty current ID returns an error message (`57–65`); the handler calls `RenameConversation` with a three-second context, returns a failure message on error, and returns `ResultSetSessionTitle` with the title on success (`47–48`, `66–69`). These details support the corrected entry without claiming persistence or downstream UI behavior.

## Exclusions and remaining questions

No adjacent command audit, persistence internals, transcript loading, downstream result consumers, server behavior, or whole-guide closure. Other README mentions of resume are outside this unit and unchanged. No shared-ledger edits. No unresolved question for the two entries at the handler boundary; durable rename storage and actual resume execution are explicitly unverified and out of scope.

## Validation

Passed: `git diff --check` for the two scoped paths; report whitespace check (including its untracked content); both report-relative local file links and the README slash-command anchor resolve. README changes are confined to lines 172–173 and introduce no links. Application tests are unnecessary for this source-comparison documentation change and were not run.
