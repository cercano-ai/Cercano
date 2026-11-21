"use client";

import { useEffect, useState } from "react";

const projectUrl = "https://github.com/cercano-ai/Cercano";
const docsUrl = "https://developer.cercloud.ai";
const cercloudUrl = "https://cercloud.ai";
const basePath = process.env.NEXT_PUBLIC_BASE_PATH ?? "";
const assetUrl = (path: string) => `${basePath}${path}`;

type DetectedOs = "macos" | "windows" | "linux" | "other";

const homebrewCommand = "brew install cercano-ai/cercano/cercano-agent cercano-ai/cercano/cercano-cli cercano-ai/cercano/cercano";
const chocolateyCommand = "choco install cercano -y";

const capabilityGroups = [
  {
    label: "Models",
    items: [
      { title: "Any model", body: "Frontier, hosted open-weight, and local models in one agent.", href: `${docsUrl}/cercano/models/` },
      { title: "Existing subscriptions", body: "Use supported Codex and Claude subscriptions alongside API providers.", href: `${docsUrl}/cercano/models/` },
      { title: "Fully local", body: "Run model inference on your own machine with managed local runtimes.", href: `${docsUrl}/cercano/models/` },
    ],
  },
  {
    label: "Agents",
    items: [
      { title: "Subagents", body: "Delegate bounded work into separate contexts with scoped tools.", href: `${docsUrl}/cercano/delegation/` },
      { title: "Parallel work", body: "Keep the main conversation moving while independent tasks run.", href: `${docsUrl}/cercano/delegation/` },
      { title: "Built-in workflows", body: "Research, planning, debugging, verification, and design decisions.", href: `${docsUrl}/extend/` },
    ],
  },
  {
    label: "Context",
    items: [
      { title: "Background compaction", body: "Manage long-running context without stopping the agent to summarize.", href: `${docsUrl}/cercano/context/` },
      { title: "Saved sessions", body: "Search, name, resume, and continue work without reconstructing the task.", href: `${docsUrl}/cercano/context/` },
      { title: "Token metrics", body: "Inspect reported usage by provider, model, source, and time.", href: `${docsUrl}/cercano/metrics/` },
    ],
  },
  {
    label: "Extend",
    items: [
      { title: "MCP servers", body: "Connect local and remote MCP tools to the agent runtime.", href: `${docsUrl}/extend/` },
      { title: "Agent skills", body: "Package reusable knowledge and workflows for agents to load when needed.", href: `${docsUrl}/extend/` },
      { title: "Headless automation", body: "Use the same agent service from the terminal, scripts, and integrations.", href: `${docsUrl}/cercano/architecture/` },
    ],
  },
];

const valueBlocks = [
  {
    label: "Token efficiency",
    title: "Make your tokens go further.",
    body: "Smart routing sends difficult work to powerful models and bounded work to cheaper or local ones.",
    link: `${docsUrl}/cercano/routing/`,
    linkLabel: "Model routing",
    visual: "routing",
  },
  {
    label: "Continuous context",
    title: "Never stop for compaction.",
    body: "Cercano compacts context in the background so long jobs keep moving without a summarization pause.",
    link: `${docsUrl}/cercano/context/`,
    linkLabel: "Context management",
    visual: "context",
  },
  {
    label: "Multimodal work",
    title: "Use rich media models for specialized tasks.",
    body: "Cercano lets the agent call vision, image, audio, and speech models when the work requires them—without leaving the workflow.",
    link: `${docsUrl}/cercano/multimodal/`,
    linkLabel: "Images and audio",
    visual: "multimodal",
  },
];

