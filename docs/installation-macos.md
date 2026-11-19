# Installing Cercano on macOS

Cercano is distributed for **Apple Silicon Macs running macOS 12 (Monterey) or
later**. Intel Macs are not supported by this release.

The formulas are published in the official dedicated
[`cercano-ai/homebrew-cercano`](https://github.com/cercano-ai/homebrew-cercano)
tap. The personal `bryancostanich/homebrew-tap` remains separate; its Cercano
formula is deprecated.

## Install

```bash
brew install cercano-ai/cercano/cercano-agent cercano-ai/cercano/cercano-cli cercano-ai/cercano/cercano
```

Homebrew 6.0.0 and later only load formulas from a third-party tap that you
have trusted ([Tap Trust](https://docs.brew.sh/Tap-Trust)). Naming a fully
qualified formula trusts only that formula, so installing just
`cercano-ai/cercano/cercano` stops at its dependencies with "Refusing to load
formula cercano-ai/cercano/cercano-agent from untrusted tap". Naming all three
avoids that. To trust the whole tap instead, including future formulas, run
`brew trust cercano-ai/cercano` once.

Since v0.27.0, `cercano` is a dependency-only convenience formula: it installs
`cercano-agent` and `cercano-cli`, which own the two commands below. The split was
published in [tap commit `0557c9e`](https://github.com/cercano-ai/homebrew-cercano/commit/0557c9ee7725f7f83832b30359381ea7e413216c).
The published definitions still carry fixture/not-release-ready caveats: publication
is not evidence of full runtime or old-monolith migration acceptance. No local
llama/model runtime is bundled.

For component-only installs, use `brew install cercano-ai/cercano/cercano-agent`
or `brew install cercano-ai/cercano/cercano-cli`. Neither component depends on
the other; CLI-only use needs a remote agent (`cercano-cli --address HOST:PORT`).
For a local interactive session, install both with the command above.

Commands installed:

| Binary | Role |
|---|---|
| `cercano` | the agent, which does the work |
| `cercano-cli` | the terminal client you interact with |

Start the terminal client:

```bash
cercano-cli
```

The client launches the agent automatically the first time you use it. You do
not need to start `cercano` yourself, and nothing is installed as a background
service — no launch agent, no daemon, nothing that runs at login. The agent
runs only when you use Cercano.

## First run

Cercano ships **no models and no credentials**. The binaries are self-contained,
but on first use you will need to configure at least one of:

- **A cloud provider** — an API key for your chosen provider.
- **A local model** — downloaded on demand, which needs disk space and time.

Run `cercano-cli` and follow the setup prompts. Configuration and conversation
history live in `~/.config/cercano/`, outside the Homebrew installation.

## Upgrading

For an existing **split-package** installation:

```bash
brew update
brew upgrade cercano-ai/cercano/cercano cercano-ai/cercano/cercano-agent cercano-ai/cercano/cercano-cli
```

The component names are explicit because the convenience formula owns no
binaries and its unversioned dependencies do not promise matching versions or
atomic upgrades. For component-only installs, upgrade only the components you
use. Check both installed command versions afterwards.

The v0.27.0 split formulas have **no post-install restart or service hooks**:
installation/upgrade does not start, stop, or restart an agent. A running agent
may still be the old version; save work and arrange a controlled restart before
expecting new behavior. Do not assume the old monolithic formula's automatic
restart behavior still applies.

### Existing monolithic installs

Before upgrading an older `cercano` formula that owns both binaries, inspect
`brew list --versions cercano cercano-agent cercano-cli`, `brew list cercano`,
and `which -a cercano cercano-cli`. The old keg can conflict with component
links during dependency-first upgrades. Migration is not yet documented as a
validated automatic path. Preserve your working installation and configuration;
if Homebrew reports conflicts, stop and seek release-specific migration guidance.
Do not force overwrites, remove packages, or delete files to bypass a conflict.
This guide does not prescribe an automatic migration.

## PATH conflicts with a development checkout

If you also run Cercano from a source checkout, you may have a development
launcher earlier in your `PATH` than Homebrew's. Check which one you are
running:

```bash
which -a cercano cercano-cli
```

Homebrew installs to `/opt/homebrew/bin`. If a development path appears first,
that copy wins, and `brew upgrade` will not change which binary you run.

The split formulas do not restart either installed or development agents.

## Troubleshooting

**"cercano" cannot be opened because the developer cannot be verified.**
Released binaries are signed with a Developer ID and notarized by Apple, so
this should not happen. If it does, you likely have an unsigned build from
another source. Check the binary path and release provenance before proceeding;
reinstalling the dependency-only `cercano` formula does not replace component binaries.

**The client cannot reach the agent.** Confirm nothing else is bound to the
configured port, then start a fresh agent by running `cercano-cli` again.

**The running version did not change.** Package upgrades and agent activation
are separate. Check command paths and arrange a controlled restart after saving work.

**Checking versions.** Both binaries report their own version offline:

```bash
cercano --version
cercano-cli --version
```

A client and agent from different versions can still talk to each other;
Cercano does not block on a version mismatch. Restarting the agent after an
upgrade is still worthwhile so you actually get the new version.

## Package ownership and user data

The convenience formula owns no commands; removing it alone does not mean the
component binaries were removed. Configuration and conversations live outside
Homebrew in `~/.config/cercano/`; downloaded models are separate too. Do not
remove user data as part of an upgrade or migration.
