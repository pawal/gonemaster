import { describe, it, expect, beforeEach } from "vitest";
import { get } from "svelte/store";
import { locale, t, setCatalog, loadCatalog } from "./i18n.js";

describe("i18n store", () => {
  beforeEach(() => {
    locale.set("en");
  });

  describe("English baseline", () => {
    it("returns the English string for a known key", () => {
      expect(get(t)("pub.app_title")).toBe("Gonemaster");
    });

    it("returns the raw key when absent from all catalogs", () => {
      expect(get(t)("pub.no_such_key_xyz")).toBe("pub.no_such_key_xyz");
    });

    it("preserves {placeholder} literals when no vars are passed", () => {
      expect(get(t)("pub.progress_testing")).toBe("Testing {domain}…");
    });

    it("all pub.* keys in en.json are non-empty strings", async () => {
      const en = (await import("./i18n/en.json")).default;
      for (const [key, val] of Object.entries(en)) {
        expect(typeof val, `key "${key}"`).toBe("string");
        expect(val.length, `key "${key}" must be non-empty`).toBeGreaterThan(0);
      }
    });

    it("all en.json keys start with 'pub.'", async () => {
      const en = (await import("./i18n/en.json")).default;
      for (const key of Object.keys(en)) {
        expect(key, `"${key}" should start with pub.`).toMatch(/^pub\./);
      }
    });
  });

  describe("interpolation", () => {
    it("replaces a {domain} placeholder", () => {
      expect(get(t)("pub.progress_testing", { domain: "example.com" }))
        .toBe("Testing example.com…");
    });

    it("replaces {seconds} in the rate-limit error", () => {
      expect(get(t)("pub.error_rate_limited", { seconds: 42 }))
        .toBe("Too many requests - please wait 42 seconds.");
    });

    it("coerces non-string var values to strings", () => {
      expect(get(t)("pub.progress_testing", { domain: 123 }))
        .toBe("Testing 123…");
    });

    it("ignores extra vars with no matching placeholder", () => {
      expect(get(t)("pub.app_title", { unused: "x" })).toBe("Gonemaster");
    });

    it("leaves unmatched placeholders literal when var is missing", () => {
      expect(get(t)("pub.progress_testing", {})).toBe("Testing {domain}…");
    });
  });

  describe("setCatalog and locale switching", () => {
    it("returns translated string after injecting a catalog", () => {
      setCatalog("xx", { "pub.app_title": "Gonemaster XX" });
      locale.set("xx");
      expect(get(t)("pub.app_title")).toBe("Gonemaster XX");
    });

    it("falls back to English for keys absent from the loaded catalog", () => {
      setCatalog("yy", { "pub.other_key": "something" });
      locale.set("yy");
      expect(get(t)("pub.app_title")).toBe("Gonemaster");
    });

    it("returns the raw key when missing from both catalog and English", () => {
      setCatalog("zz", {});
      locale.set("zz");
      expect(get(t)("pub.completely_missing_key_zz")).toBe("pub.completely_missing_key_zz");
    });

    it("reverts to English strings when locale is reset to en", () => {
      setCatalog("ww", { "pub.app_title": "Gonemaster WW" });
      locale.set("ww");
      locale.set("en");
      expect(get(t)("pub.app_title")).toBe("Gonemaster");
    });

    it("interpolates using the translated string, not the English one", () => {
      setCatalog("vv", { "pub.progress_testing": "Teste {domain}…" });
      locale.set("vv");
      expect(get(t)("pub.progress_testing", { domain: "example.com" }))
        .toBe("Teste example.com…");
    });

    it("t updates reactively when locale changes", () => {
      setCatalog("uu", { "pub.app_title": "Gonemaster UU" });
      locale.set("en");
      const first = get(t)("pub.app_title");
      locale.set("uu");
      const second = get(t)("pub.app_title");
      expect(first).toBe("Gonemaster");
      expect(second).toBe("Gonemaster UU");
    });
  });

  describe("loadCatalog", () => {
    it("is a no-op for 'en' (always bundled)", async () => {
      await expect(loadCatalog("en")).resolves.toBeUndefined();
      expect(get(t)("pub.app_title")).toBe("Gonemaster");
    });

    it("is a no-op for a locale already in the catalog", async () => {
      setCatalog("qq", { "pub.app_title": "Gonemaster QQ" });
      await expect(loadCatalog("qq")).resolves.toBeUndefined();
      locale.set("qq");
      expect(get(t)("pub.app_title")).toBe("Gonemaster QQ");
    });

    it("silently ignores a locale whose JSON file does not exist", async () => {
      await expect(loadCatalog("surely-not-a-real-locale-xyzzy")).resolves.toBeUndefined();
      locale.set("surely-not-a-real-locale-xyzzy");
      expect(get(t)("pub.app_title")).toBe("Gonemaster");
    });
  });
});
