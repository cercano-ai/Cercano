// CERCANO_PER_PROFILE_AGENTS across restarts and upgrades, with real
// binaries: hosts started in one flag state meeting clients and
// restart-after-upgrade in the other, and builds from before the flag
// (f79bf87a, per-profile hosts unconditionally) and before profiles
// (c23f4656) meeting this build. Historical builds come from
// scripts/build-historical.sh; without them those tests are reported as
// skipped (or fail with E2E_REQUIRE_HISTORICAL=1).
//
// restart-after-upgrade replaces agents only on macOS (Homebrew, cgo). On
// Linux it selects the agents to restart — every running profile's, or the
// one --address names — and then reports that the restart itself is
// unsupported, so these tests check that selection and that nothing is
// stopped. Whether a selected agent is replaced or left running is decided
// inside the coordinator from the agent's captured launch environment; that
// decision and the replacement path are covered, as an emulation, by
// cmd/cercano/per_profile_flag_transition_test.go.
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { ready, exitCode } from "../helpers/cli.mjs";
import { readAgentRecord, portOpen, waitForPort } from "../helpers/agent.mjs";
import { agentProcesses, alive, listenAddrs, sandboxProcesses, waitForExit } from "../helpers/procs.mjs";
import {
  REFUSAL,
  assertFlagState,
  envFor,
  flagCase,
  historicalBin,
  quit,
  spawnCercano,
  startCli,
  startMcp,
  startProfileAgent,
  waitFor,
} from "../helpers/flag.mjs";

const REPLY = "served across the transition";
const LINUX_UNSUPPORTED = "Homebrew upgrade restart requires macOS with cgo";
const f79 = historicalBin("f79bf87a");
const c23 = historicalBin("c23f4656");
const c23Cli = historicalBin("c23f4656", "cercano-cli");

// restartAfterUpgrade runs this build's `cercano restart-after-upgrade` with
// the flag in the given state and returns its exit code and output.
async function restartAfterUpgrade(sb, track, flag, args = []) {
  const run = track(spawnCercano(sb, ["restart-after-upgrade", "--timeout", "10s", ...args], { env: envFor(sb, flag), logName: "restart" }));
  const { code } = await run.waitExit(60_000);
  return { code, out: run.log() };
}

// assertStillServing checks that a host is the same process with the same
// registration and still listening.
async function assertStillServing(sb, host, profile, what) {
  assert.equal(alive(host.pid), true, `${what} should still run`);
  assert.equal(readAgentRecord(sb, profile)?.instance_id, host.record.instance_id, `${what} should keep its registration`);
  assert.equal(await portOpen(host.port), true, `${what} should still listen`);
}

test("a named host started flag-on is refused but never touched by flag-off clients", async (t) => {
  await flagCase(t, { fakes: { main: { reply: REPLY } } }, async ({ sb, fakes, track }) => {
    sb.writeConfig("", { profile: "a" });
    const host = track(await startProfileAgent(sb, { profile: "a", flag: "1" }));
    assertFlagState(host.pid, "1", "profile a's host");

    const cli = track(await startCli(t, sb, { flag: undefined, label: "off", args: ["--profile", "a"] }));
    await cli.getByText("CERCANO_PER_PROFILE_AGENTS=1").expect();
    assert.equal(await exitCode(cli), 1);

    const mcp = track(startMcp(sb, { env: envFor(sb, undefined, { CERCANO_PROFILE: "a" }) }));
    await mcp.initialize();
    const res = await mcp.call("cercano_models");
    assert.equal(res.isError, true);
    assert.match(res.text, REFUSAL);
    assert.equal((await mcp.close()).code, 0);

    // A second flag-off agent for a is refused before it takes a's lock.
    const second = track(spawnCercano(sb, ["agent"], { env: envFor(sb, undefined, { CERCANO_PROFILE: "a", CERCANO_PORT: "" }), logName: "agent-a-off" }));
    assert.equal((await second.waitExit()).code, 1);
    assert.match(second.log(), REFUSAL);

    // Every flag-off client has exited; the flag-on host is as it was.
    await assertStillServing(sb, host, "a", "profile a's flag-on host");
    assert.deepEqual(agentProcesses(sb).map((p) => p.pid), [host.pid]);

    // Turning the flag on in the client reaches the same host.
    const on = track(await startCli(t, sb, { flag: "1", label: "on", args: ["--profile", "a"] }));
    await ready(on);
    await on.submit("hello again");
    await on.getByText(REPLY).expect({ timeout: 30_000 });
    assert.deepEqual(agentProcesses(sb).map((p) => p.pid), [host.pid], "the flag-on client should use the running host");
    await quit(on);
    assert.ok(fakes.main.chatRequests().length >= 1);
  });
});

