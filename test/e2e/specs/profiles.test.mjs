import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { join } from "node:path";
import { withCli, runCli, ready, exitCode } from "../helpers/cli.mjs";
import { readAgentRecord, waitForAgentRecord } from "../helpers/agent.mjs";
import { stopOwnedProfileAgent } from "../helpers/owned-agent.mjs";

const MESSAGE = "profile a keeps this conversation";
const REPLY = "noted in profile a";

function alive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch (err) {
    return err.code === "EPERM";
  }
}

async function waitForExit(pid, timeout = 15_000) {
  const deadline = Date.now() + timeout;
  while (alive(pid)) {
    if (Date.now() > deadline) throw new Error(`agent ${pid} still running after ${timeout}ms`);
    await new Promise((r) => setTimeout(r, 100));
  }
}

async function quit(term) {
  await term.press("Ctrl+C");
  await term.press("Ctrl+C");
  assert.equal(await exitCode(term), 0);
}

// Two profiles in one HOME: each CLI auto-launches its own profile's agent on
// an ephemeral port, and conversations stay in the profile that holds them.
test("each profile runs its own agent and keeps its own conversations", async (t) => {
  await withCli(
    t,
    { profile: "a", args: ["--profile", "a"], agent: false, ollama: { reply: REPLY } },
    async ({ term, sandbox, defer }) => {
      sandbox.writeConfig("", { profile: "b" });
      // Registered before any wait can fail, and run once every client is
      // closed, so no client relaunches an agent. Each profile's agent is
      // found through its agent.json and stopped only if it is verifiably
      // this sandbox's.
      for (const profile of ["a", "b"]) defer(() => stopOwnedProfileAgent(sandbox, profile));
      const terms = {};
      let ok = false;
      try {
        await ready(term);
        const recA = await waitForAgentRecord(sandbox, { profile: "a" });
        await term.submit(MESSAGE);
        await term.getByText(REPLY).expect({ timeout: 30_000 });

        terms.b = await runCli(t, sandbox, { label: "b", args: ["--profile", "b", "-r"] });
        const recB = await waitForAgentRecord(sandbox, { profile: "b" });

        assert.equal(recA.profile, "a");
        assert.equal(recB.profile, "b");
        assert.notEqual(recA.pid, recB.pid, "each profile should have its own agent process");
        assert.notEqual(recA.addr, recB.addr, "each profile's agent should listen on its own port");
        assert.equal(readAgentRecord(sandbox, "a")?.pid, recA.pid, "profile a's agent should still be registered");
        assert.ok(!existsSync(join(sandbox.profileDir("default"), "agent.json")), "no default-profile agent should start");

        // Profile b's history is empty; the same picker in profile a lists
        // the conversation, through the agent already running for a.
        await terms.b.getByText("(no saved conversations)").expect();
        await terms.b.getByText(MESSAGE).expect({ not: true, timeout: 1_000 });
        terms.a2 = await runCli(t, sandbox, { label: "a-resume", args: ["--profile", "a", "-r"] });
        await terms.a2.getByText(MESSAGE).expect();
        assert.equal(readAgentRecord(sandbox, "a")?.pid, recA.pid, "a second client of profile a should reuse its agent");

        // Each agent exits once its own profile's last client has gone.
        await quit(terms.b);
        await waitForExit(recB.pid);
        assert.equal(alive(recA.pid), true, "profile a's agent should outlive profile b's clients");
        await quit(terms.a2);
        await quit(term);
        await waitForExit(recA.pid);
        ok = true;
      } finally {
        for (const [label, other] of Object.entries(terms)) {
          if (!ok) t.diagnostic(`screen ${label}:\n${await other.text().catch(() => "<no screen>")}`);
          await other.close({ failed: !ok }).catch(() => {});
        }
      }
    },
  );
});
