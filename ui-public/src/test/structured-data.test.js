// Guards the JSON-LD structured-data block in index.html; the policy the
// assertions enforce is documented next to the block itself.

import { describe, expect, it } from "vitest";
import { readFileSync, readdirSync } from "node:fs";
import { basename, dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const indexHtml = readFileSync(resolve(here, "..", "..", "index.html"), "utf8");

function extractJsonLd() {
  const match = indexHtml.match(
    /<script type="application\/ld\+json">([\s\S]*?)<\/script>/
  );
  return match ? match[1] : null;
}

function parseGraph() {
  const raw = extractJsonLd().replaceAll(
    "__PUBLIC_URL__",
    "https://example.com/"
  );
  return JSON.parse(raw);
}

function appNode() {
  return parseGraph()["@graph"].find((node) =>
    [].concat(node["@type"]).includes("SoftwareApplication")
  );
}

function websiteNode() {
  return parseGraph()["@graph"].find((node) => node["@type"] === "WebSite");
}

describe("index.html JSON-LD", () => {
  it("has exactly one ld+json block", () => {
    const blocks = indexHtml.match(/application\/ld\+json/g) ?? [];
    expect(blocks.length).toBe(1);
  });

  it("uses the __PUBLIC_URL__ placeholder instead of hardcoded URLs", () => {
    const raw = extractJsonLd();
    expect(raw.includes("__PUBLIC_URL__")).toBe(true);
  });

  it("parses as valid JSON once the placeholder is substituted", () => {
    const data = parseGraph();
    expect(data["@context"]).toBe("https://schema.org");
    expect(Array.isArray(data["@graph"])).toBe(true);
    expect(data["@graph"].length).toBe(2);
  });

  it("multi-types the app node and links it to the website node", () => {
    const app = appNode();
    expect(app["@type"]).toEqual(["WebApplication", "SoftwareApplication"]);
    expect(app.isPartOf["@id"]).toBe(websiteNode()["@id"]);
  });

  it("joins the image URL onto the substituted public URL", () => {
    // resolvePublicURL guarantees a trailing slash, so plain
    // concatenation must yield a valid absolute URL.
    expect(appNode().image).toBe(
      "https://example.com/android-chrome-512x512.png"
    );
  });

  it("declares the app free without offers or aggregateRating", () => {
    const app = appNode();
    expect(app.isAccessibleForFree).toBe(true);
    expect("offers" in app).toBe(false);
    expect("aggregateRating" in app).toBe(false);
  });

  it("has no FAQPage node while the UI has no visible FAQ", () => {
    const types = parseGraph()
      ["@graph"].flatMap((node) => [].concat(node["@type"]));
    expect(types.includes("FAQPage")).toBe(false);
  });

  it("keeps inLanguage in sync with the shipped locales", () => {
    const locales = readdirSync(resolve(here, "..", "i18n"))
      .filter((name) => name.endsWith(".json"))
      .map((name) => basename(name, ".json"))
      .sort();
    const inLanguage = [...websiteNode().inLanguage].sort();
    expect(inLanguage).toEqual(locales);
  });
});

describe("index.html social meta images", () => {
  // og:image and twitter:image must be absolute URLs for link scrapers,
  // so they build on __PUBLIC_URL__ rather than a root-relative path.
  it("build og:image and twitter:image on the __PUBLIC_URL__ placeholder", () => {
    const images = [
      ...indexHtml.matchAll(
        /(?:property="og:image"|name="twitter:image") content="([^"]*)"/g
      ),
    ].map((match) => match[1]);
    expect(images.length).toBe(2);
    for (const image of images) {
      expect(image).toBe("__PUBLIC_URL__android-chrome-512x512.png");
    }
  });
});
