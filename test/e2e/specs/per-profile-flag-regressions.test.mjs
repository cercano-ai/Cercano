// Regression tests for defects found in the CERCANO_PER_PROFILE_AGENTS
// patch, through the real binaries:
//
// - A named profile refused while the flag is off must be refused before
//   pre-profile data is migrated: no file moves, no migration marker, and a
//   running pre-profile agent keeps running.
// - `cercano --mcp` must keep standard output to JSON-RPC messages for real
//   tool calls, also with the embedded in-process agent, whose diagnostics
//   belong on standard error.
//
// The upgrade-restart defect is covered by
// source/server/cmd/cercano/per_profile_flag_transition_test.go.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { portOpen } from "../helpers/agent.mjs";
import { agentProcesses, alive, listenAddrs } from "../helpers/procs.mjs";
import { FLAG_OFF, REFUSAL, assertFlagState, envFor, flagCase, historicalBin, spawnCercano, startMcp, startProfileAgent, waitFor } from "../helpers/flag.mjs";

const c23 = historicalBin("c23f4656");

// legacyState reports where pre-profile data is.
function legacyState(sb) {
  return {
    legacyConfig: existsSync(join(sb.configDir, "config.yaml")),
    profilesDir: existsSync(sb.profilesDir),
    marker: existsSync(join(sb.profilesDir, ".migrated-v1")),
  };
}
const UNTOUCHED = { legacyConfig: true, profilesDir: false, marker: false };

// seedLegacyInstall turns the sandbox into an install from before profiles:
// config.yaml directly in ~/.config/cercano and no profiles directory.
function seedLegacyInstall(sb) {
  rmSync(sb.profilesDir, { recursive: true, force: true });
  sb.writeConfig("", { legacy: true });
}

// Every main command that runs or dials the named profile's agent host.
const HOST_COMMANDS = [["agent"], [], ["--server"], ["run", "hello"]];

for (const { label, value } of FLAG_OFF) {
  test(`flag ${label}: a refused named profile leaves a pre-profile install untouched (agent, server, run)`, async (t) => {
    await flagCase(t, {}, async ({ sb, track }) => {
      seedLegacyInstall(sb);
      for (const args of HOST_COMMANDS) {
        const what = `cercano ${args.join(" ")}`.trim();
        const p = track(spawnCercano(sb, args, { env: envFor(sb, value, { CERCANO_PROFILE: "work", CERCANO_PORT: "" }), logName: "refused" }));
        assert.equal((await p.waitExit()).code, 1, `${what}:\n${p.log()}`);
        assert.match(p.log(), REFUSAL, what);
        assert.doesNotMatch(p.log(), /Moved existing Cercano data|Server listening/, what);
        assert.deepEqual(legacyState(sb), UNTOUCHED, `${what} should not migrate`);
      }
      assert.deepEqual(agentProcesses(sb), []);
    });
  });

  test(`flag ${label}: named-profile cercano --mcp serves degraded without migrating`, async (t) => {
    await flagCase(t, {}, async ({ sb, track }) => {
      seedLegacyInstall(sb);
      const mcp = track(startMcp(sb, { env: envFor(sb, value, { CERCANO_PROFILE: "work", CERCANO_PORT: "" }) }));
      assertFlagState(mcp.pid, value, "the MCP server");
      await mcp.initialize();
      for (const name of ["cercano_models", "cercano_local"]) {
        const res = await mcp.call(name, name === "cercano_local" ? { prompt: "hello" } : {});
        assert.equal(res.isError, true, `${name}: ${res.text}`);
        assert.match(res.text, REFUSAL, name);
      }
      assert.deepEqual(listenAddrs(mcp.pid), [], "a degraded server runs no agent");
      const closed = await mcp.close();
      assert.equal(closed.code, 0, mcp.stderr());
      assert.equal(closed.forced, false);
      assert.deepEqual(mcp.protocolViolations(), []);
      assert.deepEqual(legacyState(sb), UNTOUCHED);
      assert.deepEqual(agentProcesses(sb), []);
    });
  });
}

test("flag absent: refused named-profile commands leave a live pre-profile agent serving", { skip: c23.skip }, async (t) => {
  await flagCase(t, {}, async ({ sb, track }) => {
    seedLegacyInstall(sb);
    const legacy = track(spawnCercano(sb, ["agent"], { env: envFor(sb, undefined), bin: c23.path, logName: "agent-c23" }));
    await waitFor(async () => (await portOpen(sb.port)) || legacy.exited(), { what: "the c23f4656 host" });
    assert.equal(legacy.exited(), null, legacy.log());

    const env = envFor(sb, undefined, { CERCANO_PROFILE: "work", CERCANO_PORT: "" });
    for (const args of HOST_COMMANDS) {
      const p = track(spawnCercano(sb, args, { env, logName: "refused" }));
      assert.equal((await p.waitExit(90_000)).code, 1, p.log());
      assert.match(p.log(), REFUSAL);
    }
    const mcp = track(startMcp(sb, { env }));
    await mcp.initialize();
    assert.match((await mcp.call("cercano_models")).text, REFUSAL);
    assert.equal((await mcp.close()).code, 0);

    // Every refused command has exited: the old agent was never asked to
    // stop, so it is the same process, still listening, data in place.
    assert.equal(alive(legacy.pid), true, `the pre-profile agent should keep running:\n${legacy.log().slice(-1500)}`);
    assert.equal(await portOpen(sb.port), true);
    assert.doesNotMatch(legacy.log(), /ShutdownAgent accepted/);
    assert.deepEqual(legacyState(sb), UNTOUCHED);
  });
});

