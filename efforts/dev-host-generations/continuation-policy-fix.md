# Continuation policy verification

f0559a8ee995 fixes a reproduced duplicate prepared-policy metadata failure: a task/completion context retains `cercano-worker-host-credentials-only=true`, and RunTurn previously appended another value. Worker correctly rejected `[true true]`. Host now replaces its authoritative key while preserving other metadata/context; strict validation is unchanged.

Probe `TestPreparedCredentialsRealWorkerIsolation/inherited` failed with observed `[true true]` before the fix and passed after. Injected memory credential test admission is now locally enabled via credentialtest helper; no native opener or child process in that test. Tagged legacy devhostfixture init now consumes --prepared through strict parser and constructor option.

Focused race checks for worker credential integration, TestDevHostCompiledExecution and TestDevHostTerminalPTY passed from main driver, logs `/private/tmp/el-mcnw2t5f/fixture-repair-after.log`. This is regression evidence, NOT yet ordinary launcher A/B acceptance. New ordinary launcher snapshots must include this commit to exercise independent continuation successfully.
