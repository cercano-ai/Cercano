// CERCANO_PER_PROFILE_AGENTS through the real binaries: absent, "false" and
// "0" must keep the pre-profile single-agent-host behavior and refuse named
// profiles; "1" and "true" give each profile its own loopback host. Every
// process a test starts has its environment read back, so each case is known
// to run the flag state it names (createSandbox's own default is "" for the
// default profile and "1" for named ones; see the last test).
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { ready, exitCode } from "../helpers/cli.mjs";
import { createSandbox, freePort } from "../helpers/sandbox.mjs";
import { startAgent, readAgentRecord, portOpen, waitForPort } from "../helpers/agent.mjs";
import { agentProcesses, listenAddrs, sandboxProcesses, waitForExit, processEnv, reapSandbox } from "../helpers/procs.mjs";
import {
  FLAG,
  FLAG_OFF,
  FLAG_ON,
  REFUSAL,
  assertFlagState,
  cliProcesses,
  envFor,
  flagCase,
  quit,
  spawnCercano,
  startCli,
  startMcp,
  startProfileAgent,
  waitFor,
} from "../helpers/flag.mjs";

const REPLY = "served by the flag test agent";

function writeProfileConfig(sb, profile, ollamaUrl, extra = "") {
  sb.writeConfig(extra, { profile });
  // writeConfig points every profile at the sandbox's first fake Ollama;
  // each profile here gets its own.
  const path = join(sb.profileDir(profile), "config.yaml");
  const text = readFileSync(path, "utf8").replace(/^ollama_url: .*$/m, `ollama_url: ${ollamaUrl}`);
  writeFileSync(path, text);
}

