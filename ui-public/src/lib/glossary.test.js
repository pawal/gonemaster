import { describe, expect, it } from "vitest";
import { glossarySlugs, buildEntries, linkify } from "./glossary.js";

// A tiny fake catalog + t() so the tests are independent of the real
// en.json contents. Entries mirror the shape the real sync produces.
const CATALOG = {
  "pub.glossary.dnssec": "DNSSEC adds signatures.",
  "pub.glossary.dnssec.match": "DNSSEC",
  "pub.glossary.ds": "A DS record fingerprints a key.",
  "pub.glossary.ds.match": "DS",
  "pub.glossary.nsec": "NSEC proves non-existence.",
  "pub.glossary.nsec.match": "NSEC",
  "pub.glossary.nsec3": "NSEC3 hashes the names.",
  "pub.glossary.nsec3.match": "NSEC3",
  "pub.glossary.glue": "Glue is a nameserver address.",
  "pub.glossary.glue.match": "glue",
  "pub.glossary.zone-apex": "The zone apex is the top of the domain.",
  "pub.glossary.zone-apex.match": "zone apex, apex",
};

function fakeT() {
  return (key) => CATALOG[key] ?? key;
}

function entriesFrom(catalog) {
  const slugs = glossarySlugs(catalog);
  return buildEntries((key) => catalog[key] ?? key, slugs);
}

const ENTRIES = entriesFrom(CATALOG);

// Helper: reconstruct the plain string from segments (round-trips text).
function flatten(segments) {
  return segments.map((s) => s.value).join("");
}

describe("glossarySlugs", () => {
  it("returns one slug per .match key, keeping hyphens", () => {
    expect(glossarySlugs(CATALOG)).toContain("zone-apex");
    expect(glossarySlugs(CATALOG)).toContain("dnssec");
    // Definition-only keys (without .match) are not slugs.
    expect(glossarySlugs(CATALOG)).not.toContain("dnssec.match");
  });

  it("reads real catalog keys", () => {
    // The bundled en.json should carry glossary entries after syncing.
    expect(glossarySlugs().length).toBeGreaterThan(0);
  });
});

describe("buildEntries", () => {
  it("splits comma-separated match phrases", () => {
    const apex = entriesFrom(CATALOG).find((e) => e.slug === "zone-apex");
    expect(apex.phrases).toEqual(["zone apex", "apex"]);
  });

  it("skips slugs whose .match key is missing", () => {
    const t = (key) => (key === "pub.glossary.ds.match" ? key : CATALOG[key] ?? key);
    const built = buildEntries(t, glossarySlugs(CATALOG));
    expect(built.find((e) => e.slug === "ds")).toBeUndefined();
  });
});

