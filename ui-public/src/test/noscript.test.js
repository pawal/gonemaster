// With JavaScript off, this block is the whole page: the body is otherwise
// just <div id="app"></div>. Easy to lose in a refactor without anyone
// noticing in a normal browser, so pin the parts that matter.

import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const indexHtml = readFileSync(resolve(here, "..", "..", "index.html"), "utf8");

function noscriptBlock() {
  const match = indexHtml.match(/<noscript>([\s\S]*?)<\/noscript>/);
  return match ? match[1] : null;
}

describe("index.html noscript fallback", () => {
  it("has exactly one noscript block", () => {
    const blocks = indexHtml.match(/<noscript>/g) ?? [];
    expect(blocks.length).toBe(1);
  });

  it("places the fallback in the body, before the app mount point", () => {
    const noscriptAt = indexHtml.indexOf("<noscript>");
    const appAt = indexHtml.indexOf('<div id="app">');
    const bodyAt = indexHtml.indexOf("<body>");
    expect(noscriptAt).toBeGreaterThan(bodyAt);
    expect(noscriptAt).toBeLessThan(appAt);
  });

  it("names the product in a heading", () => {
    expect(noscriptBlock()).toMatch(/<h1>\s*Gonemaster\s*<\/h1>/);
  });

  it("says the interactive checker needs JavaScript", () => {
    expect(noscriptBlock()).toMatch(/needs JavaScript/);
  });

  it("offers the public API as the no-JavaScript alternative", () => {
    const block = noscriptBlock();
    expect(block).toMatch(/curl/);
    expect(block).toMatch(/pub\/api\/v1\/jobs/);
  });

  it("builds every API URL on the __PUBLIC_URL__ placeholder", () => {
    // A hardcoded host would give every other deployment wrong instructions.
    const urls = [...noscriptBlock().matchAll(/\S*pub\/api\/v1\/\S*/g)].map((m) => m[0]);
    expect(urls.length).toBeGreaterThan(0);
    for (const url of urls) {
      expect(url.startsWith("__PUBLIC_URL__")).toBe(true);
    }
  });

  it("links to the documentation and the source repository", () => {
    const block = noscriptBlock();
    expect(block).toMatch(/href="https:\/\/pawal\.codeberg\.page\/gonemaster\/"/);
    expect(block).toMatch(/href="https:\/\/codeberg\.org\/pawal\/gonemaster"/);
  });

  it("carries no inline style attribute", () => {
    // style-src 'self' has no 'unsafe-inline'.
    expect(/\sstyle="/.test(noscriptBlock())).toBe(false);
  });

  it("carries no script tag, which would never run here anyway", () => {
    expect(/<script/.test(noscriptBlock())).toBe(false);
  });
});