test("the default profile's host survives flag changes in either direction", async (t) => {
  await flagCase(t, { fakes: { main: { reply: REPLY } } }, async ({ sb, track }) => {
    // Host flag-on, client flag-off.
    let host = track(await startProfileAgent(sb, { profile: "default", flag: "1" }));
    assert.deepEqual(listenAddrs(host.pid), [`127.0.0.1:${sb.port}`]);
    let cli = track(await startCli(t, sb, { flag: undefined, label: "off-client" }));
    await ready(cli);
    await cli.submit("hello");
    await cli.getByText(REPLY).expect({ timeout: 30_000 });
    await quit(cli);
    assert.deepEqual(agentProcesses(sb).map((p) => p.pid), [host.pid]);
    assert.equal((await host.stop()).code, 0);
    await waitForPort(sb.port, { open: false });

    // Restarted flag-off on the same port: back to every interface. Then a
    // flag-on client.
    host = track(await startProfileAgent(sb, { profile: "default", flag: undefined }));
    assert.deepEqual(listenAddrs(host.pid), [`*:${sb.port}`]);
    assert.equal(host.record.addr, `127.0.0.1:${sb.port}`);
    cli = track(await startCli(t, sb, { flag: "1", label: "on-client" }));
    await ready(cli);
    await cli.submit("hello");
    await cli.getByText(REPLY).expect({ timeout: 30_000 });
    await quit(cli);
    assert.deepEqual(agentProcesses(sb).map((p) => p.pid), [host.pid]);
    assert.equal((await host.stop()).code, 0);
    assert.deepEqual(sandboxProcesses(sb), []);
  });
});

test("restart-after-upgrade (Linux): every profile's host is selected whatever the flag, --address selects that profile's, nothing is stopped", async (t) => {
  await flagCase(t, {}, async ({ sb, track }) => {
    sb.writeConfig("", { profile: "a" });
    const def = track(await startProfileAgent(sb, { profile: "default", flag: undefined }));
    const named = track(await startProfileAgent(sb, { profile: "a", flag: "1" }));

    // The command's own flag plays no part in the selection.
    for (const flag of [undefined, "0", "1"]) {
      const r = await restartAfterUpgrade(sb, track, flag);
      for (const profile of ["default", "a"]) {
        assert.match(r.out, new RegExp(`Upgrade restart failed for profile "${profile}": ${LINUX_UNSUPPORTED}`), `flag ${flag}: ${r.out}`);
      }
      assert.doesNotMatch(r.out, /Left the agent/);
      assert.equal(r.code, 1);
      await assertStillServing(sb, def, "default", "the default host");
      await assertStillServing(sb, named, "a", "profile a's host");
    }

    // --address naming a's host restarts it as profile a's agent, and only it.
    const r = await restartAfterUpgrade(sb, track, undefined, ["--address", `127.0.0.1:${named.port}`]);
    assert.match(r.out, new RegExp(`Upgrade restart failed for profile "a": ${LINUX_UNSUPPORTED}`), r.out);
    assert.doesNotMatch(r.out, /profile "default"/);
    assert.equal(r.code, 1);

    await assertStillServing(sb, def, "default", "the default host");
    await assertStillServing(sb, named, "a", "profile a's host");
    assert.deepEqual(agentProcesses(sb).map((p) => p.pid).sort(), [def.pid, named.pid].sort());
  });
});

