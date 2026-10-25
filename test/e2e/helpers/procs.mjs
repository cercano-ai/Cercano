// Process inspection for specs that must prove what a sandbox ran: the
// environment a process really got (so a spec knows which flag state it
// exercised), the addresses it listens on, and which processes a sandbox
// started. Cleanup signals only processes whose environment carries the
// sandbox's own HOME and TMPDIR (unique mkdtemp paths) and whose executable
// is one of this suite's builds under test/e2e/.bin; anything else is never
// touched.
import { execFileSync } from "node:child_process";
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { E2E_DIR } from "./paths.mjs";

const LINUX = process.platform === "linux";
const BIN_ROOT = join(E2E_DIR, ".bin") + "/";

function readProc(pid, name) {
  try {
    return readFileSync(`/proc/${pid}/${name}`);
  } catch {
    return null;
  }
}

// processEnv returns pid's environment as a Map, or null when it cannot be
// read (exited, or another user's).
export function processEnv(pid) {
  if (LINUX) {
    const raw = readProc(pid, "environ");
    if (!raw) return null;
    return new Map(
      raw
        .toString("utf8")
        .split("\0")
        .filter(Boolean)
        .map((entry) => {
          const i = entry.indexOf("=");
          return [entry.slice(0, i), entry.slice(i + 1)];
        }),
    );
  }
  // macOS: ps appends the environment to the command; values with spaces are
  // ambiguous, which is fine for the exact KEY=value tokens specs look for.
  try {
    const out = execFileSync("ps", ["eww", "-p", String(pid), "-o", "command="], { encoding: "utf8" });
    const env = new Map();
    for (const token of out.trim().split(/\s+/)) {
      const i = token.indexOf("=");
      if (i > 0 && /^[A-Z_][A-Z0-9_]*$/.test(token.slice(0, i))) env.set(token.slice(0, i), token.slice(i + 1));
    }
    return env;
  } catch {
    return null;
  }
}

// processArgs returns pid's argv, or null.
export function processArgs(pid) {
  if (LINUX) {
    const raw = readProc(pid, "cmdline");
    return raw?.length ? raw.toString("utf8").replace(/\0$/, "").split("\0") : null;
  }
  try {
    return execFileSync("ps", ["-p", String(pid), "-o", "args="], { encoding: "utf8" }).trim().split(/\s+/);
  } catch {
    return null;
  }
}

// startTime identifies one process across PID reuse.
export function startTime(pid) {
  if (LINUX) {
    const raw = readProc(pid, "stat");
    if (!raw) return null;
    const s = raw.toString("utf8");
    return s.slice(s.lastIndexOf(")") + 2).split(" ")[19] ?? null;
  }
  try {
    return execFileSync("ps", ["-p", String(pid), "-o", "lstart="], { encoding: "utf8" }).trim() || null;
  } catch {
    return null;
  }
}

export function alive(pid) {
  try {
    process.kill(pid, 0);
  } catch (err) {
    return err.code === "EPERM";
  }
  // A zombie still answers kill(0) but has exited; without an init that reaps
  // orphans it can linger, so it counts as gone.
  if (LINUX) {
    const raw = readProc(pid, "stat");
    if (!raw) return false;
    const s = raw.toString("utf8");
    return s.slice(s.lastIndexOf(")") + 2, s.lastIndexOf(")") + 3) !== "Z";
  }
  return true;
}

export async function waitForExit(pid, timeout = 15_000) {
  const deadline = Date.now() + timeout;
  while (alive(pid)) {
    if (Date.now() > deadline) throw new Error(`process ${pid} still running after ${timeout}ms`);
    await new Promise((r) => setTimeout(r, 100));
  }
}

function allPids() {
  if (LINUX) return readdirSync("/proc").filter((d) => /^\d+$/.test(d)).map(Number);
  try {
    return execFileSync("ps", ["-A", "-o", "pid="], { encoding: "utf8" }).trim().split(/\s+/).map(Number);
  } catch {
    return [];
  }
}

// sandboxProcesses lists this suite's binaries running with the sandbox's
// HOME and TMPDIR: every agent, MCP server, worker or CLI the sandbox started,
// however it was launched (directly, auto-launched, or by a restart).
export function sandboxProcesses(sandbox) {
  const found = [];
  for (const pid of allPids()) {
    if (pid === process.pid) continue;
    const args = processArgs(pid);
    if (!args?.[0]?.startsWith(BIN_ROOT)) continue;
    const env = processEnv(pid);
    if (env?.get("HOME") !== sandbox.home || env?.get("TMPDIR") !== sandbox.tmp) continue;
    if (!alive(pid)) continue;
    found.push({ pid, args, env });
  }
  return found;
}

// agentProcesses lists the sandbox's `cercano agent` processes.
export function agentProcesses(sandbox) {
  return sandboxProcesses(sandbox).filter((p) => p.args[1] === "agent");
}

// listenAddrs returns the TCP addresses pid listens on, as lsof prints them
// ("*:50052" for every interface, "127.0.0.1:50052" for loopback).
export function listenAddrs(pid) {
  let out;
  try {
    out = execFileSync("lsof", ["-nP", "-a", "-p", String(pid), "-iTCP", "-sTCP:LISTEN", "-Fn"], { encoding: "utf8" });
  } catch (err) {
    if (err.status === 1) return []; // no listening socket
    throw err;
  }
  return out
    .split("\n")
    .filter((l) => l.startsWith("n"))
    .map((l) => l.slice(1));
}

// reapSandbox stops every process sandboxProcesses finds: SIGTERM, a bounded
// wait, then SIGKILL, rechecking before each signal that the PID still names
// the same sandbox process. It returns what it had to stop, so a spec can
// assert that nothing was left behind.
export async function reapSandbox(sandbox, { wait = 5_000 } = {}) {
  const procs = sandboxProcesses(sandbox).map((p) => ({ ...p, start: startTime(p.pid) }));
  const same = (p) => {
    if (!p.start || startTime(p.pid) !== p.start) return false;
    const env = processEnv(p.pid);
    return env?.get("HOME") === sandbox.home && env?.get("TMPDIR") === sandbox.tmp;
  };
  const signal = (p, name) => {
    if (!same(p)) return;
    try {
      process.kill(p.pid, name);
    } catch (err) {
      if (err.code !== "ESRCH") throw err;
    }
  };
  for (const p of procs) signal(p, "SIGTERM");
  const deadline = Date.now() + wait;
  while (procs.some((p) => alive(p.pid) && same(p)) && Date.now() < deadline) await new Promise((r) => setTimeout(r, 100));
  for (const p of procs) if (alive(p.pid)) signal(p, "SIGKILL");
  return procs.map(({ pid, args }) => ({ pid, args: args.join(" ") }));
}
