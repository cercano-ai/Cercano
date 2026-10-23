// Per-test isolation: a throwaway HOME/XDG_CONFIG_HOME/TMPDIR, so a test never
// reads the developer's ~/.config/cercano or attaches to the real agent on
// :50052.
import { mkdtempSync, mkdirSync, rmSync, writeFileSync, realpathSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createServer } from "node:net";

export const DEFAULT_PROFILE = "default";

export async function freePort() {
  return new Promise((resolve, reject) => {
    const srv = createServer();
    srv.unref();
    srv.on("error", reject);
    srv.listen(0, "127.0.0.1", () => {
      const { port } = srv.address();
      srv.close(() => resolve(port));
    });
  });
}

// createSandbox returns the directories and env for one test. Unless
// seedConfig is false it writes the profile's config.yaml so the CLI skips the
// setup wizard; the seeded config runs open_only against Ollama at ollamaUrl
// (a fake server, or an unused port when the test never chats).
//
// Without profile the sandbox serves the default profile on a free fixed
// CERCANO_PORT. With profile it selects that profile through CERCANO_PROFILE
// and sets no port, so each profile's agent takes an ephemeral port and
// publishes it in <profile>/agent.json, and several profiles can run in one
// sandbox.
export async function createSandbox({ seedConfig = true, ollamaUrl, profile } = {}) {
  // The agent puts worker Unix sockets under $TMPDIR/cercano-workers/, and
  // macOS caps socket paths at 104 bytes; its default /var/folders/... tmpdir
  // nested under a sandbox overflows that. Use the short /tmp on Unix.
  const base = process.platform === "win32" ? tmpdir() : "/tmp";
  // realpath: on macOS /tmp is a symlink to /private/tmp.
  const root = realpathSync(mkdtempSync(join(base, "ce2e-")));
  const home = join(root, "home");
  const configDir = join(home, ".config", "cercano");
  const profilesDir = join(configDir, "profiles");
  const tmp = join(root, "t");
  mkdirSync(configDir, { recursive: true });
  mkdirSync(tmp, { recursive: true });

  const port = profile ? null : await freePort();
  const ollama = ollamaUrl ?? `http://127.0.0.1:${await freePort()}`;
  const profileDir = (name = profile ?? DEFAULT_PROFILE) => join(profilesDir, name);
  const configPath = join(profileDir(), "config.yaml");

  // tui-test merges this over the inherited environment, so anything that
  // could leak the developer's setup is overridden explicitly. Empty values
  // read as unset to the Go side. Conversations and wizard state follow the
  // active profile's directory under the sandbox HOME.
  const env = {
    HOME: home,
    XDG_CONFIG_HOME: join(home, ".config"),
    TMPDIR: tmp,
    TERM: "xterm-256color",
    CERCANO_PROFILE: profile ?? "",
    CERCANO_PORT: port ? String(port) : "",
    // Local models stay in the agent process: no shared model host is
    // launched, so none outlives the test.
    CERCANO_MODEL_HOST: "inprocess",
    OLLAMA_URL: ollama,
    CERCANO_OPEN_RUNTIME: "ollama",
    CERCANO_OPEN_MODEL: "fake-model",
    CERCANO_CONVERSATIONS_DB: "",
    CERCANO_WIZARD_STATE: "",
    CERCANO_UI_CONFIG: join(configDir, "ui.yaml"),
    CERCANO_COLORPROBE_LOG: join(root, "colorprobe.log"),
    CERCANO_LOCAL_RUNTIME: "",
    CERCANO_LOCAL_MODEL: "",
    CERCANO_EMBEDDING_MODEL: "",
    GEMINI_API_KEY: "",
    ANTHROPIC_API_KEY: "",
    OPENAI_API_KEY: "",
  };

  const sandbox = {
    root,
    home,
    configDir,
    profilesDir,
    profileDir,
    configPath,
    tmp,
    port,
    profile: profile ?? DEFAULT_PROFILE,
    ollamaUrl: ollama,
    env,
    // writeConfig seeds config.yaml for a profile (the sandbox's own unless
    // profile names another), or at the pre-profile location
    // ~/.config/cercano/config.yaml when legacy is true.
    writeConfig(extra = "", { profile: name, legacy = false } = {}) {
      const dir = legacy ? configDir : profileDir(name);
      mkdirSync(dir, { recursive: true });
      const lines = ["locus_mode: open_only", "open_runtime: ollama", `ollama_url: ${ollama}`];
      if (port) lines.push(`port: "${port}"`);
      lines.push(extra);
      writeFileSync(join(dir, "config.yaml"), lines.join("\n") + "\n");
    },
    cleanup() {
      if (!process.env.E2E_KEEP_SANDBOX) rmSync(root, { recursive: true, force: true });
    },
  };
  if (seedConfig) sandbox.writeConfig();
  return sandbox;
}
