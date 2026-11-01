# Direct compiled PTY resume verification

## Scope

`efforts/dev-host-generations/everyday-launcher-compiled.py --resume-journey`
launches the installed-command-shaped wrapper, publisher, two compiled ordinary
hosts, and compiled CLIs over real pseudo-terminals. It uses private HOME,
configuration, database, runtime directories and synthetic inference through
existing acceptance build tags. It does not exercise native credentials or a
loaded model, install into the user's bin directory, or touch live sessions.

## Reproduction and correction

The first direct run failed waiting for the takeover prompt even though resumed
title/history appeared. A disposable instrumented rerun showed attachment
conflict delivery to the reducer. `TestTakeoverPromptSurvivesResumeTail` reduced
the ordering: an attachment conflict queues a confirmation, then progressive
history replaces its rendered transcript entry. The confirmation remains active
but invisible. The test failed before the correction. History-tail replacement
now appends the active confirmation after loading history. No takeover authority
or ownership behavior changed.

## Verification

- Small regression: failed before the production edit, passed afterward.
- Full CLI: `go test ./... -count=1` passed with disposable HOME/config.
- UI races: `go test -race ./internal/ui -run 'Takeover|Resume|ConversationAttachment' -count=3` passed.
- Two uninstrumented compiled PTY runs passed after the fix. The final run adds
  durable checks for `/clear` placement, preserved independent work and B's reply.
- Final evidence: `/tmp/cercano-resume-pty-direct-3/compiled.json`, plus A/B PTY
  captures, provider/worker logs and fixture host logs in that directory.
- Both successful runs and the original failing run recorded no remaining
  fixture processes after bounded cleanup.

The final PTY run establishes:

1. A starts, persists history and launches an independent task.
2. B publishes without stopping A; failed build/startup candidates leave A intact.
3. B's CLI resumes A's conversation and shows an explicit takeover prompt.
4. Declining preserves A's ability to execute another turn.
5. `/clear` acquires on default B, then another resume offers takeover again.
6. Explicit acceptance leaves durable owner/epoch/state unchanged on A.
7. The resumed title/history render; a subsequent turn executes on A and its
   assistant reply persists. Independent work remains running.
8. Confirmed Ctrl-C exits the replacement terminal.
9. A fresh ordinary launch selects B, executes a turn there and persists its reply.

## Limits

This is a live-owner resume/takeover journey followed by intentional close, not
post-close reacquisition or dead-owner recovery. The independent task is retained
until fixture cleanup; its normal completion/drain is not asserted here.
Repeating `/resume` for the already-selected conversation after declining did not
re-offer attachment in the instrumented run; this harness deliberately uses
`/clear` before retrying. Same-conversation retry remains a separate UX gap.
The PTY run does not exercise an inherited permission prompt or verify every
revoked-client command; their existing focused tests are not replaced by this
acceptance. No landing, push, production installation or live transition occurred.
