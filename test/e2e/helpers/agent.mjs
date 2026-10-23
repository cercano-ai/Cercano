// Starts `cercano agent` for a sandbox so specs control its lifetime and can
// read its log, rather than relying on the CLI's auto-launch.
import { spawn } from "node:child_process";
import { openSync, closeSync, readFileSync } from "node:fs";
import { connect } from "node:net";
import { join } from "node:path";
import { AGENT_BIN } from "./paths.mjs";

export function portOpen(port) {
  return new Promise((resolve) => {
    const sock = connect({ port, host: "127.0.0.1" });
    sock.once("connect", () => {
      sock.destroy();
      resolve(true);
    });
    sock.once("error", () => resolve(false));
  });
}

export async function waitForPort(port, { open = true, timeout = 15_000 } = {}) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if ((await portOpen(port)) === open) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`port ${port} still ${open ? "closed" : "open"} after ${timeout}ms`);
}

// readAgentRecord returns a profile's agent.json, or null while there is none.
export function readAgentRecord(sandbox, profile = sandbox.profile) {
  try {
    return JSON.parse(readFileSync(join(sandbox.profileDir(profile), "agent.json"), "utf8"));
  } catch {
    return null;
  }
}

// waitForAgentRecord waits until the profile's agent.json names a listening
// agent (and, when pid is given, that process) and returns the record.
export async function waitForAgentRecord(sandbox, { profile = sandbox.profile, pid, timeout = 15_000 } = {}) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const rec = readAgentRecord(sandbox, profile);
    if (rec && (pid === undefined || rec.pid === pid)) {
      const port = Number(rec.addr?.split(":").at(-1));
      if (port && (await portOpen(port))) return rec;
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`no listening agent recorded for profile ${profile} after ${timeout}ms`);
}

export async function startAgent(sandbox) {
  const logPath = join(sandbox.root, "agent.log");
  const fd = openSync(logPath, "a");
  // Own process group, so stop() also reaches the `cercano worker` children.
  const child = spawn(AGENT_BIN, ["agent"], {
    env: { ...process.env, ...sandbox.env },
    stdio: ["ignore", fd, fd],
    detached: true,
  });
  closeSync(fd);

  let exited = null;
  child.once("exit", (code, signal) => (exited = { code, signal }));

  const log = () => {
    try {
      return readFileSync(logPath, "utf8");
    } catch {
      return "";
    }
  };

  // A default-profile sandbox has a fixed port; a named profile's agent
  // takes an ephemeral one and publishes it in agent.json.
  try {
    if (sandbox.port) await waitForPort(sandbox.port);
    else await waitForAgentRecord(sandbox, { pid: child.pid });
  } catch (err) {
    stopGroup(child.pid, "SIGKILL");
    const where = sandbox.port ? `on ${sandbox.port}` : `for profile ${sandbox.profile}`;
    throw new Error(`agent did not listen ${where} (exit=${JSON.stringify(exited)}):\n${log()}`, { cause: err });
  }

  return {
    pid: child.pid,
    logPath,
    log,
    async stop() {
      if (exited) return;
      const done = new Promise((r) => child.once("exit", r));
      stopGroup(child.pid, "SIGTERM");
      const timer = setTimeout(() => stopGroup(child.pid, "SIGKILL"), 5_000);
      await done;
      clearTimeout(timer);
    },
  };
}

function stopGroup(pid, signal) {
  try {
    process.kill(-pid, signal);
  } catch {
    // already gone
  }
}