describe("linkify", () => {
  it("wraps a known term and preserves the surrounding text", () => {
    const segs = linkify("Enable DNSSEC today.", ENTRIES);
    const term = segs.find((s) => s.type === "term");
    expect(term.value).toBe("DNSSEC");
    expect(term.slug).toBe("dnssec");
    expect(flatten(segs)).toBe("Enable DNSSEC today.");
  });

  it("links only the first occurrence of a term", () => {
    const segs = linkify("DNSSEC here and DNSSEC there.", ENTRIES);
    const terms = segs.filter((s) => s.type === "term");
    expect(terms.length).toBe(1);
    expect(flatten(segs)).toBe("DNSSEC here and DNSSEC there.");
  });

  it("prefers the longer phrase (NSEC3 over NSEC)", () => {
    const segs = linkify("The NSEC3 record.", ENTRIES);
    const term = segs.find((s) => s.type === "term");
    expect(term.value).toBe("NSEC3");
    expect(term.slug).toBe("nsec3");
  });

  it("prefers the longer phrase (zone apex over apex)", () => {
    const segs = linkify("At the zone apex.", ENTRIES);
    const term = segs.find((s) => s.type === "term");
    expect(term.value).toBe("zone apex");
    expect(term.slug).toBe("zone-apex");
  });

  it("maps a shorter alias to the same slug when the phrase stands alone", () => {
    const segs = linkify("The apex record.", ENTRIES);
    const term = segs.find((s) => s.type === "term");
    expect(term.value).toBe("apex");
    expect(term.slug).toBe("zone-apex");
  });

  it("respects word boundaries for acronyms (no DS inside DNSSEC)", () => {
    // "DNSSEC" contains neither a standalone DS nor NSEC, so only DNSSEC links.
    const segs = linkify("Just DNSSEC.", ENTRIES);
    const terms = segs.filter((s) => s.type === "term");
    expect(terms.length).toBe(1);
    expect(terms[0].slug).toBe("dnssec");
  });

  it("does not match an acronym in the wrong case", () => {
    // Lowercase "ds" must not link the DS acronym.
    const segs = linkify("The lands and roads.", ENTRIES);
    expect(segs.filter((s) => s.type === "term").length).toBe(0);
    expect(flatten(segs)).toBe("The lands and roads.");
  });

  it("matches lower-case words case-insensitively", () => {
    const capitalised = linkify("Glue is required.", ENTRIES);
    expect(capitalised.find((s) => s.type === "term").value).toBe("Glue");
    const lower = linkify("Missing glue here.", ENTRIES);
    expect(lower.find((s) => s.type === "term").value).toBe("glue");
  });

  it("returns the text unchanged when nothing matches", () => {
    const segs = linkify("Nothing to see here.", ENTRIES);
    expect(segs).toEqual([{ type: "text", value: "Nothing to see here." }]);
  });

  it("returns a single text segment for empty entries", () => {
    expect(linkify("Some text.", [])).toEqual([
      { type: "text", value: "Some text." },
    ]);
  });

  it("handles empty or missing input", () => {
    expect(linkify("", ENTRIES)).toEqual([{ type: "text", value: "" }]);
    expect(linkify(undefined, ENTRIES)).toEqual([{ type: "text", value: "" }]);
  });

  it("matches accented terms that end in a non-ASCII letter", () => {
    // Simulates a French-style phrase list (glue -> "colle", digest ->
    // "condense" with an accent) matched against accented prose.
    const accented = [
      { slug: "glue", phrases: ["colle"] },
      { slug: "digest", phrases: ["condensé"] },
      { slug: "authoritative", phrases: ["faisant autorité"] },
    ];
    const segs = linkify("La colle et le condensé faisant autorité ici.", accented);
    const slugs = segs.filter((s) => s.type === "term").map((s) => s.slug);
    expect(slugs).toEqual(["glue", "digest", "authoritative"]);
    expect(flatten(segs)).toBe("La colle et le condensé faisant autorité ici.");
  });

  it("does not match an accented term embedded in a longer word", () => {
    const entries = [{ slug: "digest", phrases: ["condensé"] }];
    // "condensée" (feminine) should not match the masculine "condensé".
    const segs = linkify("une valeur condensée ici", entries);
    expect(segs.filter((s) => s.type === "term").length).toBe(0);
  });

  it("matches Latin acronyms and terms flanked by CJK characters", () => {
    // Japanese-style prose has no spaces; a term sits directly against
    // Japanese letters on both sides and must still match.
    const jp = [
      { slug: "dnssec", phrases: ["DNSSEC"] },
      { slug: "glue", phrases: ["グルー"] },
    ];
    const segs = linkify("これはDNSSECとグルーの説明です。", jp);
    const slugs = segs.filter((s) => s.type === "term").map((s) => s.slug);
    expect(slugs).toEqual(["dnssec", "glue"]);
    expect(flatten(segs)).toBe("これはDNSSECとグルーの説明です。");
  });

  it("links several distinct terms in one string", () => {
    const segs = linkify("DNSSEC uses a DS and NSEC3.", ENTRIES);
    const slugs = segs.filter((s) => s.type === "term").map((s) => s.slug);
    expect(slugs).toEqual(["dnssec", "ds", "nsec3"]);
    expect(flatten(segs)).toBe("DNSSEC uses a DS and NSEC3.");
  });
});
