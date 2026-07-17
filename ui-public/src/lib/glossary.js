import en from "../i18n/en.json";

const PREFIX = "pub.glossary.";
const MATCH_SUFFIX = ".match";

// Slugs of every glossary entry, in catalog order.
export function glossarySlugs(catalog = en) {
  const slugs = [];
  for (const key of Object.keys(catalog)) {
    if (key.startsWith(PREFIX) && key.endsWith(MATCH_SUFFIX)) {
      slugs.push(key.slice(PREFIX.length, -MATCH_SUFFIX.length));
    }
  }
  return slugs;
}

// Resolve each slug's match phrases for the active locale via t().
export function buildEntries(t, slugs = glossarySlugs()) {
  const entries = [];
  for (const slug of slugs) {
    const key = `${PREFIX}${slug}${MATCH_SUFFIX}`;
    const raw = t(key);
    if (!raw || raw === key) continue;
    const phrases = raw.split(",").map((s) => s.trim()).filter(Boolean);
    if (phrases.length) entries.push({ slug, phrases });
  }
  return entries;
}

function escapeRe(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// Acronyms match case-sensitively; other phrases case-insensitively.
function isAcronym(phrase) {
  return /^[A-Z0-9]{2,}$/.test(phrase);
}

// Split text into text/term segments, linking the first occurrence of
// each term. Longer phrases win over shorter ones; matches respect word
// boundaries.
export function linkify(text, entries) {
  if (!text || !entries || !entries.length) {
    return [{ type: "text", value: text ?? "" }];
  }
  const items = [];
  for (const e of entries) {
    for (const phrase of e.phrases) {
      items.push({ slug: e.slug, phrase, cs: isAcronym(phrase) });
    }
  }
  items.sort((a, b) => b.phrase.length - a.phrase.length);
  const pattern = items.map((i) => escapeRe(i.phrase)).join("|");
  // Unicode-aware boundaries so accented terms (e.g. French "condense"
  // with an accent, Czech "podpis") match even when a term ends in a
  // non-ASCII letter, where ASCII \b would place no boundary.
  const re = new RegExp(`(?<![\\p{L}\\p{N}])(?:${pattern})(?![\\p{L}\\p{N}])`, "giu");

  const used = new Set();
  const segments = [];
  let last = 0;
  for (const m of text.matchAll(re)) {
    const matched = m[0];
    const item = items.find((i) =>
      i.cs
        ? matched === i.phrase
        : matched.toLowerCase() === i.phrase.toLowerCase(),
    );
    if (!item || used.has(item.slug)) continue;
    used.add(item.slug);
    if (m.index > last) {
      segments.push({ type: "text", value: text.slice(last, m.index) });
    }
    segments.push({ type: "term", value: matched, slug: item.slug });
    last = m.index + matched.length;
  }
  if (last < text.length) {
    segments.push({ type: "text", value: text.slice(last) });
  }
  return segments;
}
