// Helpers for specs of the CERCANO_PER_PROFILE_AGENTS feature flag: run the
// real binaries with the flag absent, off or on, and check that each process
// really got that state.
//
// createSandbox sets the flag to "" for default-profile sandboxes and "1" for
// named-profile ones; these helpers override that per process instead. "Absent"
// means the variable is not in the environment at all: tui-test can only add
// variables over the inherited environment, so importing this module removes
// the flag from this test process's own environment.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { closeSync, mkdirSync, openSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { existsSync } from "node:fs";
import { AGENT_BIN, CLI_BIN, E2E_DIR } from "./paths.mjs";
import { readAgentRecord, portOpen } from "./agent.mjs";
import { processEnv, reapSandbox, sandboxProcesses } from "./procs.mjs";
import { createSandbox } from "./sandbox.mjs";
import { startFakeOllama } from "./fake-ollama.mjs";
import { runCli, exitCode } from "./cli.mjs";

export const FLAG = "CERCANO_PER_PROFILE_AGENTS";

delete process.env[FLAG];

// Values the flag is documented to read as off (unset, or strconv.ParseBool
// false) and on.
export const FLAG_OFF = [
  { label: "absent", value: undefined },
  { label: '"false"', value: "false" },
  { label: '"0"', value: "0" },
];
export const FLAG_ON = [
  { label: '"1"', value: "1" },
  { label: '"true"', value: "true" },
];

// withFlag returns env with the flag set to value, or without it when value
// is undefined.
export function withFlag(env, value) {
  const out = { ...env };
  if (value === undefined) delete out[FLAG];
  else out[FLAG] = value;
  return out;
}

// assertFlagState checks that pid runs with the flag exactly as intended.
export function assertFlagState(pid, value, what = `process ${pid}`) {
  const env = processEnv(pid);
  assert.ok(env, `cannot read the environment of ${what}`);
  if (value === undefined) assert.equal(env.has(FLAG), false, `${what} should run without ${FLAG}, has ${FLAG}=${env.get(FLAG)}`);
  else assert.equal(env.get(FLAG), value, `${what} should run with ${FLAG}=${value}`);
}

// blockExternalNetwork returns env that keeps the binaries off the network:
// Go never proxies loopback, so the fake Ollama still answers, while any
// external HTTP(S) request (MCP mode's release check) goes to a dead proxy.
export function blockExternalNetwork(env) {
  return { ...env, HTTPS_PROXY: "http://127.0.0.1:9", HTTP_PROXY: "http://127.0.0.1:9", NO_PROXY: "", https_proxy: "http://127.0.0.1:9", http_proxy: "http://127.0.0.1:9", no_proxy: "" };
}

// seedUpdateCache writes a fresh release-check result where MCP mode looks
// for it, so `cercano --mcp` does not ask GitHub for the latest release.
export function seedUpdateCache(sandbox) {
  mkdirSync(sandbox.configDir, { recursive: true });
  writeFileSync(
    join(sandbox.configDir, "update_check.json"),
    JSON.stringify({ latest_version: "0.0.0", checked_at: new Date().toISOString(), release_url: "", install_method: "" }),
  );
}

// spawnCercano starts bin with args in its own process group, logging to
// logPath, and returns a handle with exit tracking. The spec owns the child.
export function spawnCercano(sandbox, args, { env, bin = AGENT_BIN, logName = "cercano" } = {}) {
  const logPath = join(sandbox.root, `${logName}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}.log`);
  const fd = openSync(logPath, "a");
  const child = spawn(bin, args, {
    env: blockExternalNetwork(env ?? { ...process.env, ...sandbox.env }),
    stdio: ["ignore", fd, fd],
    detached: true,
    cwd: sandbox.root,
  });
  closeSync(fd);
  let exited = null;
  const exit = new Promise((resolve) => child.once("exit", (code, signal) => resolve((exited = { code, signal }))));
  const log = () => {
    try {
      return readFileSync(logPath, "utf8");
    } catch {
      return "";
    }
  };
  return {
    child,
    pid: child.pid,
    logPath,
    log,
    exit,
    exited: () => exited,
    async waitExit(timeout = 30_000) {
      let timer;
      const timedOut = new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error(`${args.join(" ")} did not exit within ${timeout}ms:\n${log()}`)), timeout);
      });
      try {
        return await Promise.race([exit, timedOut]);
      } finally {
        clearTimeout(timer);
      }
    },
    async stop(timeout = 10_000) {
      if (exited) return exited;
      try {
        process.kill(-child.pid, "SIGTERM");
      } catch {
        // already gone
      }
      const timer = setTimeout(() => {
        try {
          process.kill(-child.pid, "SIGKILL");
        } catch {
          // already gone
        }
      }, timeout);
      const result = await exit;
      clearTimeout(timer);
      return result;
    },
  };
}

