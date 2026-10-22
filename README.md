# Cercano

**Cercano is a 100% free + open, premium agent/harness that not only has a premium 
UX, but is designed to use context and compute deliberately.**

**Frontier + open, separately or together.** Use a frontier provider through an
available subscription sign-in or API credentials, run open-weight models, or
combine them in one terminal workflow. Keep demanding reasoning in the main
conversation and delegate bounded work to appropriately priced models, on your
hardware or with a hosted provider. Subscription eligibility and limits depend
on the provider; see [provider options](docs/agent/features/openai-providers.md).

Built-in workflows guide **research**, **planning**, **sub-agent execution**, 
**debugging**, and **design decisions**. Background context management and 
compaction and a polished terminal interface help you stay focused as sessions 
grow, with subagent tabs keeping delegated work visible without mixing every 
intermediate step into the main conversation.

## What makes Cercano different

### The right model for the job

**Don't spend frontier compute on every task.** Route demanding reasoning,
routine searches, and mechanical edits independently. Choose quality and
execution destination separately, mixing frontier and open-weight models to
match the work—not forcing every task through the same model.

[Explore the routing engine →](docs/agent/features/advanced-routing.md)

### Delegate the work. Keep the focus.

**Give investigations their own workspace, not your entire context window.**
Built-in subagents work with separate context and scoped tools, then return
focused results. Follow their progress in dedicated tabs while keeping the main
conversation centered on decisions and the context clean.

Watch a delegated investigation in its own subagent tab:

https://github.com/user-attachments/assets/0656f5b1-a0bd-4faf-a0b3-50db4ea43cbf

[Explore built-in delegation →](docs/agent/features/built-in-delegation.md)

### A process, not just a prompt

**Spend less time telling your agent how to work.** Built-in workflows guide
research, planning, execution, debugging, and design decisions. Reproduce the
failure before fixing it. Compare real alternatives before choosing one. Verify
the result instead of stopping at plausible output.

[Explore agent workflows →](docs/agent/features/powerful-agent-workflows.md)

### Keep working. Let context catch up.

**Long sessions shouldn't mean carrying every old token forward.** Background
compaction turns older conversation into layered summaries while retaining
recent working context. Make room for current evidence without stopping for a
summarizer—with stored history kept separate from lossy summaries.

[Explore context management →](docs/agent/features/context-management.md)

### Know where your tokens go

**Make model choices with numbers, not guesses.** Break token usage down by
provider, model, and source. See what's consuming your budget and use that
visibility to tune your routes; estimated savings stay labeled as estimates.

See the token metrics dashboard in action:

https://github.com/user-attachments/assets/82c65acd-6b7f-4d6e-8a56-dbe0d3133d8c

[Explore token metrics →](docs/agent/features/advanced-metrics.md)

### Local models, less assembly required

**Use your hardware without building an inference stack first.** Cercano's
managed local model runtime brings open-weight models into the same workflow as your
cloud providers. Configure a suitable model and let Cercano manage the runtime.

[Explore the integrated runtime →](docs/agent/features/integrated-local-runtime.md)

### Your workflow. Your choice of provider.

**Switch models without switching coding agents.** Connect compatible hosted
services or self-hosted endpoints, and combine them with frontier providers.
Keep your sessions, tools, and delegation workflow while choosing where the
work runs. Subscription sign-in and API access have provider-specific limits.

[Explore provider options →](docs/agent/features/openai-providers.md)

### A terminal that keeps the work readable

**Less log-diving. More clarity.** Rich formatting, subagent tabs, a live context
meter, and responsive layouts organize dense agent activity. Model settings,
routing, and permissions live in the same interface—and themes make it yours.

Take a quick tour of configuration and theming:

https://github.com/user-attachments/assets/89989b0f-91a1-4fa6-aca0-98ce12a446c6

[Explore the terminal UI →](docs/agent/features/advanced-terminal-ui.md)