test("upgrade from f79bf87a: its named host is refused by flag-off clients and used by flag-on ones", { skip: f79.skip }, async (t) => {
  await flagCase(t, { fakes: { main: { reply: REPLY } } }, async ({ sb, fakes, track }) => {
    sb.writeConfig("", { profile: "work" });
    // f79bf87a has no flag: it serves named profiles unconditionally, on
    // loopback.
    const old = track(await startProfileAgent(sb, { profile: "work", flag: undefined, bin: f79.path }));
    assertFlagState(old.pid, undefined, "the f79bf87a host");
    assert.deepEqual(listenAddrs(old.pid), [`127.0.0.1:${old.port}`]);

    const off = track(await startCli(t, sb, { flag: undefined, label: "off", args: ["--profile", "work"] }));
    await off.getByText("CERCANO_PER_PROFILE_AGENTS=1").expect();
    assert.equal(await exitCode(off), 1);

    // Selected on Linux, where the restart is unsupported and stops nothing
    // (the Go transition test shows the coordinator leaving it running).
    const r = await restartAfterUpgrade(sb, track, undefined);
    assert.match(r.out, new RegExp(`Upgrade restart failed for profile "work": ${LINUX_UNSUPPORTED}`), r.out);
    await assertStillServing(sb, old, "work", "the f79bf87a host");

    const on = track(await startCli(t, sb, { flag: "1", label: "on", args: ["--profile", "work"] }));
    await ready(on);
    await on.submit("hello old host");
    await on.getByText(REPLY).expect({ timeout: 30_000 });
    assert.deepEqual(agentProcesses(sb).map((p) => p.pid), [old.pid], "the flag-on client should use the f79bf87a host");
    await quit(on);
    assert.ok(fakes.main.chatRequests().length >= 1);
  });
});

test("upgrade from f79bf87a: the default profile moves from its loopback host to a flag-off all-interfaces host", { skip: f79.skip }, async (t) => {
  await flagCase(t, { fakes: { main: { reply: REPLY } } }, async ({ sb, track, fakes }) => {
    const old = track(await startProfileAgent(sb, { profile: "default", flag: undefined, bin: f79.path }));
    assert.deepEqual(listenAddrs(old.pid), [`127.0.0.1:${sb.port}`]);

    let cli = track(await startCli(t, sb, { flag: undefined, label: "with-old" }));
    await ready(cli);
    await cli.submit("hello");
    await cli.getByText(REPLY).expect({ timeout: 30_000 });
    await quit(cli);
    assert.deepEqual(agentProcesses(sb).map((p) => p.pid), [old.pid], "this build's flag-off CLI should use the old default host");
    assert.equal((await old.stop()).code, 0);
    await waitForPort(sb.port, { open: false });

    // This build's flag-off CLI now auto-launches its own sibling host.
    cli = track(await startCli(t, sb, { flag: undefined, label: "auto" }));
    await ready(cli);
    const [host] = await waitFor(() => agentProcesses(sb).length && agentProcesses(sb), { what: "the auto-launched host" });
    assertFlagState(host.pid, undefined, "the auto-launched host");
    assert.equal(host.env.get("CERCANO_AUTOLAUNCHED"), "1");
    assert.deepEqual(listenAddrs(host.pid), [`*:${sb.port}`]);
    assert.equal(readAgentRecord(sb, "default")?.addr, `127.0.0.1:${sb.port}`);
    await cli.submit("hello");
    await cli.getByText(REPLY).expect({ timeout: 30_000 });
    await quit(cli);
    await waitForExit(host.pid, 20_000);
    assert.ok(fakes.main.chatRequests().length >= 2);
  });
});

