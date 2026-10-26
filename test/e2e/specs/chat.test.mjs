import { test } from "node:test";
import assert from "node:assert/strict";
import { withCli, ready } from "../helpers/cli.mjs";

const REPLY = "pelicans are the official e2e bird";

test("a chat turn streams the model's reply", async (t) => {
  await withCli(t, { ollama: { reply: REPLY } }, async ({ term, ollama }) => {
    await ready(term);
    await term.submit("hello from the e2e suite");
    await term.getByText(REPLY).expect({ timeout: 30_000 });

    // The agent also makes non-streaming side calls (e.g. ghost-text
    // suggestions); the turn itself is the streaming request.
    const turn = ollama.chatRequests().find((r) => r.body?.stream === true);
    assert.ok(turn, "agent should have streamed a /api/chat turn");
    assert.equal(turn.body.model, "fake-model");
    for (const request of ollama.chatRequests()) {
      assert.equal(request.body.model, "fake-model", "side calls must also stay on the fake model");
    }
    const user = turn.body.messages.filter((m) => m.role === "user").at(-1);
    assert.equal(user.content, "hello from the e2e suite");
  });
});

test("Esc cancels a streaming turn", async (t) => {
  // 20 words at 200ms each (~4s): long enough to cancel well before the end.
  const words = Array.from({ length: 20 }, (_, i) => `w${i}`);
  words[words.length - 1] = "never-reached-sentinel";
  await withCli(t, { ollama: { reply: words.join(" "), chunkDelayMs: 200 } }, async ({ term }) => {
    await ready(term);
    await term.submit("stream something long");
    await term.getByText("w3").expect({ timeout: 30_000 });
    await term.press("Escape");
    await term.getByText("⊘ canceled").expect();
    await ready(term);
    // Past the point where the stream would have finished uncancelled.
    await new Promise((r) => setTimeout(r, 5_000));
    await term.getByText("never-reached-sentinel").expect({ not: true, timeout: 1_000 });
  });
});