for (const { label, value } of FLAG_OFF) {
  test(`flag ${label}: the default profile keeps one all-interfaces agent host and named profiles are refused`, async (t) => {
    await flagCase(t, { fakes: { main: { reply: REPLY } } }, async ({ sb, fakes, track }) => {
      // A named profile with its own config: only the flag can refuse it.
      sb.writeConfig("", { profile: "work" });

      // The default profile's host: every interface at the configured port,
      // as before profiles, with a dialable loopback address in agent.json.
      const agent = track(await startProfileAgent(sb, { profile: "default", flag: value }));
      assertFlagState(agent.pid, value, "the default agent");
      assert.deepEqual(listenAddrs(agent.pid), [`*:${sb.port}`], "flag-off default host should listen on every interface");
      assert.equal(agent.record.profile, "default");
      assert.equal(agent.record.addr, `127.0.0.1:${sb.port}`);
      assert.equal(agent.record.pid, agent.pid);

      // The CLI uses that host for a real turn against the fake Ollama.
      const cli = track(await startCli(t, sb, { flag: value, label: "default" }));
      await ready(cli);
      const [cliProc] = await waitFor(() => cliProcesses(sb).length && cliProcesses(sb), { what: "the CLI process" });
      assertFlagState(cliProc.pid, value, "the CLI");
      await cli.submit("hello from the flag test");
      await cli.getByText(REPLY).expect({ timeout: 30_000 });
      assert.ok(fakes.main.chatRequests().length >= 1, "the turn should reach the fake Ollama");
      assert.deepEqual(agentProcesses(sb).map((p) => p.pid), [agent.pid], "the CLI should use the running host, not launch one");
      await quit(cli);

      // cercano-cli --profile work: refused, naming the flag, before any agent
      // is launched.
      const named = track(await startCli(t, sb, { flag: value, label: "named", args: ["--profile", "work"] }));
      await named.getByText("CERCANO_PER_PROFILE_AGENTS=1").expect();
      assert.equal(await exitCode(named), 1);
      assert.match((await named.text()).replace(/\n/g, ""), REFUSAL);

      // cercano agent for the named profile: exits 1, never listens.
      const namedAgent = track(spawnCercano(sb, ["agent"], { env: envFor(sb, value, { CERCANO_PROFILE: "work" }), logName: "agent-work" }));
      assert.equal((await namedAgent.waitExit()).code, 1, namedAgent.log());
      assert.match(namedAgent.log(), REFUSAL);
      assert.doesNotMatch(namedAgent.log(), /Server listening/);

      // cercano --mcp for the named profile: a degraded server that answers
      // every tool with the flag hint, and no agent is launched.
      const mcp = track(startMcp(sb, { env: envFor(sb, value, { CERCANO_PROFILE: "work" }) }));
      assertFlagState(mcp.pid, value, "the named-profile MCP server");
      await mcp.initialize();
      const res = await mcp.call("cercano_models");
      assert.equal(res.isError, true, res.text);
      assert.match(res.text, REFUSAL);
      assert.deepEqual(mcp.protocolViolations(), []);
      assert.deepEqual(listenAddrs(mcp.pid), [], "a degraded MCP server runs no embedded agent");
      const closed = await mcp.close();
      assert.equal(closed.code, 0);
      assert.equal(closed.forced, false);

      // Every refusal has finished (each process exited), so anything one of
      // them launched would exist by now: nothing did, and nothing was
      // written for the named profile.
      assert.deepEqual(agentProcesses(sb).map((p) => p.pid), [agent.pid], "no agent should start for the named profile");
      for (const name of ["agent.json", "agent.lock", join("logs", "agent.log")]) {
        assert.equal(existsSync(join(sb.profileDir("work"), name)), false, `profiles/work/${name} should not exist`);
      }

      // The default host is untouched, then shuts down cleanly.
      assert.equal(readAgentRecord(sb, "default")?.instance_id, agent.record.instance_id);
      assert.equal(await portOpen(sb.port), true);
      assert.equal((await agent.stop()).code, 0, agent.log());
      assert.equal(readAgentRecord(sb, "default"), null, "a stopped host removes its record");
      await waitForPort(sb.port, { open: false });
      assert.deepEqual(sandboxProcesses(sb), [], "nothing should outlive the host");
    });
  });

  test(`flag ${label}: cercano --mcp runs an embedded in-process agent, never a shared host`, async (t) => {
    await flagCase(t, { fakes: { main: { reply: REPLY } } }, async ({ sb, fakes, track }) => {
      const first = track(startMcp(sb, { env: envFor(sb, value) }));
      assertFlagState(first.pid, value, "the MCP server");
      await first.initialize();
      const tools = await first.tools();
      for (const name of ["cercano_local", "cercano_models", "cercano_config"]) assert.ok(tools.includes(name), `${name} should be offered`);
      const models = await first.call("cercano_models");
      assert.equal(models.isError, false, models.text);
      assert.match(models.text, /fake-model/, "the embedded agent should list the fake Ollama's models");
      assert.ok(fakes.main.requests.some((r) => r.path === "/api/tags"));

      // The agent lives in the MCP process: it listens on its own loopback
      // port, never the configured one, and registers no profile host.
      const [embedded, ...more] = listenAddrs(first.pid);
      assert.deepEqual(more, []);
      assert.match(embedded ?? "", /^127\.0\.0\.1:\d+$/, "the MCP process should hold the embedded gRPC listener");
      assert.notEqual(embedded, `127.0.0.1:${sb.port}`);

      // A second MCP server gets its own embedded agent, as before shared
      // hosts.
      const second = track(startMcp(sb, { env: envFor(sb, value) }));
      await second.initialize();
      assert.equal((await second.call("cercano_models")).isError, false);
      const [embedded2] = listenAddrs(second.pid);
      assert.match(embedded2 ?? "", /^127\.0\.0\.1:\d+$/);
      assert.notEqual(embedded2, embedded);

      // Both answered, so a launched host would exist by now.
      assert.deepEqual(agentProcesses(sb), [], "no `cercano agent` should be launched");
      assert.equal(readAgentRecord(sb, "default"), null, "no agent.json should be written");
      assert.equal(await portOpen(sb.port), false, "nothing should listen on the configured port");
      assert.deepEqual(first.protocolViolations(), []);
      assert.deepEqual(second.protocolViolations(), []);

      for (const mcp of [first, second]) {
        const closed = await mcp.close();
        assert.equal(closed.code, 0, mcp.stderr());
        assert.equal(closed.forced, false, "MCP should exit when its client closes stdin");
      }
      await waitFor(() => sandboxProcesses(sb).length === 0, { what: "the MCP servers' children to exit" });
    });
  });
}