// startProfileAgent runs `cercano agent` for profile with the flag in the
// given state and waits until its agent.json names it and its port answers.
// A named profile's agent gets no CERCANO_PORT (so it takes an ephemeral
// port, as it would in use) unless extraEnv sets one.
export async function startProfileAgent(sandbox, { profile = sandbox.profile, flag, bin = AGENT_BIN, extraEnv = {}, timeout = 20_000 } = {}) {
  const named = profile !== "default";
  const env = withFlag(
    { ...process.env, ...sandbox.env, CERCANO_PROFILE: named ? profile : "", ...(named ? { CERCANO_PORT: "" } : {}), ...extraEnv },
    flag,
  );
  const agent = spawnCercano(sandbox, ["agent"], { env, bin, logName: `agent-${profile}` });
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if (agent.exited()) throw new Error(`agent for ${profile} exited ${JSON.stringify(agent.exited())}:\n${agent.log()}`);
    const rec = readAgentRecord(sandbox, profile);
    if (rec?.pid === agent.pid) {
      const port = Number(rec.addr.split(":").at(-1));
      if (await portOpen(port)) return { ...agent, record: rec, port };
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  await agent.stop(1_000);
  throw new Error(`agent for ${profile} did not register within ${timeout}ms:\n${agent.log()}`);
}

// startMcp runs `cercano --mcp` (plus args) and speaks newline-delimited
// JSON-RPC to it over stdio. Every stdout line is kept; protocolViolations
// lists those that are not JSON-RPC 2.0 messages, which would corrupt a
// client's framing.
export function startMcp(sandbox, { env, args = [], bin = AGENT_BIN } = {}) {
  const stderrPath = join(sandbox.root, `mcp-${Date.now()}-${Math.random().toString(36).slice(2, 8)}.stderr`);
  const errFd = openSync(stderrPath, "a");
  const child = spawn(bin, ["--mcp", ...args], {
    env: blockExternalNetwork(env ?? { ...process.env, ...sandbox.env }),
    stdio: ["pipe", "pipe", errFd],
    detached: true,
    cwd: sandbox.root,
  });
  closeSync(errFd);
  let exited = null;
  const exit = new Promise((resolve) => child.once("exit", (code, signal) => resolve((exited = { code, signal }))));
  const lines = [];
  const pending = new Map();
  let buffer = "";
  child.stdout.setEncoding("utf8");
  child.stdout.on("data", (chunk) => {
    buffer += chunk;
    let i;
    while ((i = buffer.indexOf("\n")) >= 0) {
      const line = buffer.slice(0, i);
      buffer = buffer.slice(i + 1);
      lines.push(line);
      let msg;
      try {
        msg = JSON.parse(line);
      } catch {
        continue;
      }
      if (msg && msg.id !== undefined && pending.has(msg.id)) {
        pending.get(msg.id)(msg);
        pending.delete(msg.id);
      }
    }
  });
  let nextId = 1;
  const stderr = () => {
    try {
      return readFileSync(stderrPath, "utf8");
    } catch {
      return "";
    }
  };
  const request = (method, params, timeout = 30_000) => {
    const id = nextId++;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        pending.delete(id);
        reject(new Error(`MCP ${method} got no response within ${timeout}ms; stdout:\n${lines.join("\n")}\nstderr:\n${stderr()}`));
      }, timeout);
      pending.set(id, (msg) => {
        clearTimeout(timer);
        resolve(msg);
      });
      child.stdin.write(JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n");
    });
  };
  const notify = (method, params) => child.stdin.write(JSON.stringify({ jsonrpc: "2.0", method, ...(params ? { params } : {}) }) + "\n");
  return {
    pid: child.pid,
    lines,
    stderr,
    exited: () => exited,
    async initialize() {
      const res = await request("initialize", {
        protocolVersion: "2025-06-18",
        capabilities: {},
        clientInfo: { name: "cercano-e2e", version: "0" },
      });
      assert.ok(res.result, `initialize failed: ${JSON.stringify(res)}\nstderr:\n${stderr()}`);
      notify("notifications/initialized");
      return res.result;
    },
    async tools() {
      const res = await request("tools/list", {});
      assert.ok(res.result, `tools/list failed: ${JSON.stringify(res)}`);
      return res.result.tools.map((t) => t.name);
    },
    // call returns { isError, text } of a tools/call.
    async call(name, args = {}, timeout) {
      const res = await request("tools/call", { name, arguments: args }, timeout);
      if (res.error) return { isError: true, text: res.error.message ?? JSON.stringify(res.error), rpcError: res.error };
      const text = (res.result?.content ?? []).map((c) => c.text ?? "").join("\n");
      return { isError: Boolean(res.result?.isError), text };
    },
    protocolViolations() {
      return lines.filter((line) => {
        try {
          return JSON.parse(line)?.jsonrpc !== "2.0";
        } catch {
          return true;
        }
      });
    },
    // close ends stdin, as an MCP host does, and waits for the server to exit;
    // it is killed (its whole process group) only if it does not, and the
    // result then has forced set.
    async close(timeout = 15_000) {
      if (exited) return exited;
      child.stdin.end();
      let forced = false;
      const timer = setTimeout(() => {
        forced = true;
        try {
          process.kill(-child.pid, "SIGKILL");
        } catch {
          // already gone
        }
      }, timeout);
      const result = await exit;
      clearTimeout(timer);
      return { ...result, forced };
    },
  };
}

