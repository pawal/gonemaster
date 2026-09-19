import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EXPORT_TOKENS, chainFileName, collectStyles, downloadSVG, readTokens, serializeSVG } from "./chainExport.js";

// A stand-in for one same-origin stylesheet.
const sheet = (...selectors) => ({
  cssRules: selectors.map((selectorText) => ({ selectorText, cssText: `${selectorText} { stroke: red; }` })),
});

const svgElement = () => {
  const svg = globalThis.document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 100 50");
  const rect = globalThis.document.createElementNS("http://www.w3.org/2000/svg", "rect");
  rect.setAttribute("class", "chain-node-box");
  svg.appendChild(rect);
  return svg;
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("collecting the styles an exported diagram needs", () => {
  it("writes the resolved tokens as a root block", () => {
    const css = collectStyles([], { "--grade-a": "#16a34a" });
    expect(css).toContain(":root {");
    expect(css).toContain("--grade-a: #16a34a;");
  });

  it("leaves out a token the page does not define", () => {
    const css = collectStyles([], { "--grade-a": "#16a34a", "--mono": "" });
    expect(css).not.toContain("--mono");
  });

  it("keeps the diagram rules and leaves the rest of the page behind", () => {
    const css = collectStyles([sheet(".chain-node-box", "header", ".chain-edge", "button:hover")], {});
    expect(css).toContain(".chain-node-box");
    expect(css).toContain(".chain-edge");
    expect(css).not.toContain("header");
    expect(css).not.toContain("button:hover");
  });

  // A status selector names no base class, so a hint for the base alone would
  // lose every colour and dash of a saved diagram.
  it("collects the tone, edge, frame and mark rules", () => {
    const css = collectStyles([sheet(".node-tone-bad", ".edge-warn", ".frame-ok", ".chain-frame-chip", ".mark-bad")], {});
    for (const selector of [".node-tone-bad", ".edge-warn", ".frame-ok", ".chain-frame-chip", ".mark-bad"]) {
      expect(css).toContain(selector);
    }
  });

  it("collects the severed stub and its tick", () => {
    const css = collectStyles([sheet(".chain-stub", ".chain-break-tick", ".node-orphan")], {});
    expect(css).toContain(".chain-stub");
    expect(css).toContain(".chain-break-tick");
    expect(css).toContain(".node-orphan");
  });

  // Svelte scopes a component's rules with a hash class, which the clone carries.
  it("keeps a rule the scoping hash has been appended to", () => {
    const css = collectStyles([sheet(".chain-node-box.svelte-1a2b3c")], {});
    expect(css).toContain(".chain-node-box.svelte-1a2b3c");
  });

  it("steps over a stylesheet it may not read", () => {
    const blocked = {
      get cssRules() {
        throw new Error("cross-origin");
      },
    };
    expect(collectStyles([blocked, sheet(".chain-edge")], {})).toContain(".chain-edge");
  });

  it("steps over a rule that carries no selector, such as a font face", () => {
    const css = collectStyles([{ cssRules: [{ cssText: "@font-face { }" }, ...sheet(".chain-node-box").cssRules] }], {});
    expect(css).toContain(".chain-node-box");
    expect(css).not.toContain("font-face");
  });
});

describe("reading the theme tokens", () => {
  it("resolves every token the diagram uses", () => {
    vi.spyOn(globalThis, "getComputedStyle").mockReturnValue({ getPropertyValue: () => "  #123456 " });
    const tokens = readTokens({ style: {} });

    expect(Object.keys(tokens)).toEqual([...EXPORT_TOKENS]);
    expect(tokens["--grade-f"]).toBe("#123456");
  });

  it("returns nothing when the document cannot be read", () => {
    expect(readTokens(null)).toEqual({});
  });
});

describe("serializing the diagram", () => {
  it("produces a standalone document carrying its own styles", () => {
    const text = serializeSVG(svgElement(), { tokens: { "--ink": "#000" }, sheets: [sheet(".chain-node-box")] });

    expect(text.startsWith("<?xml")).toBe(true);
    expect(text).toContain('xmlns="http://www.w3.org/2000/svg"');
    expect(text).toContain("<style");
    expect(text).toContain(".chain-node-box");
    expect(text).toContain("--ink: #000;");
    expect(text).toContain('class="chain-node-box"');
  });

  it("stamps the size asked for, so the file is the diagram at rest", () => {
    const text = serializeSVG(svgElement(), { width: 640, height: 480, sheets: [] });
    expect(text).toContain('viewBox="0 0 640 480"');
    expect(text).toContain('width="640"');
    expect(text).toContain('height="480"');
  });

  it("leaves the element on the page untouched", () => {
    const svg = svgElement();
    serializeSVG(svg, { width: 640, height: 480, sheets: [] });

    expect(svg.getAttribute("viewBox")).toBe("0 0 100 50");
    expect(svg.querySelector("style")).toBeNull();
  });

  it("returns nothing at all without an element", () => {
    expect(serializeSVG(null)).toBe("");
  });
});

describe("naming the saved file", () => {
  it.each([
    ["example.com", "example.com-dnssec-chain.svg"],
    ["example.com.", "example.com-dnssec-chain.svg"],
    [".", "root-dnssec-chain.svg"],
    ["", "root-dnssec-chain.svg"],
    ["a/b.example", "a_b.example-dnssec-chain.svg"]
  ])("names the file for zone %j %s", (zone, want) => {
    expect(chainFileName(zone)).toBe(want);
  });
});

describe("handing the diagram to the browser", () => {
  // jsdom has no object URLs, so each test gets fresh stubs and leaves none behind.
  beforeEach(() => {
    Object.defineProperty(globalThis.URL, "createObjectURL", { value: vi.fn(() => "blob:one"), configurable: true });
    Object.defineProperty(globalThis.URL, "revokeObjectURL", { value: vi.fn(), configurable: true });
  });

  afterEach(() => {
    delete globalThis.URL.createObjectURL;
    delete globalThis.URL.revokeObjectURL;
  });

  it("names the file and releases the object URL again", () => {
    const clicked = [];
    const anchor = globalThis.document.createElement("a");
    anchor.click = () => clicked.push({ href: anchor.href, download: anchor.download });
    vi.spyOn(globalThis.document, "createElement").mockReturnValue(anchor);

    downloadSVG("<svg/>", "example.com-dnssec-chain.svg");

    expect(globalThis.URL.createObjectURL).toHaveBeenCalledTimes(1);
    expect(clicked[0].download).toBe("example.com-dnssec-chain.svg");
    expect(globalThis.URL.revokeObjectURL).toHaveBeenCalledWith("blob:one");
  });

  it("does nothing with an empty diagram", () => {
    downloadSVG("", "empty.svg");
    expect(globalThis.URL.createObjectURL).not.toHaveBeenCalled();
  });
});
