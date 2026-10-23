import { test } from "node:test";
import assert from "node:assert/strict";
import { copyFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import { withCli, ready, exitCode } from "../helpers/cli.mjs";
import { waitForPort } from "../helpers/agent.mjs";
import { stopOwnedListener } from "../helpers/owned-agent.mjs";

const MESSAGE = "a conversation from before profiles";
const REPLY = "remembered across the upgrade";
const DB = "conversations.db";

// recordConversation has a real agent store one conversation and returns a
// copy of its database, written by the same build under test.
async function recordConversation(t, dest) {
  await withCli(t, { ollama: { reply: REPLY } }, async ({ term, sandbox, agent }) => {
    await ready(term);
    await term.submit(MESSAGE);
    await term.getByText(REPLY).expect({ timeout: 30_000 });
    await term.press("Ctrl+C");
    await term.press("Ctrl+C");
    assert.equal(await exitCode(term), 0);
    await agent.stop(); // closes the database, folding in its WAL
    const db = join(sandbox.profileDir(), DB);
    assert.ok(existsSync(db), "the agent should store conversations in the profile directory");
    for (const suffix of ["", "-wal", "-shm"]) {
      if (existsSync(db + suffix)) copyFileSync(db + suffix, join(dest, DB + suffix));
    }
  });
}

// An install from before profiles keeps config.yaml and conversations.db
// directly in ~/.config/cercano. The first start moves them into
// profiles/default, and the default profile's history still lists the
// conversation.
test("a pre-profile install moves into the default profile and keeps its conversations", async (t) => {
  await withCli(
    t,
    {
      agent: false,
      seedConfig: false,
      args: ["-r"],
      before: async (sandbox) => {
        sandbox.writeConfig("", { legacy: true });
        await recordConversation(t, sandbox.configDir);
        assert.ok(existsSync(join(sandbox.configDir, DB)));
      },
    },
    async ({ term, sandbox, defer }) => {
      defer(() => stopOwnedListener(sandbox));
      await term.getByText(MESSAGE).expect();

      const profile = sandbox.profileDir("default");
      assert.ok(existsSync(join(sandbox.profilesDir, ".migrated-v1")), "migration should record that it finished");
      for (const name of ["config.yaml", DB]) {
        assert.ok(existsSync(join(profile, name)), `${name} should be in profiles/default`);
        assert.ok(!existsSync(join(sandbox.configDir, name)), `${name} should no longer be at the legacy path`);
      }
      assert.ok(existsSync(join(profile, "agent.json")), "the default profile's agent should be registered");
      assert.ok(existsSync(join(profile, "logs", "agent.log")), "the auto-launched agent should log in the profile");

      await term.press("Ctrl+C");
      await term.press("Ctrl+C");
      assert.equal(await exitCode(term), 0);
      await waitForPort(sandbox.port, { open: false, timeout: 15_000 });
    },
  );
});