export default function Home() {
  const [theme, setTheme] = useState<"day" | "night">("day");
  const [copiedCommand, setCopiedCommand] = useState<"homebrew" | "chocolatey" | null>(null);
  const [detectedOs, setDetectedOs] = useState<DetectedOs>("other");

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      const active = document.documentElement.dataset.theme === "night" ? "night" : "day";
      setTheme(active);
      const platform = `${window.navigator.platform ?? ""} ${window.navigator.userAgent ?? ""}`.toLowerCase();
      if (platform.includes("win")) setDetectedOs("windows");
      else if (platform.includes("mac")) setDetectedOs("macos");
      else if (platform.includes("linux")) setDetectedOs("linux");
      else setDetectedOs("other");
    });
    return () => window.cancelAnimationFrame(frame);
  }, []);

  const toggleTheme = () => {
    const next = theme === "night" ? "day" : "night";
    document.documentElement.dataset.theme = next;
    window.localStorage.setItem("cercano-theme", next);
    setTheme(next);
  };

  const copyInstallCommand = async (command: string, installer: "homebrew" | "chocolatey") => {
    await navigator.clipboard.writeText(command);
    setCopiedCommand(installer);
    window.setTimeout(() => setCopiedCommand(null), 1800);
  };

  const heroInstall = detectedOs === "macos"
    ? { label: "macOS detected · Homebrew", command: homebrewCommand, installer: "homebrew" as const }
    : detectedOs === "windows"
      ? { label: "Windows detected · Chocolatey", command: chocolateyCommand, installer: "chocolatey" as const }
      : detectedOs === "linux"
        ? { label: "Linux detected", command: null, installer: null }
        : { label: "Choose your operating system", command: null, installer: null };

  return (
    <div className="page-shell">
      <header className="site-header">
        <a className="brand" href="#top" aria-label="Cercano home">
          <img className="brand-mark" src={assetUrl("/icon.svg")} alt="" width="24" height="24" />
          <span>cercano</span>
        </a>
        <nav className="primary-nav" aria-label="Primary navigation">
          <a href="#features">Features</a>
          <a href="#value">Benefits</a>
          <a href="#install">Install</a>
          <a href={docsUrl}>Docs</a>
          <a href={projectUrl}>GitHub</a>
          <button
            className="theme-toggle"
            type="button"
            onClick={toggleTheme}
            aria-label={`Switch to ${theme === "night" ? "daylight" : "night"} theme`}
            aria-pressed={theme === "night"}
          >
            <span>{theme === "night" ? "Night" : "Day"}</span>
            <span className="theme-track" aria-hidden="true"><span /></span>
          </button>
        </nav>
      </header>

      <main id="top">
        <section className="hero" aria-labelledby="hero-title">
          <div className="hero-copy">
            <p className="eyebrow">Free forever · Apache 2.0 · runs on your machine</p>
            <h1 id="hero-title">The free and open source AI agent for any model.</h1>
          </div>
          <div className="hero-aside">
            <p className="lede">A fast, polished agent that works with frontier models, local models, and the subscriptions you already pay for.</p>
            <div className="hero-installer" aria-live="polite">
              <p>{heroInstall.label}</p>
              {heroInstall.command && heroInstall.installer ? (
                <div className="hero-command">
                  <code><span>{detectedOs === "windows" ? ">" : "$"}</span> {heroInstall.command}</code>
                  <button type="button" onClick={() => copyInstallCommand(heroInstall.command, heroInstall.installer)}>
                    {copiedCommand === heroInstall.installer ? "Copied" : "Copy"}
                  </button>
                </div>
              ) : (
                <a className="button button--primary" href={`${docsUrl}/getting-started/install/`}>{detectedOs === "linux" ? "View Linux installation" : "Choose an installer"}</a>
              )}
              <a className="other-installers" href={`${docsUrl}/getting-started/install/`}>Other OS installation instructions</a>
            </div>
          </div>
        </section>

        <figure className="hero-product-shot">
          <img src={assetUrl("/hero-tui.png")} alt="Cercano in the terminal describing the project, with cloud and local model status and a context meter visible" />
          <figcaption>Cercano in the terminal—model status, tool activity, working context, and the conversation in one place.</figcaption>
        </figure>

        <section className="capabilities" id="features" aria-labelledby="capabilities-title">
          <div className="capabilities-intro">
            <p className="section-label">What is Cercano?</p>
            <div>
              <h2 id="capabilities-title">A complete agent, not a model wrapper.</h2>
              <p>Cercano is a free and open source AI agent that runs on your machine. It combines model choice with the tools, workflows, context, and controls required for serious software work.</p>
            </div>
          </div>
          <div className="capability-console">
            {capabilityGroups.map((group) => (
              <div className="capability-row" key={group.label}>
                <div className="capability-row__label"><span>{group.label}</span></div>
                <div className="capability-row__items">
                  {group.items.map((capability) => (
                    <a href={capability.href} key={capability.title}>
                      <strong>{capability.title}</strong>
                      <span>{capability.body}</span>
                    </a>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </section>

        <section className="value-system" id="value" aria-labelledby="value-title">
          <div className="value-system__heading">
            <p className="section-label">Why Cercano</p>
            <h2 id="value-title">More capability. Less friction.</h2>
          </div>
          <div className="value-blocks">
            {valueBlocks.map((value) => (
              <article className="value-block" key={value.title}>
                <div className={`value-illustration value-illustration--${value.visual}`} aria-hidden="true">
                  {value.visual === "routing" && (
                    <svg viewBox="0 0 320 124">
                      <circle className="illustration-token" cx="20" cy="62" r="11" />
                      <circle className="illustration-token" cx="47" cy="62" r="11" />
                      <circle className="illustration-token" cx="74" cy="62" r="11" />
                      <path className="illustration-info" d="M20 56v12M47 56v12M74 56v12M85 62h25" />
                      <path className="illustration-selected" d="m110 62 27-27 27 27-27 27z" />
                      <circle className="illustration-primary-fill" cx="137" cy="62" r="5" />
                      <path className="illustration-primary" d="M164 62h20V18h20M184 62h20M184 62v44h20" />
                      <rect className="illustration-surface" x="204" y="6" width="108" height="24" />
                      <rect className="illustration-surface" x="204" y="50" width="108" height="24" />
                      <rect className="illustration-surface" x="204" y="94" width="108" height="24" />
                      <path className="illustration-muted" d="M218 18h47M218 62h47M218 106h47" />
                      <path className="illustration-success" d="m282 17 6 6 12-12M282 61l6 6 12-12M282 105l6 6 12-12" />
                    </svg>
                  )}
                  {value.visual === "context" && (
                    <svg viewBox="0 0 320 124">
                      <path className="illustration-primary" d="M8 99h296" />
                      <path className="illustration-primary-fill" d="m304 93 10 6-10 6z" />
                      <rect className="illustration-surface" x="12" y="22" width="44" height="62" />
                      <rect className="illustration-surface" x="64" y="22" width="44" height="62" />
                      <rect className="illustration-surface" x="116" y="22" width="44" height="62" />
                      <path className="illustration-muted" d="M22 36h24M22 47h18M22 58h24M74 36h24M74 47h18M74 58h24M126 36h24M126 47h18M126 58h24" />
                      <circle className="illustration-roller" cx="181" cy="46" r="15" />
                      <circle className="illustration-roller" cx="181" cy="82" r="15" />
                      <path className="illustration-primary" d="M196 64h26" />
                      <path className="illustration-primary-fill" d="m221 58 10 6-10 6z" />
                      <rect className="illustration-selected" x="237" y="36" width="68" height="56" />
                      <path className="illustration-primary" d="M248 49h46M248 60h37M248 71h42M248 82h28" />
                    </svg>
                  )}
                  {value.visual === "multimodal" && (
                    <svg viewBox="0 0 320 124">
                      <rect className="illustration-rack" x="7" y="14" width="306" height="96" rx="2" />
                      <rect className="illustration-cartridge illustration-cartridge--vision" x="21" y="29" width="78" height="58" />
                      <circle className="illustration-info" cx="60" cy="58" r="15" />
                      <circle className="illustration-info-fill" cx="60" cy="58" r="5" />
                      <rect className="illustration-cartridge illustration-cartridge--image" x="121" y="29" width="78" height="58" />
                      <circle className="illustration-primary-fill" cx="140" cy="45" r="4" />
                      <path className="illustration-primary" d="m132 74 18-17 12 10 10-8 16 15" />
                      <rect className="illustration-cartridge illustration-cartridge--audio" x="221" y="29" width="78" height="58" />
                      <path className="illustration-success" d="M234 64v-12M244 72V44M254 66V50M264 76V40M274 68V48M284 63V53" />
                      <path className="illustration-muted" d="M21 96h78M121 96h78M221 96h78" />
                      <circle className="illustration-info-fill" cx="60" cy="96" r="3.5" />
                      <circle className="illustration-primary-fill" cx="160" cy="96" r="3.5" />
                      <circle className="illustration-accent-fill" cx="260" cy="96" r="3.5" />
                    </svg>
                  )}
                </div>
                <p className="value-label">{value.label}</p>
                <h3>{value.title}</h3>
                <p className="value-summary">{value.body}</p>
                <a className="text-link" href={value.link}>{value.linkLabel}</a>
              </article>
            ))}
          </div>
        </section>

        <section className="control" aria-labelledby="control-title">
          <div>
            <p className="section-label">Fully local</p>
            <h2 id="control-title">Run a full-power agent entirely on your machine.</h2>
          </div>
          <div className="control-copy">
            <p>Keep prompts, context, and inference on your own machine when privacy matters. Cercano manages local models as first-class agent runtimes—with tools, delegation, sessions, and workflows—not as a separate chat window. You can still mix in hosted models when you choose.</p>
            <div className="control-options" aria-label="Model deployment choices">
              <span><i />Local</span><span><i />Hosted</span><span><i />Both</span>
            </div>
            <a className="text-link" href={`${docsUrl}/cercano/models/`}>Set up models</a>
          </div>
        </section>

        <section className="open-source" aria-labelledby="open-title">
          <p className="section-label">Free + open</p>
          <div>
            <h2 id="open-title">The agent is yours.</h2>
            <p>Cercano is not a limited free tier or a funnel into a paid coding agent. The complete agent is free and open source under Apache 2.0. Run it, inspect it, extend it, and make it your own.</p>
            <div className="open-actions">
              <a className="button button--primary" href={projectUrl}>Explore the source</a>
              <a className="button" href={`${projectUrl}/blob/main/LICENSE`}>Read the license</a>
            </div>
          </div>
        </section>

        <section className="install" id="install" aria-labelledby="install-title">
          <div className="install-copy">
            <div><p className="section-label">Try Cercano</p><h2 id="install-title">Install in one line.</h2></div>
            <p>Choose your package manager, launch Cercano, and connect the model source you want. The full setup and troubleshooting guides live in the developer documentation.</p>
          </div>
          <div className="install-panel" aria-label="Cercano installation options">
            <div className="installer-grid">
              <article className="installer-option">
                <div className="installer-heading"><span>Homebrew</span><span className="availability">Available</span></div>
                <p className="installer-platform">macOS</p>
                <div className="install-command">
                  <code><span>$</span> brew install cercano-ai/cercano/cercano-agent cercano-ai/cercano/cercano-cli cercano-ai/cercano/cercano</code>
                  <button type="button" onClick={() => copyInstallCommand("brew install cercano-ai/cercano/cercano-agent cercano-ai/cercano/cercano-cli cercano-ai/cercano/cercano", "homebrew")} aria-label="Copy Homebrew install command">{copiedCommand === "homebrew" ? "Copied" : "Copy"}</button>
                </div>
              </article>
              <article className="installer-option">
                <div className="installer-heading"><span>Chocolatey</span><span className="availability">Available</span></div>
                <p className="installer-platform">Windows</p>
                <div className="install-command">
                  <code><span>&gt;</span> choco install cercano -y</code>
                  <button type="button" onClick={() => copyInstallCommand("choco install cercano -y", "chocolatey")} aria-label="Copy Chocolatey install command">{copiedCommand === "chocolatey" ? "Copied" : "Copy"}</button>
                </div>
              </article>
              <article className="installer-option installer-option--pending">
                <div className="installer-heading"><span>apt</span><span className="availability availability--pending">Coming soon</span></div>
                <p className="installer-platform">Debian + Ubuntu</p>
                <div className="install-pending">Package publishing in progress</div>
              </article>
            </div>
            <p className="after-install">Then run <code>cercano-cli</code>.</p>
            <div className="install-links">
              <a href={`${docsUrl}/getting-started/install/`}>Installation guide</a>
              <a href={`${docsUrl}/getting-started/first-run/`}>First run</a>
              <a href={`${docsUrl}/troubleshooting/`}>Troubleshooting</a>
              <a href={`${projectUrl}/releases`}>Releases</a>
            </div>
          </div>
        </section>

        <section className="cercloud" aria-labelledby="cercloud-title">
          <div><p className="section-label">Cercano for organizations</p><h2 id="cercloud-title">Love Cercano?<br />Give it to the whole team.</h2></div>
          <div className="cercloud-copy">
            <p>Cercano gives every developer a powerful local agent. Cercloud turns that capability into an organizational platform with shared context, persistent agents, identity, policy, cost controls, auditability, and compliance.</p>
            <a className="button button--primary" href={cercloudUrl}>Explore Cercloud</a>
          </div>
        </section>
      </main>

      <footer><a href={projectUrl}>Cercano — free forever + open source</a><a href={docsUrl}>Developer documentation</a></footer>
    </div>
  );
}