### Pick up where you left off

**Your last investigation shouldn't disappear when you close the terminal.**
Conversations are saved automatically, with titles, history, search, and resume
helping you find prior work and continue without reconstructing the discussion.

Watch how to resume a saved conversation:

https://github.com/user-attachments/assets/d4a81e65-0206-475c-994b-067174f3a5f2

[Explore session retention →](docs/agent/features/automatic-session-retention.md)

### One agent. Room for any client.

**The agent isn't tied to the interface.** Cercano's independent agent server
handles execution, models, permissions, and saved conversations; clients provide
the experience. Today, that means the terminal UI and headless commands. The same
architecture opens the door to IDE integrations, full desktop applications, and
other clients—without rebuilding the agent for each one. Those possibilities
are largely unbuilt, but the separation is here today.

[Explore the client/server architecture →](docs/agent/features/client-server-architecture.md)

### Free and open. Yours to build on.

**A premium agent experience, without a software subscription.** Cercano is free
and open source under the Apache 2.0 license. Inspect how it works, change it to
fit your workflow, or build something new on top of it. The software is free;
cloud inference, provider subscriptions, and the hardware you run it on can
still carry costs.

[Read the license →](LICENSE) · [Build and contribute →](docs/agent/self-dev.md)

## Get started

### macOS (supported)

