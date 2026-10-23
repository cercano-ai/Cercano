import { test } from "node:test";
import assert from "node:assert/strict";
import { copyFileSync, readFileSync, rmSync } from "node:fs";
import { stopOwnedListener } from "../helpers/owned-agent.mjs";
import { join } from "node:path";
import { withCli, exitCode, ready } from "../helpers/cli.mjs";
import { waitForPort, portOpen, readAgentRecord } from "../helpers/agent.mjs";
import { CLI_BIN } from "../helpers/paths.mjs";

test("without a reachable or launchable agent the CLI explains and exits 1", async (t) => {
  // A lone copy of the CLI: no sibling `cercano`, and a PATH without one (a
  // developer's PATH may well have a real install).
  const lone = join(process.env.TMPDIR ?? "/tmp", `cercano-cli-lone-${process.pid}`);
  copyFileSync(CLI_BIN, lone);
  t.after(() => rmSync(lone, { force: true }));

  await withCli(t, { agent: false, bin: lone, env: { PATH: "/usr/bin:/bin" } }, async ({ term, sandbox }) => {
    await term.getByText("could not auto-launch").expect();
    assert.equal(await exitCode(term), 1);
    assert.equal(await portOpen(sandbox.port), false);
  });
});

test("the CLI auto-launches its sibling agent, which exits after the last client", async (t) => {
  await withCli(t, { agent: false }, async ({ term, sandbox, defer }) => {
    defer(() => stopOwnedListener(sandbox));
    await ready(term);
    assert.equal(await portOpen(sandbox.port), true, "auto-launched agent should be listening");
    const log = readFileSync(join(sandbox.profileDir(), "logs", "agent.log"), "utf8");
    assert.match(log, /Server listening/);
    assert.equal(readAgentRecord(sandbox)?.addr, `127.0.0.1:${sandbox.port}`, "the default profile's agent.json should name its port");

    await term.press("Ctrl+C");
    await term.press("Ctrl+C");
    assert.equal(await exitCode(term), 0);
    await waitForPort(sandbox.port, { open: false, timeout: 15_000 });
  });
});
