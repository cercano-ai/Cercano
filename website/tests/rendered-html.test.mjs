import assert from "node:assert/strict";
import { readFile, stat } from "node:fs/promises";
import test from "node:test";

async function render() {
  const workerUrl = new URL("../dist/server/index.js", import.meta.url);
  workerUrl.searchParams.set("test", `${process.pid}-${Date.now()}`);
  const { default: worker } = await import(workerUrl.href);
  return worker.fetch(new Request("http://localhost/", { headers: { accept: "text/html" } }), {
    ASSETS: { fetch: async () => new Response("Not found", { status: 404 }) },
  }, { waitUntil() {}, passThroughOnException() {} });
}

test("renders the Cercano landing page", async () => {
  const response = await render();
  assert.equal(response.status, 200);
  assert.match(response.headers.get("content-type") ?? "", /^text\/html\b/i);
  const html = await response.text();
  assert.match(html, /The free and open source AI agent for any model/);
  assert.doesNotMatch(html, /Use the best model/);
  assert.match(html, /Make your tokens go further/);
  assert.match(html, /Never stop for compaction/);
  assert.match(html, /A complete agent, not a model wrapper/);
  assert.match(html, /Background compaction/);
  assert.match(html, /Headless automation/);
  assert.match(html, /Run a full-power agent on local models/);
  assert.match(html, /Use rich media models for specialized tasks/);
  assert.match(html, /The agent is yours/);
  assert.match(html, /free and open source under Apache 2\.0/);
  assert.match(html, /More capability. Less friction/);
  assert.match(html, /href="https:\/\/developer\.cercloud\.ai\/cercano\/routing\/"/);
  assert.match(html, /href="https:\/\/developer\.cercloud\.ai\/cercano\/multimodal\/"/);
  assert.doesNotMatch(html, /<video\b/);
  assert.match(html, /\/hero-tui\.png/);
  assert.doesNotMatch(html, /\/videos\/terminal-ui\.mp4/);
  assert.match(html, /Try Cercano/);
  assert.match(html, /Install in one line/);
  assert.match(html, /brew install cercano-ai\/cercano\/cercano-agent/);
  assert.match(html, /Homebrew/);
  assert.match(html, /Debian \+ Ubuntu/);
  assert.match(html, /amd64 only/);
  assert.match(html, /sudo apt install cercano/);
  assert.match(html, /choco install cercano -y/);
  assert.doesNotMatch(html, /Package publishing in progress|Coming soon/);
  assert.match(html, /href="https:\/\/developer\.cercloud\.ai\/getting-started\/install\/"/);
  assert.doesNotMatch(html, /github\.com\/cercano-ai\/Cercano\/blob\/main\/docs/);
  assert.match(html, /Switch to (daylight|night) theme/);
  assert.doesNotMatch(html, /codex-preview|Building your site|react-loading-skeleton/);
});

test("keeps the TUI palettes and persistent theme behavior", async () => {
  const [css, page, layout] = await Promise.all([
    readFile(new URL("../app/globals.css", import.meta.url), "utf8"),
    readFile(new URL("../app/page.tsx", import.meta.url), "utf8"),
    readFile(new URL("../app/layout.tsx", import.meta.url), "utf8"),
  ]);

  for (const color of ["#fbf3e0", "#3b3020", "#1e7a3c", "#1763a0", "#1a1a1a", "#ea8212", "#bdf000", "#00c8e8"]) {
    assert.match(css, new RegExp(color, "i"));
  }
  assert.match(css, /--font-mono:\s*"IBM Plex Mono"/);
  assert.match(css, /body\s*\{[^}]*font-family:\s*var\(--font-mono\)/s);
  assert.match(page, /className="brand-mark"[^>]*assetUrl\("\/icon\.svg"\)/);
  assert.match(css, /\.value-blocks\s*\{[^}]*grid-template-columns:\s*repeat\(3/s);
  assert.match(page, /localStorage\.setItem\("cercano-theme"/);
  assert.match(page, /navigator\.clipboard\.writeText\(command\)/);
  assert.match(page, /window\.navigator\.platform/);
  assert.match(page, /Other OS installation instructions/);
  assert.match(page, /copyInstallCommand\("brew install cercano-ai\/cercano\/cercano-agent cercano-ai\/cercano\/cercano-cli cercano-ai\/cercano\/cercano", "homebrew"\)/);
  assert.match(page, /copyInstallCommand\("choco install cercano -y", "chocolatey"\)/);
  assert.match(page, /copyInstallCommand\(aptCommand, "apt"\)/);
  assert.match(layout, /prefers-color-scheme:\s*dark/);
  assert.match(layout, /fonts\.googleapis\.com\/css2\?family=IBM\+Plex\+Mono:wght@400;500;600/);
});

test("shows current package availability and links the developer documentation", async () => {
  const page = await readFile(new URL("../app/page.tsx", import.meta.url), "utf8");
  assert.match(page, /choco install cercano -y/);
  assert.match(page, /Chocolatey[\s\S]*Available/);
  assert.match(page, /apt[\s\S]*Available/);
  assert.match(page, /Debian \+ Ubuntu · amd64 only/);
  assert.match(page, /sudo apt install cercano/);
  assert.doesNotMatch(page, /moderation pending|availability are not confirmed/i);
  assert.match(page, /developer\.cercloud\.ai/);
  assert.match(page, /getting-started\/install/);
  assert.match(page, /getting-started\/first-run/);
  assert.match(page, /troubleshooting/);
  assert.doesNotMatch(page, /github\.com\/cercano-ai\/Cercano\/blob\/main\/docs/);
});

test("ships all four README demonstrations and their poster frames", async () => {
  for (const name of ["delegation", "metrics", "terminal-ui", "sessions"]) {
    const [video, poster] = await Promise.all([
      stat(new URL(`../public/videos/${name}.mp4`, import.meta.url)),
      stat(new URL(`../public/videos/${name}.jpg`, import.meta.url)),
    ]);
    assert.ok(video.size > 100_000, `${name}.mp4 should contain the demonstration`);
    assert.ok(poster.size > 20_000, `${name}.jpg should contain a useful poster frame`);
  }
});
