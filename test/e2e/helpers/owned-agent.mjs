// A port or a registry record is discovery, never proof of ownership. Fail
// closed unless ps shows this build's agent with this sandbox's inherited
// environment. Like startAgent, allow graceful shutdown before escalating, and
// recheck identity each time.
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { AGENT_BIN } from "./paths.mjs";

// ownedAgentIdentity returns the start time of pid when it is this build's
// auto-launched agent for sandbox, else null. A sandbox without a fixed port
// (a named profile's) must show CERCANO_PORT empty; profile, when given, must
// match the agent's CERCANO_PROFILE.
export function ownedAgentIdentity(pid, sandbox, exec = execFileSync, profile) {
  if (!Number.isSafeInteger(pid) || pid <= 1) return null;
  try {
    const command = exec("ps", ["-p", String(pid), "-o", "args="], { encoding: "utf8" }).trim();
    if (command !== `${AGENT_BIN} agent`) return null;
    const environment = exec("ps", ["eww", "-p", String(pid), "-o", "command="], { encoding: "utf8" }).trim().split(/\s+/);
    const required = [`HOME=${sandbox.home}`, `TMPDIR=${sandbox.tmp}`, `CERCANO_PORT=${sandbox.port ?? ""}`, "CERCANO_AUTOLAUNCHED=1"];
    if (profile) required.push(`CERCANO_PROFILE=${profile}`);
    for (const entry of required) {
      if (!environment.includes(entry)) return null;
    }
    return exec("ps", ["-p", String(pid), "-o", "lstart="], { encoding: "utf8" }).trim() || null;
  } catch {
    return null; // exited, inaccessible, or unsupported ps: never guess
  }
}

const defaults = { exec: execFileSync, kill: process.kill, delay: (ms) => new Promise((r) => setTimeout(r, ms)) };

async function stopOwned(pid, sandbox, { exec, kill, delay }, profile) {
  const identity = ownedAgentIdentity(pid, sandbox, exec, profile);
  if (!identity) return;
  const signal = (name) => {
    if (ownedAgentIdentity(pid, sandbox, exec, profile) !== identity) return false;
    try { kill(pid, name); } catch (err) { if (err.code !== "ESRCH") throw err; }
    return true;
  };
  if (!signal("SIGTERM")) return;
  for (let i = 0; i < 50 && ownedAgentIdentity(pid, sandbox, exec, profile) === identity; i++) await delay(100);
  signal("SIGKILL");
}

// stopOwnedAgent stops pid if it is the sandbox's auto-launched agent (for
// profile, when given).
export async function stopOwnedAgent(pid, sandbox, { profile, ...opts } = {}) {
  await stopOwned(pid, sandbox, { ...defaults, ...opts }, profile);
}

// stopOwnedListener stops the sandbox's auto-launched agent found through its
// fixed port.
export async function stopOwnedListener(sandbox, opts = {}) {
  const o = { ...defaults, ...opts };
  let output;
  try {
    output = o.exec("lsof", ["-ti", `tcp:${sandbox.port}`, "-sTCP:LISTEN"], { encoding: "utf8" });
  } catch (err) {
    if (err.status === 1) return; // no listener
    throw err; // don't silently hide a broken cleanup tool
  }
  for (const raw of output.trim().split(/\s+/).filter(Boolean)) {
    await stopOwned(Number(raw), sandbox, o);
  }
}

// stopOwnedProfileAgent stops the agent auto-launched for profile in sandbox,
// found through the profile's agent.json.
export async function stopOwnedProfileAgent(sandbox, profile, opts = {}) {
  let rec;
  try {
    rec = JSON.parse(readFileSync(join(sandbox.profileDir(profile), "agent.json"), "utf8"));
  } catch {
    return; // no record: no running agent
  }
  await stopOwnedAgent(rec.pid, sandbox, { ...opts, profile });
}
