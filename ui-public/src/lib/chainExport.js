// Serializing the chain diagram to a standalone SVG file. The drawing's rules
// live in the page's own stylesheet, so they are read from the CSSOM rather
// than duplicated here, and the theme tokens are resolved at export time,
// which is what makes the file readable outside the application.

// The class prefixes the diagram draws with. A rule naming none of them
// belongs to the page, not to the drawing. Status selectors carry a prefix of
// their own, since none of them names the base class it decorates.
export const CLASS_HINTS = ["chain-", "node-", "edge-", "frame-", "mark-"];

export const EXPORT_TOKENS = [
  "--surface",
  "--surface-2",
  "--ink",
  "--ink-2",
  "--muted",
  "--border",
  "--accent",
  "--accent-2",
  "--grade-a",
  "--grade-c",
  "--grade-f",
  "--sans",
  "--mono",
];

export function readTokens(root) {
  if (!root || typeof globalThis.getComputedStyle !== "function") return {};
  const computed = globalThis.getComputedStyle(root);
  const out = {};
  for (const token of EXPORT_TOKENS) out[token] = String(computed.getPropertyValue(token) ?? "").trim();
  return out;
}

export function collectStyles(sheets, tokens) {
  const declarations = Object.entries(tokens ?? {})
    .filter(([, value]) => value !== "")
    .map(([name, value]) => `  ${name}: ${value};`)
    .join("\n");
  const rules = [];
  for (const sheet of sheets ?? []) {
    let list;
    try {
      list = sheet.cssRules;
    } catch {
      // A stylesheet from another origin may not be read; skip it.
      continue;
    }
    for (const rule of list ?? []) {
      const selector = rule.selectorText;
      if (typeof selector !== "string") continue;
      if (CLASS_HINTS.some((hint) => selector.includes(hint))) rules.push(rule.cssText);
    }
  }
  return `:root {\n${declarations}\n}\n${rules.join("\n")}`;
}

// serializeSVG returns a standalone document. The element on the page is
// cloned and stamped with the drawing's own size, so the file is the diagram
// at rest whatever the card did to fit it.
export function serializeSVG(element, options = {}) {
  if (!element) return "";
  const { tokens, sheets = globalThis.document?.styleSheets ?? [], width, height } = options;
  const resolved = tokens ?? readTokens(globalThis.document?.documentElement);

  const clone = element.cloneNode(true);
  clone.setAttribute("xmlns", "http://www.w3.org/2000/svg");
  if (width && height) {
    clone.setAttribute("viewBox", `0 0 ${width} ${height}`);
    clone.setAttribute("width", String(width));
    clone.setAttribute("height", String(height));
  }
  const style = globalThis.document.createElementNS("http://www.w3.org/2000/svg", "style");
  style.textContent = collectStyles(sheets, resolved);
  clone.insertBefore(style, clone.firstChild);

  return `<?xml version="1.0" encoding="UTF-8"?>\n${new globalThis.XMLSerializer().serializeToString(clone)}`;
}

// chainFileName names the saved file for the zone. The root has no name of its
// own, and a separator would be read as a path.
export function chainFileName(domain) {
  const name = String(domain ?? "")
    .replace(/\.$/, "")
    .replaceAll("/", "_");
  return `${name || "root"}-dnssec-chain.svg`;
}

export function downloadSVG(text, filename) {
  if (!text) return;
  const url = globalThis.URL.createObjectURL(new Blob([text], { type: "image/svg+xml;charset=utf-8" }));
  const anchor = globalThis.document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  globalThis.URL.revokeObjectURL(url);
}