for (const { label, value } of FLAG_ON) {
  test(`flag ${label}: each profile gets its own loopback host, state and routing`, async (t) => {
    const fakes = { a: { reply: "reply from profile a" }, b: { reply: "reply from profile b" }, d: { reply: "reply from default" } };
    await flagCase(t, { profile: "a", fakes }, async ({ sb, fakes, track }) => {
      const defaultPort = await freePort();
      writeProfileConfig(sb, "a", fakes.a.url);
      writeProfileConfig(sb, "b", fakes.b.url);
      writeProfileConfig(sb, "default", fakes.d.url, `port: "${defaultPort}"`);
      assert.equal(sb.env.CERCANO_PORT, "", "a named-profile sandbox sets no fixed port");

      const start = (profile, fake) => startProfileAgent(sb, { profile, flag: value, extraEnv: { OLLAMA_URL: fake.url } });
      const hosts = { a: track(await start("a", fakes.a)), b: track(await start("b", fakes.b)), default: track(await start("default", fakes.d)) };
      for (const [profile, h] of Object.entries(hosts)) {
        assertFlagState(h.pid, value, `profile ${profile}'s agent`);
        assert.equal(h.record.profile, profile);
        assert.deepEqual(listenAddrs(h.pid), [`127.0.0.1:${h.port}`], `profile ${profile}'s host should listen on loopback only`);
      }
      assert.equal(hosts.default.port, defaultPort, "the default profile keeps its configured port");
      assert.equal(new Set(Object.values(hosts).map((h) => h.pid)).size, 3);
      assert.equal(new Set(Object.values(hosts).map((h) => h.port)).size, 3);
      assert.equal(new Set(Object.values(hosts).map((h) => h.record.instance_id)).size, 3);
      assert.equal(existsSync(join(sb.profileDir("b"), "agent.lock")), true, "each host holds its own profile lock");

      // The CLI for b reaches b's host (and so b's Ollama) only.
      const cliB = track(await startCli(t, sb, { flag: value, label: "b", args: ["--profile", "b"] }));
      await ready(cliB);
      await cliB.submit("which profile");
      await cliB.getByText("reply from profile b").expect({ timeout: 30_000 });
      assert.ok(fakes.b.chatRequests().length >= 1);
      assert.equal(fakes.a.chatRequests().length, 0);
      assert.equal(fakes.d.chatRequests().length, 0);
      await quit(cliB);

      // MCP for a and for default dial the profile's running host: the turn
      // is served through that host's Ollama, the MCP process runs no agent,
      // and no new agent starts. The MCP servers' own OLLAMA_URL points
      // elsewhere, so an agent of their own would show.
      for (const [profile, fake] of [["a", fakes.a], ["default", fakes.d]]) {
        const before = fake.chatRequests().length;
        const mcp = track(startMcp(sb, { env: envFor(sb, value, { CERCANO_PROFILE: profile === "default" ? "" : profile, OLLAMA_URL: fakes.b.url }) }));
        assertFlagState(mcp.pid, value, `profile ${profile}'s MCP server`);
        await mcp.initialize();
        const res = await mcp.call("cercano_local", { prompt: `hello ${profile}` }, 60_000);
        assert.equal(res.isError, false, res.text);
        assert.match(res.text, new RegExp(`Endpoint: ${fake.url.replace(/[.]/g, "\\.")}`), `profile ${profile}'s turn should use its host's Ollama`);
        assert.ok(fake.chatRequests().length > before);
        assert.deepEqual(listenAddrs(mcp.pid), [], "MCP should not embed an agent with the flag on");
        assert.deepEqual(mcp.protocolViolations(), [], "a shared host keeps MCP stdout pure");
        assert.equal((await mcp.close()).forced, false);
      }
      assert.deepEqual(agentProcesses(sb).map((p) => p.pid).sort(), Object.values(hosts).map((h) => h.pid).sort(), "no extra agent should start");

      // Stopping a's host removes only a's state; b and default keep serving.
      assert.equal((await hosts.a.stop()).code, 0, hosts.a.log());
      assert.equal(readAgentRecord(sb, "a"), null);
      await waitForPort(hosts.a.port, { open: false });
      for (const profile of ["b", "default"]) {
        assert.equal(readAgentRecord(sb, profile)?.instance_id, hosts[profile].record.instance_id, `profile ${profile}'s record should be untouched`);
        assert.equal(await portOpen(hosts[profile].port), true);
      }

      // A's lock was released: a new host takes over a's profile.
      const restarted = track(await start("a", fakes.a));
      assert.notEqual(restarted.pid, hosts.a.pid);
      assert.notEqual(restarted.record.instance_id, hosts.a.record.instance_id);
      assert.deepEqual(listenAddrs(restarted.pid), [`127.0.0.1:${restarted.port}`]);

      for (const h of [restarted, hosts.b, hosts.default]) assert.equal((await h.stop()).code, 0, h.log());
      for (const profile of ["a", "b", "default"]) assert.equal(readAgentRecord(sb, profile), null);
      assert.deepEqual(sandboxProcesses(sb), []);
    });
  });

  test(`flag ${label}: cercano --mcp launches the profile's shared host, which exits after MCP`, async (t) => {
    await flagCase(t, { fakes: { main: { reply: REPLY } } }, async ({ sb, fakes, track }) => {
      const mcp = track(startMcp(sb, { env: envFor(sb, value) }));
      await mcp.initialize();
      const res = await mcp.call("cercano_local", { prompt: "hello shared host" }, 60_000);
      assert.equal(res.isError, false, res.text);
      assert.match(res.text, new RegExp(`Endpoint: ${fakes.main.url.replace(/[.]/g, "\\.")}`));

      const [host, ...others] = agentProcesses(sb);
      assert.ok(host, "MCP should have launched the default profile's agent");
      assert.deepEqual(others, []);
      assert.equal(host.env.get("CERCANO_AUTOLAUNCHED"), "1");
      assertFlagState(host.pid, value, "the launched host");
      const rec = readAgentRecord(sb, "default");
      assert.equal(rec?.pid, host.pid);
      assert.equal(rec?.addr, `127.0.0.1:${sb.port}`);
      assert.deepEqual(listenAddrs(host.pid), [`127.0.0.1:${sb.port}`], "flag-on hosts listen on loopback only");
      assert.deepEqual(listenAddrs(mcp.pid), []);
      assert.deepEqual(mcp.protocolViolations(), []);

      assert.equal((await mcp.close()).forced, false);
      await waitForExit(host.pid, 20_000);
      assert.equal(readAgentRecord(sb, "default"), null);
      assert.deepEqual(sandboxProcesses(sb), []);
    });
  });
}