// REFUSAL matches the error every entry point gives a named profile while the
// flag is off.
export const REFUSAL = /per-profile agent hosts are (off|disabled).*CERCANO_PER_PROFILE_AGENTS=1/s;

// historicalBin returns the cercano (or cercano-cli) built from rev by
// scripts/build-historical.sh, and a skip reason when it was not built. With
// E2E_REQUIRE_HISTORICAL=1 a missing build fails instead of skipping.
export function historicalBin(rev, name = "cercano") {
  const path = join(E2E_DIR, ".bin", rev, name);
  if (existsSync(path)) return { path, skip: false };
  const reason = `${path} is missing; run scripts/build-historical.sh`;
  if (process.env.E2E_REQUIRE_HISTORICAL === "1") throw new Error(reason);
  return { path, skip: `SKIPPED (not silently): ${reason}` };
}

// flagCase runs fn with a sandbox (for profile, when given), fake Ollama
// servers by name, and track(handle) for processes and terminals the spec
// starts. Teardown always runs: terminals close, tracked processes stop, and
// any other process still carrying the sandbox's HOME and TMPDIR is stopped
// and reported. On failure the tracked logs are printed.
export async function flagCase(t, { profile, fakes = { main: {} } } = {}, fn) {
  const servers = {};
  for (const [name, opts] of Object.entries(fakes)) servers[name] = await startFakeOllama(opts);
  const sb = await createSandbox({ ollamaUrl: Object.values(servers)[0]?.url, profile });
  seedUpdateCache(sb);
  const tracked = [];
  const track = (h) => (tracked.push(h), h);
  let failed = false;
  try {
    return await fn({ sb, fakes: servers, track });
  } catch (err) {
    failed = true;
    for (const h of tracked) {
      if (h.getByText) t.diagnostic(`screen:\n${await h.text().catch(() => "<no screen>")}`);
      else if (h.lines) t.diagnostic(`mcp ${h.pid} stdout:\n${h.lines.join("\n")}\nstderr:\n${h.stderr()}`);
      else if (h.log) t.diagnostic(`process ${h.pid} log:\n${h.log()}`);
    }
    for (const p of ["default", sb.profile, "work", "a", "b"]) {
      try {
        t.diagnostic(`profiles/${p}/logs/agent.log:\n${readFileSync(join(sb.profileDir(p), "logs", "agent.log"), "utf8")}`);
      } catch {
        // no auto-launched agent for p
      }
    }
    throw err;
  } finally {
    for (const h of tracked.reverse()) {
      if (h.getByText) await h.close({ failed }).catch(() => {});
      else if (h.lines) await h.close(5_000).catch(() => {});
      else await h.stop?.(5_000).catch(() => {});
    }
    const leftovers = await reapSandbox(sb);
    if (leftovers.length) t.diagnostic(`stopped leftover sandbox processes: ${JSON.stringify(leftovers)}`);
    for (const s of Object.values(servers)) await s.close();
    sb.cleanup();
  }
}

// envFor returns the sandbox environment for one process: the flag in the
// given state, plus overrides (CERCANO_PROFILE, OLLAMA_URL, ...).
export function envFor(sb, flag, overrides = {}) {
  return withFlag({ ...process.env, ...sb.env, ...overrides }, flag);
}

// startCli runs cercano-cli in a PTY with the flag in the given state and
// returns the terminal (tracked by the caller).
export async function startCli(t, sb, { flag, label, args = [], env = {}, bin = CLI_BIN } = {}) {
  return runCli(t, { ...sb, env: withFlag({ ...sb.env, ...env }, flag) }, { label, args, bin });
}

// cliProcesses lists the sandbox's cercano-cli processes started from bin.
export function cliProcesses(sb, bin = CLI_BIN) {
  return sandboxProcesses(sb).filter((p) => p.args[0] === bin);
}

// waitFor polls check until it returns a truthy value.
export async function waitFor(check, { timeout = 15_000, what = "condition" } = {}) {
  const deadline = Date.now() + timeout;
  for (;;) {
    const v = await check();
    if (v) return v;
    if (Date.now() > deadline) throw new Error(`timed out after ${timeout}ms waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 100));
  }
}

// quit leaves the chat with Ctrl+C twice and expects a clean exit.
export async function quit(term) {
  await term.press("Ctrl+C");
  await term.press("Ctrl+C");
  assert.equal(await exitCode(term), 0);
}
