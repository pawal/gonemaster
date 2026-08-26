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

// The server substitutes these per request (server/public/public.go). Losing one
// ships a literal placeholder, or a result page with no summary in it.
describe("index.html server hooks", () => {
  it.each([
    "__PAGE_TITLE__",
    "__PAGE_DESCRIPTION__",
    "__OG_URL__",
    "__PUBLIC_URL__",
    "<!-- CANONICAL -->",
    "<!-- ROBOTS_TAG -->",
    "<!-- HREFLANG_TAGS -->",
  ])("keeps the %s hook", (hook) => {
    expect(indexHtml.includes(hook)).toBe(true);
  });

  it("titles the page through the hook rather than hardcoding it", () => {
    expect(indexHtml).toMatch(/<title>__PAGE_TITLE__<\/title>/);
  });

  it("takes og:title, og:description and twitter equivalents from the hooks", () => {
    const tagged = [
      ...indexHtml.matchAll(
        /(?:property|name)="(?:og|twitter):(title|description)" content="([^"]*)"/g
      ),
    ];
    expect(tagged.length).toBe(4);
    for (const [, kind, value] of tagged) {
      expect(value).toBe(kind === "title" ? "__PAGE_TITLE__" : "__PAGE_DESCRIPTION__");
    }
  });

  it("wraps the generic noscript body in the markers the server cuts on", () => {
    const block = noscriptBlock();
    const start = block.indexOf("<!-- NOSCRIPT_GENERIC_START -->");
    const end = block.indexOf("<!-- NOSCRIPT_GENERIC_END -->");
    const summary = block.indexOf("<!-- RESULT_SUMMARY -->");
    expect(summary).toBeGreaterThan(-1);
    expect(start).toBeGreaterThan(summary);
    expect(end).toBeGreaterThan(start);
    expect(block.slice(start, end)).toMatch(/needs JavaScript/);
  });
});
