import { describe, it, expect, beforeEach } from "vitest";
import { get } from "svelte/store";
import { locale, t, setCatalog, loadCatalog } from "./i18n.js";
import enCatalog from "./i18n/en.json";
import daCatalog from "./i18n/da.json";
import esCatalog from "./i18n/es.json";
import fiCatalog from "./i18n/fi.json";
import frCatalog from "./i18n/fr.json";
import jaCatalog from "./i18n/ja.json";
import nbCatalog from "./i18n/nb.json";
import slCatalog from "./i18n/sl.json";
import svCatalog from "./i18n/sv.json";

describe("i18n store", () => {
  beforeEach(() => {
    locale.set("en");
  });

  describe("English baseline", () => {
    it("returns the English string for a known key", () => {
      expect(get(t)("app_title")).toBe("Gonemaster");
    });

    it("returns the raw key when the key is absent from all catalogs", () => {
      expect(get(t)("no_such_key_xyz")).toBe("no_such_key_xyz");
    });

    it("preserves {placeholder} literals when no vars are passed", () => {
      expect(get(t)("job_created")).toBe("Job {id} created.");
    });
  });

  describe("interpolation", () => {
    it("replaces a single {var} placeholder", () => {
      expect(get(t)("job_created", { id: "abc-123" })).toBe("Job abc-123 created.");
    });

    it("replaces multiple {var} placeholders in one string", () => {
      expect(get(t)("batch_accepted", { id: "b-99" })).toBe("Batch b-99 accepted.");
    });

    it("coerces non-string var values to strings", () => {
      expect(get(t)("job_created", { id: 42 })).toBe("Job 42 created.");
    });

    it("ignores extra vars that have no matching placeholder", () => {
      expect(get(t)("app_title", { unused: "x" })).toBe("Gonemaster");
    });

    it("leaves unmatched placeholders literal when their var is missing", () => {
      // job_created = "Job {id} created." - if we omit `id` the template is preserved.
      expect(get(t)("job_created", {})).toBe("Job {id} created.");
    });
  });

  describe("setCatalog and locale switching", () => {
    it("returns translated string after injecting a catalog", () => {
      setCatalog("xx", { app_title: "Gonemaster XX" });
      locale.set("xx");
      expect(get(t)("app_title")).toBe("Gonemaster XX");
    });

    it("falls back to English for keys absent from the loaded catalog", () => {
      setCatalog("yy", { other_key: "something" });
      locale.set("yy");
      // app_title not in yy → English fallback
      expect(get(t)("app_title")).toBe("Gonemaster");
    });

    it("returns the raw key when missing from both catalog and English", () => {
      setCatalog("zz", {});
      locale.set("zz");
      expect(get(t)("completely_missing_key_zz")).toBe("completely_missing_key_zz");
    });

    it("reverts to English strings when locale is reset to en", () => {
      setCatalog("ww", { app_title: "Gonemaster WW" });
      locale.set("ww");
      locale.set("en");
      expect(get(t)("app_title")).toBe("Gonemaster");
    });

    it("interpolates vars using the translated string, not the English one", () => {
      setCatalog("vv", { job_created: "Jobb {id} opprettet." });
      locale.set("vv");
      expect(get(t)("job_created", { id: "j-1" })).toBe("Jobb j-1 opprettet.");
    });

    it("t updates reactively when locale changes", () => {
      setCatalog("uu", { app_title: "Gonemaster UU" });
      locale.set("en");
      const first = get(t)("app_title");
      locale.set("uu");
      const second = get(t)("app_title");
      expect(first).toBe("Gonemaster");
      expect(second).toBe("Gonemaster UU");
    });
  });

  describe("loadCatalog", () => {
    it("is a no-op for 'en' (always bundled)", async () => {
      // Should resolve without error and leave the English catalog intact.
      await expect(loadCatalog("en")).resolves.toBeUndefined();
      expect(get(t)("app_title")).toBe("Gonemaster");
    });

    it("is a no-op for a locale already in the catalog", async () => {
      setCatalog("qq", { app_title: "Gonemaster QQ" });
      // Second call should not overwrite.
      await expect(loadCatalog("qq")).resolves.toBeUndefined();
      locale.set("qq");
      expect(get(t)("app_title")).toBe("Gonemaster QQ");
    });

    it("silently ignores a locale whose JSON file does not exist", async () => {
      // The dynamic import will fail; loadCatalog must not throw.
      await expect(loadCatalog("surely-not-a-real-locale-xyzzy")).resolves.toBeUndefined();
      // Locale stays on English after the failed load.
      locale.set("surely-not-a-real-locale-xyzzy");
      expect(get(t)("app_title")).toBe("Gonemaster"); // English fallback
    });
  });

  describe("translation completeness", () => {
    // app_title is intentionally absent from non-English catalogs (brand name).
    const SKIP = new Set(["app_title"]);
    const enKeys = Object.keys(enCatalog).filter((k) => !SKIP.has(k));

    const catalogs = { da: daCatalog, es: esCatalog, fi: fiCatalog, fr: frCatalog, ja: jaCatalog, nb: nbCatalog, sl: slCatalog, sv: svCatalog };

    for (const [lang, catalog] of Object.entries(catalogs)) {
      it(`${lang} catalog contains all required keys`, () => {
        const missing = enKeys.filter((k) => !(k in catalog));
        expect(missing, `${lang} missing keys: ${missing.join(", ")}`).toHaveLength(0);
      });

      it(`${lang} translations have no empty values`, () => {
        const empty = enKeys.filter((k) => k in catalog && catalog[k] === "");
        expect(empty, `${lang} empty values for: ${empty.join(", ")}`).toHaveLength(0);
      });
    }
  });
});