// An explicit --grpc-addr always dials that address, whatever the flag and
// profile: no embedded agent, no profile routing, no launch.
for (const { label, value } of [FLAG_OFF[0], FLAG_ON[0]]) {
  test(`flag ${label}: cercano --mcp --grpc-addr uses the given host`, async (t) => {
    await flagCase(t, { profile: "a", fakes: { a: { reply: REPLY }, other: {} } }, async ({ sb, fakes, track }) => {
      const host = track(await startProfileAgent(sb, { profile: "a", flag: "1" }));
      const mcp = track(
        startMcp(sb, { args: ["--grpc-addr", host.record.addr], env: envFor(sb, value, { CERCANO_PROFILE: "", OLLAMA_URL: fakes.other.url }) }),
      );
      assertFlagState(mcp.pid, value, "the MCP server");
      await mcp.initialize();
      const res = await mcp.call("cercano_local", { prompt: "hello" }, 60_000);
      assert.equal(res.isError, false, res.text);
      assert.ok(res.text.includes(`Endpoint: ${fakes.a.url}`), `the turn should run on the given host: ${res.text}`);
      assert.equal(fakes.other.chatRequests().length, 0);
      assert.deepEqual(listenAddrs(mcp.pid), [], "no embedded agent with --grpc-addr");
      assert.deepEqual(mcp.protocolViolations(), []);
      assert.equal((await mcp.close()).forced, false);
      assert.deepEqual(agentProcesses(sb).map((p) => p.pid), [host.pid]);
      assert.equal(readAgentRecord(sb, "default"), null, "no default-profile host should be launched");
    });
  });
}

// The shared helpers' own flag defaults, which every other spec inherits:
// default-profile sandboxes run flag-off ("" reads as unset) and get the
// all-interfaces host; profile sandboxes run "1" and get a loopback host.
test("existing sandboxes run default-profile specs flag-off and profile specs flag-on", async () => {
  for (const [profile, expected, bind] of [
    [undefined, "", (sb) => [`*:${sb.port}`]],
    ["a", "1", (sb, rec) => [rec.addr]],
  ]) {
    const sb = await createSandbox({ profile });
    let agent;
    try {
      assert.equal(sb.env[FLAG], expected);
      agent = await startAgent(sb);
      const env = processEnv(agent.pid);
      assert.equal(env?.get(FLAG), expected, `sandbox ${profile ?? "default"} agent environment`);
      const rec = readAgentRecord(sb, sb.profile);
      assert.deepEqual(listenAddrs(agent.pid), bind(sb, rec));
      if (profile) assert.match(rec.addr, /^127\.0\.0\.1:\d+$/);
    } finally {
      await agent?.stop();
      await reapSandbox(sb);
      sb.cleanup();
    }
  }
});
