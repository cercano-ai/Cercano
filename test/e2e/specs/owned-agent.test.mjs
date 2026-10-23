import { test } from "node:test";
import assert from "node:assert/strict";
import { AGENT_BIN } from "../helpers/paths.mjs";
import { ownedAgentIdentity, stopOwnedAgent, stopOwnedListener } from "../helpers/owned-agent.mjs";
import { createSandbox } from "../helpers/sandbox.mjs";
import { startAgent, waitForPort, portOpen } from "../helpers/agent.mjs";

const sandbox = { home: "/tmp/suite/home", tmp: "/tmp/suite/t", port: 12345 };
const environment = `${AGENT_BIN} agent HOME=${sandbox.home} TMPDIR=${sandbox.tmp} CERCANO_PORT=${sandbox.port} CERCANO_AUTOLAUNCHED=1`;
function probe({ command = `${AGENT_BIN} agent`, env = environment, start = "Thu Oct 22 10:00:00 2026" } = {}) {
  return (bin, args) => {
    if (bin === "lsof") return "123\n";
    if (args.at(-1) === "args=") return command;
    if (args.at(-1) === "command=") return env;
    if (args.at(-1) === "lstart=") return start;
    throw new Error("unexpected probe");
  };
}

test("cleanup never signals an unrelated listener or partially matching environment", async () => {
  for (const options of [
    { command: "unrelated-server" },
    { env: environment.replace(sandbox.home, `${sandbox.home}-other`) },
    { env: environment.replace(sandbox.tmp, `${sandbox.tmp}-other`) },
    { env: environment.replace("CERCANO_PORT=12345", "CERCANO_PORT=99999") },
    { env: environment.replace("CERCANO_AUTOLAUNCHED=1", "") },
  ]) {
    const signals = [];
    await stopOwnedListener(sandbox, { exec: probe(options), kill: (...args) => signals.push(args) });
    assert.deepEqual(signals, []);
  }
});

test("ownership inspection fails closed", () => {
  assert.equal(ownedAgentIdentity(123, sandbox, () => { throw new Error("ps unavailable"); }), null);
  for (const pid of [0, -1, NaN, 1]) assert.equal(ownedAgentIdentity(pid, sandbox, probe()), null);
});

test("owned cleanup sends TERM before bounded KILL", async () => {
  const signals = [];
  await stopOwnedListener(sandbox, { exec: probe(), kill: (...args) => signals.push(args), delay: async () => {} });
  assert.deepEqual(signals, [[123, "SIGTERM"], [123, "SIGKILL"]]);
});

test("cleanup revalidates identity before escalation", async () => {
  let replaced = false;
  const signals = [];
  await stopOwnedListener(sandbox, {
    exec: (bin, args) => probe({ start: replaced ? "a different process" : "original process" })(bin, args),
    kill: (...args) => { signals.push(args); replaced = true; },
    delay: async () => {},
  });
  assert.deepEqual(signals, [[123, "SIGTERM"]]);
});

test("profile cleanup signals only that profile's agent", async () => {
  const profiled = { home: sandbox.home, tmp: sandbox.tmp, port: null };
  const env = `${AGENT_BIN} agent HOME=${sandbox.home} TMPDIR=${sandbox.tmp} CERCANO_PORT= CERCANO_PROFILE=a CERCANO_AUTOLAUNCHED=1`;
  for (const [profile, options] of [
    ["b", { env }],
    ["a", { env: env.replace("CERCANO_PORT=", "CERCANO_PORT=12345") }],
    ["a", { env: env.replace("CERCANO_PROFILE=a", "") }],
  ]) {
    const signals = [];
    await stopOwnedAgent(123, profiled, { profile, exec: probe(options), kill: (...args) => signals.push(args), delay: async () => {} });
    assert.deepEqual(signals, [], `profile ${profile} with ${options.env}`);
  }
  const signals = [];
  await stopOwnedAgent(123, profiled, { profile: "a", exec: probe({ env }), kill: (...args) => signals.push(args), delay: async () => {} });
  assert.deepEqual(signals, [[123, "SIGTERM"], [123, "SIGKILL"]]);
});

test("ownership probes recognize and stop a real sandbox agent", async () => {
  const sb = await createSandbox();
  sb.env.CERCANO_AUTOLAUNCHED = "1";
  let agent;
  try {
    agent = await startAgent(sb);
    assert.ok(ownedAgentIdentity(agent.pid, sb));
    await stopOwnedListener({ ...sb, home: `${sb.home}-other` });
    assert.equal(await portOpen(sb.port), true, "another sandbox's agent must survive cleanup");
    await stopOwnedListener(sb);
    await waitForPort(sb.port, { open: false });
  } finally {
    await agent?.stop();
    sb.cleanup();
  }
});
