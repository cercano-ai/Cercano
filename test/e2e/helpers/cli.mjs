// withCli wires one spec end to end: sandbox, optional agent and fake Ollama,
// and cercano-cli running in a tui-test PTY. Teardown always runs; on failure
// it keeps a tui-test trace under artifacts/ and prints the agent log.
//
// before(sandbox) runs before the agent and CLI start, to seed files. fn also
// gets defer(cleanup): deferred cleanups run after the CLI is closed and
// before the sandbox is removed, so stopping an auto-launched agent there
// neither races a client that would relaunch it nor lets it write into a
// removed sandbox.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { TuiTest } from "@microsoft/tui-test";
import { createSandbox } from "./sandbox.mjs";
import { startAgent } from "./agent.mjs";
import { startFakeOllama } from "./fake-ollama.mjs";
import { ARTIFACTS_DIR, CLI_BIN } from "./paths.mjs";

function readText(path) {
  try {
    return readFileSync(path, "utf8");
  } catch {
    return "<none>";
  }
}

export const PLACEHOLDER = "type a message, /help for commands";

// newTerm creates a tui-test terminal that keeps a trace and the failing
// screen under artifacts/ when the test fails.
export function newTerm(name) {
  return TuiTest.ephemeral(`cercano-${name}`, {
    timeouts: { text: 15_000 },
    trace: { mode: "on-failure", directory: join(ARTIFACTS_DIR, "traces") },
    artifacts: { dir: join(ARTIFACTS_DIR, "failures", name), onFailure: "text" },
  });
}

// runCli starts another cercano-cli in sandbox, beside the one withCli runs.
// The caller closes the returned terminal.
export async function runCli(t, sandbox, { label, args = [], cols = 120, rows = 40, bin = CLI_BIN } = {}) {
  const term = newTerm(`${t.name.replace(/[^\w-]+/g, "_")}-${label}`);
  await term.run(bin, args, { cols, rows, env: sandbox.env, cwd: sandbox.root });
  return term;
}

export async function withCli(
  t,
  { args = [], agent = true, seedConfig = true, ollama = false, cols = 120, rows = 40, bin = CLI_BIN, env = {}, profile, before } = {},
  fn,
) {
  const fake = ollama ? await startFakeOllama(typeof ollama === "object" ? ollama : {}) : null;
  const sandbox = await createSandbox({ seedConfig, ollamaUrl: fake?.url, profile });
  Object.assign(sandbox.env, env);
  const term = newTerm(t.name.replace(/[^\w-]+/g, "_"));

  let server = null;
  let failed = false;
  const deferred = [];
  const defer = (cleanup) => deferred.push(cleanup);
  try {
    await before?.(sandbox);
    if (agent) server = await startAgent(sandbox);
    await term.run(bin, args, { cols, rows, env: sandbox.env, cwd: sandbox.root });
    return await fn({ term, sandbox, agent: server, ollama: fake, defer });
  } catch (err) {
    failed = true;
    const screen = await term.text().catch(() => "<no screen>");
    t.diagnostic(`screen at failure:\n${screen}`);
    if (server) t.diagnostic(`agent log:\n${server.log()}`);
    else t.diagnostic(`auto-launched agent log:\n${readText(join(sandbox.profileDir(), "logs", "agent.log"))}`);
    if (fake) t.diagnostic(`fake ollama requests:\n${JSON.stringify(fake.requests.map((r) => r.path))}`);
    throw err;
  } finally {
    await term.close({ failed }).catch(() => {});
    let cleanupError;
    for (const cleanup of deferred.reverse()) {
      try {
        await cleanup();
      } catch (err) {
        cleanupError ??= err;
      }
    }
    await server?.stop();
    await fake?.close();
    sandbox.cleanup();
    if (cleanupError && !failed) throw cleanupError;
  }
}

// exitCode waits for the program to exit and returns its status.
export async function exitCode(term, timeout = 10_000) {
  await term.waitExit({ timeout });
  return (await term.state()).exited;
}

// ready waits until the chat prompt is idle and accepting input.
export async function ready(term) {
  await term.getByText(PLACEHOLDER).expect();
}