// An explicit --grpc-addr is not a profile host: a named profile with the
// flag off still connects to it.
test("flag absent: named-profile cercano --mcp --grpc-addr still uses the given host", async (t) => {
  await flagCase(t, { profile: "a", fakes: { a: { reply: "served" } } }, async ({ sb, fakes, track }) => {
    const host = track(await startProfileAgent(sb, { profile: "a", flag: "1" }));
    const mcp = track(startMcp(sb, { args: ["--grpc-addr", host.record.addr], env: envFor(sb, undefined, { CERCANO_PROFILE: "work" }) }));
    assertFlagState(mcp.pid, undefined, "the MCP server");
    await mcp.initialize();
    const res = await mcp.call("cercano_local", { prompt: "hello" }, 60_000);
    assert.equal(res.isError, false, res.text);
    assert.ok(res.text.includes(`Endpoint: ${fakes.a.url}`), res.text);
    assert.equal((await mcp.close()).code, 0);
    assert.deepEqual(mcp.protocolViolations(), []);
    assert.deepEqual(agentProcesses(sb).map((p) => p.pid), [host.pid]);
  });
});

// exerciseTools makes real calls to the main tools, checking after each that
// a response came back and standard output still carries only JSON-RPC.
async function exerciseTools(mcp, ollamaUrl) {
  const calls = [
    ["cercano_models", {}],
    ["cercano_config", { action: "get" }],
    ["cercano_config", { action: "set", local_model: "fake-model", ollama_url: ollamaUrl, local_runtime: "ollama" }],
    ["cercano_local", { prompt: "hello" }],
    ["cercano_local", { prompt: "hello again", context: "some context" }],
    ["cercano_skills", { action: "list" }],
    ["cercano_stats", {}],
    ["cercano_submit_usage", { cloud_input_tokens: 1, cloud_output_tokens: 1 }],
    ["cercano_summarize", { text: "A long text to summarize. It has two sentences." }],
    ["cercano_explain", { text: "func main() {}" }],
    ["cercano_classify", { text: "error: disk full", categories: "bug,feature" }],
    ["cercano_extract", { query: "the number", text: "The number is 42." }],
  ];
  const results = {};
  for (const [name, args] of calls) {
    const res = await mcp.call(name, args, 90_000);
    assert.equal(res.rpcError, undefined, `${name} should answer as a tool result: ${res.text}`);
    assert.deepEqual(mcp.protocolViolations(), [], `standard output after ${name} ${JSON.stringify(args)}`);
    results[`${name} ${args.action ?? ""}`.trim()] ??= res;
  }
  assert.match(results["cercano_config set"].text, /Configuration update success/);
  assert.ok(results.cercano_local.text.includes(`Endpoint: ${ollamaUrl}`), results.cercano_local.text);
  assert.match(results.cercano_models.text, /fake-model/);
  return results;
}

for (const { label, value } of FLAG_OFF) {
  test(`flag ${label}: embedded cercano --mcp keeps standard output to JSON-RPC across real tool calls`, async (t) => {
    await flagCase(t, { fakes: { main: { reply: "reply" } } }, async ({ sb, fakes, track }) => {
      const mcp = track(startMcp(sb, { env: envFor(sb, value) }));
      assertFlagState(mcp.pid, value, "the MCP server");
      await mcp.initialize();
      assert.match(listenAddrs(mcp.pid)[0] ?? "", /^127\.0\.0\.1:\d+$/, "the agent should be embedded in the MCP process");
      await exerciseTools(mcp, fakes.main.url);
      assert.deepEqual(agentProcesses(sb), [], "no shared host");
      const closed = await mcp.close();
      assert.equal(closed.code, 0, mcp.stderr());
      assert.equal(closed.forced, false);
      assert.deepEqual(mcp.protocolViolations(), []);
      // The diagnostics still exist; they moved to standard error.
      const stderr = mcp.stderr();
      for (const diag of [/Received request \(Unary\): hello/, /ProcessRequest completed successfully/, /UpdateConfig: Local model set to fake-model/, /UpdateConfig: Ollama URL set to/]) {
        assert.match(stderr, diag);
      }
    });
  });
}

// The flag-on control: the same calls through the shared host, whose log
// gets the diagnostics.
test(`flag "1": cercano --mcp through the shared host keeps standard output to JSON-RPC across the same calls`, async (t) => {
  await flagCase(t, { fakes: { main: { reply: "reply" } } }, async ({ sb, fakes, track }) => {
    const mcp = track(startMcp(sb, { env: envFor(sb, "1") }));
    await mcp.initialize();
    await exerciseTools(mcp, fakes.main.url);
    const [host, ...others] = agentProcesses(sb);
    assert.ok(host, "MCP should use a shared host");
    assert.deepEqual(others, []);
    assert.deepEqual(listenAddrs(mcp.pid), []);
    assert.equal((await mcp.close()).code, 0);
    assert.deepEqual(mcp.protocolViolations(), []);
    const hostLog = readFileSync(join(sb.profileDir("default"), "logs", "agent.log"), "utf8");
    assert.match(hostLog, /Received request \(Unary\): hello/);
    assert.match(hostLog, /UpdateConfig: Local model set to fake-model/);
  });
});