**macOS 12 (Monterey) or later, Apple Silicon:** install with
[Homebrew](https://brew.sh) and launch the terminal client:

```bash
brew install cercano-ai/cercano/cercano
cercano-cli
```

The client starts the agent (`cercano`) automatically; you do not need to start
it separately. Intel Macs are not supported by this release.

To upgrade:

```bash
brew upgrade cercano
```

See the [macOS installation guide](docs/installation-macos.md) for first-run
setup, upgrade troubleshooting, and development-launcher PATH conflicts.

### Linux x86_64 (experimental, unsigned)

Download and unpack the published release, verify its checksum, then launch
from the extracted directory (requires `curl`, `sha256sum`, and `tar`):

```bash
version=0.24.0
archive="cercano-$version-linux-x64.tar.gz"
url="https://github.com/cercano-ai/Cercano/releases/download/v$version"
curl -fLO "$url/$archive" &&
curl -fLO "$url/$archive.sha256" &&
sha256sum -c "$archive.sha256" &&
tar -xzf "$archive" &&
"./cercano-$version-linux-x64/bin/cercano-cli"
```

See the [Linux artifact guide](docs/linux-artifact.md) for limitations,
including credential storage requiring a desktop Secret Service/KWallet.

### Windows x64 (experimental, unsigned)

In PowerShell, download, verify, extract, and launch the published release:

```powershell
$ErrorActionPreference = 'Stop'
$version = '0.24.0'
$archive = "cercano-$version-windows-x64.zip"
$url = "https://github.com/cercano-ai/Cercano/releases/download/v$version"
Invoke-WebRequest "$url/$archive" -OutFile $archive
Invoke-WebRequest "$url/$archive.sha256" -OutFile "$archive.sha256"
$expected = ((Get-Content "$archive.sha256" -Raw).Trim() -split '\s+')[0]
if ((Get-FileHash $archive -Algorithm SHA256).Hash -ne $expected) { throw 'Checksum mismatch' }
Expand-Archive -Path $archive -DestinationPath .
& ".\cercano-$version-windows-x64\bin\cercano-cli.exe"
```

Windows may show an unknown-publisher warning. See the
[Windows artifact guide](docs/windows-artifact.md) for limitations and testing.

These examples use release **0.24.0** and leave both binaries together in the
extracted `bin` directory; no administrator access or PATH changes are needed.
Linux and Windows remain **experimental, not supported releases**. The current
website lists apt and Chocolatey as coming soon, not available install methods.
Checksums verify download integrity, not publisher identity.

### First run

The terminal client starts the agent automatically on all three platforms.
For development, use the [source build instructions](docs/agent/self-dev.md#layout--build).

Open `/config` to configure models, routing, and permissions. Then try a bounded,
read-only task in a repository you know:

> Find where this project loads its configuration. Delegate the code search to
> a read-only subagent, then explain the result with file references. Do not edit files.

[GitHub Releases](https://github.com/cercano-ai/Cercano/releases) provides the
archives and checksums above, plus a signed and notarized macOS Apple Silicon
tarball.

For scripts and automation (on Linux/Windows, use the extracted
`bin/cercano` or `bin\cercano.exe` path instead of the bare command):

```bash
cercano run "Explain this repository's top-level structure without editing files"
```

Cloud routes require the relevant provider credentials. Local routes require
suitable hardware and model downloads. Model quality, latency, and cost depend
on your configuration; delegation is not a guarantee of savings. See each
feature page for controls and limitations.

## Known issues

Looking for a place to contribute? These are great areas to jump in. Some already
have substantial foundations but need testing, tuning, or user-experience polish.
Bug reports, reproducible examples, design feedback, and pull requests are all
welcome. See the [developer guide](docs/agent/self-dev.md) to get started, or
[open an issue](https://github.com/cercano-ai/Cercano/issues) to discuss an idea.

- **Supervisory threads:** The architecture and much of the implementation are
  in place, but the behavior needs polish. Supervisors currently intervene too
  aggressively; tuning when and how they step in would make them more useful.
  See the [watchdog design](docs/features/agent-capabilities/watchdog/design.md)
  and [polish plan](docs/features/agent-capabilities/watchdog/polish-plan.md).
- **Task pane:** Most of the implementation is in place, including a development
  sandbox, but expanding and collapsing tasks needs to be more reliable. Mouse
  events should go to the individual controls rather than the entire panel, and
  task tracking needs better bookkeeping. Longer term, the pane should support
  conversations and edits that stay synchronized with plans and the main thread.
  See the [interaction-fix spec](efforts/fix-task-panel-scrollbars/spec.md),
  [debug sandbox plan](efforts/debug-task-pane-controls/plan.md), and
  [task-model design](docs/features/agent-capabilities/task-model/design.md).
- **Skills:** Cercano supports built-in workflow protocols via `get_protocol`
  and organization-assigned text skills via `get_shared_skill`. General discovery
  and loading of user or third-party `SKILL.md` packages is not yet supported;
  skills sync only exports skills for other agents.
- **Windows support:** Windows remains under-tested. Testing real workflows,
  reporting reproducible platform-specific bugs, and improving compatibility
  are especially valuable contributions. See the
  [Windows artifact and testing checklist](docs/windows-artifact.md).
- **IDE plugins:** An older VS Code extension draft needs to be reworked for
  Cercano's current full agent architecture. Infrastructure for a Zed integration
  has also been drafted, but the integration itself is not implemented.
  One approach to evaluate is implementing Agent Client Protocol (ACP) to connect
  compatible editors to Cercano's existing agent. This could cover much of IDE
  integration and reduce bespoke plugin work, depending on each editor's ACP
  support and capabilities.
  Background: [earlier VS Code integration fixes](docs/internal/ide-fixes.md)
  and [IDE technology stack / Zed scaffold](docs/reference/tech-stack.md#3-ide-integration).

## Documentation and support

- [Feature guide](docs/agent/features/README.md): what each differentiator does and why it matters.
- [Agent guide](docs/agent/README.md): setup, commands, permissions, and architecture.
- [Routing guide](docs/cloud-routing.md): task classes, model quality, destinations, and backups.
- [CLI track](docs/features/cli/README.md): implementation status and outstanding work.
- [Developer guide](docs/agent/self-dev.md): building, testing, and working on Cercano.
- [Report a problem](https://github.com/cercano-ai/Cercano/issues): include your version, platform, route/model, and reproduction steps; redact credentials and private content.