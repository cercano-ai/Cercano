# Cercano

Cercano is a free, open-source AI coding agent and harness with a terminal
interface, built-in delegation, and configurable cloud and local inference.

**[Visit the Cercano website](https://cercano-ai.github.io/Cercano/)** for the
product overview and links to [installation](https://developer.cercloud.ai/getting-started/install/),
[first-run setup and usage](https://developer.cercloud.ai/getting-started/first-run/),
and user documentation. This README focuses on development and planning.

## Installation

- **macOS** - Requires Apple Silicon and macOS 12 or later. Install with [Homebrew](https://brew.sh/):

  ```bash
  brew install cercano-ai/cercano/cercano-agent cercano-ai/cercano/cercano-cli cercano-ai/cercano/cercano
  ```

- **First run** - Run `cercano-cli` and follow the setup prompts; it starts the local agent automatically.
- **Package details and upgrades** - See the [macOS installation guide](docs/installation-macos.md) for split-package readiness caveats and migration guidance before upgrading an older install.
- **Linux and Windows** - Remain experimental and unsigned. See the [Linux artifact guide](docs/linux-artifact.md) and [Windows installation guide](docs/installation-windows.md); Windows package approval and availability are not yet confirmed.
- **More help** - See the [installation guide](https://developer.cercloud.ai/getting-started/install/) and [first-run guide](https://developer.cercloud.ai/getting-started/first-run/).

## Development

- **[Developer guide](docs/agent/self-dev.md)** - Start here for source builds, tests, isolated development hosts, and exact reconnect guidance.
- **[Agent architecture](docs/agent/README.md)** - Understand the shared agent, configuration, permissions, and client/server responsibilities.
- **[CLI development](docs/features/cli/README.md)** - Find the current terminal interface, implementation status, and outstanding work.
- **[Contributor instructions](AGENTS.md)** - Follow repository-specific guidance when working on Cercano.

Build and test both Go modules from the repository root using the
[developer guide](docs/agent/self-dev.md#layout--build). Prefer its Makefile
builds for version metadata and conditional signing; module builds do not launch
an agent. Follow the guide's separate launcher and state-isolation instructions
before running a development host against real conversations.

## Architecture and repository map

The shared Go agent owns execution, model routing, permissions, and conversation
storage; the terminal UI is a separate Go module and gRPC client.

- **[Agent server](source/server/)** - Agent, gRPC server, MCP handlers, and shared packages.
- **[Terminal client](source/clients/cli/)** - Bubble Tea terminal UI and gRPC client.
- **[Inference interfaces](source/server/internal/engine/)** - Pluggable inference and embedding services.
- **[MCP tools](source/server/internal/mcp/)** - Cercano tool handlers; [web pipelines](source/server/internal/web/) handle fetching, search, and research.
- **[Tool skills](source/server/internal/skills/catalog/)** - Canonical skill instructions; see [contributor instructions](AGENTS.md) for generated mirrors and distribution copies.
- **[Website](website/README.md)** - Website development and deployment guidance.
- **[Documentation](docs/)** - Architecture, developer guides, specifications, plans, and research.
- **[Efforts](efforts/)** - Scoped specifications and execution plans, including the roadmap links below.

## Known issues

These are limitations and areas needing real-workflow feedback, not all
confirmed defects. Report reproducible examples rather than assuming roadmap
goals are available features.

- **Supervisory threads** - Real-workflow feedback is welcome, especially reproducible examples of unnecessary challenges or escalation.
- **Task pane** - Report reproducible bookkeeping inconsistencies between the task pane, plans, and conversation.
- **Platform validation** - Linux and Windows remain experimental; Windows artifacts are unsigned and native test coverage and real-workflow validation remain incomplete. See the [Linux artifact guide](docs/linux-artifact.md) and [Windows testing checklist](docs/windows-artifact.md); check the [Windows installation guide](docs/installation-windows.md) for package availability caveats.
- **Package migration** - Split Homebrew packages do not automatically restart agents, and migration from older monolithic installs needs care. See the [macOS installation guide](docs/installation-macos.md) before upgrading.
- **IDE integration** - ACP v1 stdio is implemented, but editor-specific UI validation remains. Session-history loading, editor-provided MCP servers, and ACP v2 are unsupported; see [ACP limitations](docs/acp.md).

## Roadmap

These are roadmap goals, not a list of currently available features.

### Host and development environments

- **Persistent server** - A long-running Cercano host in the spirit of dots/*claw,
  bringing integrations and remote access together beyond individual client sessions.
  - **Tool locus — Almost Done** - Select whether workspace tools run on the host
    or a connected client while keeping orchestration on the host. See the
    [execution plan](efforts/client-side-tool-execution/plan.md).
  - **Container skills** - Skills for creating, tearing down, and managing containers
    as isolated development spaces, with security dependent on configuration and
    permissions—not guaranteed by containers alone.

### Personal agent and orchestration

- **Personal Agent — Semi-prototyped** - Early exploration exists as
  [throwaway prototype code](https://github.com/cercano-ai/personal-assistant-old).
- **Orchestration** - Similar in concept to Gas City and other agent orchestration
  systems, with specialized organizational role agents such as a "director of
  engineering" or "executive assistant," tied closely to the personal agent architecture.

### Integrations

- **Computer use — Planned** - Shared desktop control for the CLI and future
  desktop client. See the [approved spec](efforts/cua-computer-use-integration/spec.md).
- **Remote control** - Remote access and control, including a mobile app.
- **Google apps** - Integration with Google's productivity apps.
- **Microsoft Office 365** - Integration with Microsoft's productivity suite.
- **Slack** - Messaging and workspace integration.
- **Texting** - Explore computer use and/or Twilio as possible integration paths.

### Desktop Client

- **Desktop client** - A ChatGPT-style desktop client written in Avalonia.

## Planning and contributing

- **[CLI track](docs/features/cli/README.md)** - Consult current implementation status and outstanding work alongside the roadmap above.
- **[Routing design](docs/cloud-routing.md)** - Understand task classes, model quality, destinations, and backups before changing execution policy.
- **[Report a problem](https://github.com/cercano-ai/Cercano/issues)** - Include version, platform, route/model, and reproduction steps; redact credentials and private content. Bug reports and pull requests are welcome.
- **[License](LICENSE)** - Cercano is open source under Apache 2.0.