// c23f4656 keeps config.yaml and conversations.db directly in
// ~/.config/cercano and its agent listens on the configured port.
async function startLegacyHost(sb, track) {
  sb.writeConfig("", { legacy: true });
  const host = track(spawnCercano(sb, ["agent"], { env: envFor(sb, undefined), bin: c23.path, logName: "agent-c23" }));
  await waitFor(async () => (await portOpen(sb.port)) || host.exited(), { what: "the c23f4656 host" });
  assert.equal(host.exited(), null, host.log());
  return host;
}

test("upgrade from c23f4656: flag-off cercano-cli --profile refuses without touching a live pre-profile install", { skip: c23.skip }, async (t) => {
  await flagCase(t, {}, async ({ sb, track }) => {
    // seedConfig wrote profiles/default; a pre-profile install has none.
    const { rmSync } = await import("node:fs");
    rmSync(sb.profilesDir, { recursive: true, force: true });
    const legacy = await startLegacyHost(sb, track);

    const cli = track(await startCli(t, sb, { flag: undefined, label: "named", args: ["--profile", "work"] }));
    await cli.getByText("CERCANO_PER_PROFILE_AGENTS=1").expect();
    assert.equal(await exitCode(cli), 1);

    assert.equal(alive(legacy.pid), true, "the pre-profile agent should keep running");
    assert.equal(await portOpen(sb.port), true);
    assert.equal(existsSync(join(sb.configDir, "config.yaml")), true, "legacy config.yaml should stay");
    assert.equal(existsSync(join(sb.profilesDir, ".migrated-v1")), false, "no migration should run");
  });
});

test("upgrade from c23f4656: a flag-off default client stops the live pre-profile agent, moves its data and serves it on every interface", { skip: c23.skip || c23Cli.skip }, async (t) => {
  const MESSAGE = "a conversation recorded by c23f4656";
  await flagCase(t, { fakes: { main: { reply: REPLY } } }, async ({ sb, track }) => {
    const { rmSync } = await import("node:fs");
    rmSync(sb.profilesDir, { recursive: true, force: true });
    const legacy = await startLegacyHost(sb, track);

    // Record a conversation with the old CLI and agent.
    const oldCli = track(await startCli(t, sb, { flag: undefined, label: "c23", bin: c23Cli.path }));
    await ready(oldCli);
    await oldCli.submit(MESSAGE);
    await oldCli.getByText(REPLY).expect({ timeout: 30_000 });
    await quit(oldCli);
    assert.equal(existsSync(join(sb.configDir, "conversations.db")), true, "c23f4656 stores conversations at the legacy path");

    // This build's flag-off CLI: the migration asks the old agent to shut
    // down, moves its data, and the CLI launches this build's host.
    const cli = track(await startCli(t, sb, { flag: undefined, label: "new", args: ["-r"] }));
    await cli.getByText(MESSAGE).expect({ timeout: 30_000 });
    await waitForExit(legacy.pid);
    assert.equal(existsSync(join(sb.profilesDir, ".migrated-v1")), true);
    assert.equal(existsSync(join(sb.profileDir("default"), "config.yaml")), true);
    assert.equal(existsSync(join(sb.configDir, "config.yaml")), false);
    const [host] = agentProcesses(sb);
    assert.ok(host, "this build's host should run");
    assert.equal(host.args[0].endsWith("/.bin/cercano"), true, host.args.join(" "));
    assert.deepEqual(listenAddrs(host.pid), [`*:${sb.port}`], "the flag-off host keeps the pre-profile all-interfaces bind");
    assert.match(readFileSync(join(sb.profileDir("default"), "logs", "agent.log"), "utf8"), /Server listening/);
    await quit(cli);
    await waitForExit(host.pid, 20_000);
  });
});
