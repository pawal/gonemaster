// Completeness guard for the ui-public explanation-layer translations.
// Nothing in the JSON i18n pipeline tracks these dotted keys otherwise
// (missing-keys.sh only reads the engine share/lang/*.po catalogs), so
// this test is the parity check for the glossary as locales are filled in.
//
// A locale is treated as "not started" for the glossary until it defines
// at least one pub.glossary.* key. Once started, it must define the whole
// set with non-empty values.

import { describe, expect, it } from "vitest";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const i18nDir = resolve(here, "..", "i18n");

const en = JSON.parse(readFileSync(resolve(i18nDir, "en.json"), "utf8"));
const glossaryKeys = Object.keys(en).filter((k) => k.startsWith("pub.glossary."));

const locales = readdirSync(i18nDir)
  .filter((f) => f.endsWith(".json") && f !== "en.json")
  .map((f) => f.replace(/\.json$/, ""));

describe("ui-public glossary translations", () => {
  it("en defines both a definition and a match key per glossary slug", () => {
    const defs = glossaryKeys.filter((k) => !k.endsWith(".match"));
    const matches = glossaryKeys.filter((k) => k.endsWith(".match"));
    expect(defs.length).toBe(matches.length);
    expect(defs.length).toBeGreaterThan(0);
  });

  for (const loc of locales) {
    const cat = JSON.parse(readFileSync(resolve(i18nDir, `${loc}.json`), "utf8"));
    const present = glossaryKeys.filter((k) => k in cat);

    it(`${loc}: glossary is either complete or not started`, () => {
      if (present.length === 0) return; // not translated yet
      const missing = glossaryKeys.filter((k) => !(k in cat));
      expect(missing).toEqual([]);
    });

    it(`${loc}: glossary values are non-empty strings`, () => {
      for (const k of present) {
        expect(typeof cat[k]).toBe("string");
        expect(cat[k].trim().length).toBeGreaterThan(0);
      }
    });
  }
});
