import { describe, it, expect } from "vitest";
import { KEY_MAP, isEditableTarget, resolveShortcut } from "./shortcuts.js";

// Minimal event stub; only the fields resolveShortcut reads.
const ev = (key, overrides = {}) => ({
  key,
  target: null,
  isComposing: false,
  ctrlKey: false,
  metaKey: false,
  altKey: false,
  shiftKey: false,
  ...overrides,
});

const el = (tagName, extra = {}) => ({ tagName, isContentEditable: false, ...extra });

describe("KEY_MAP", () => {
  it("maps the g-sequence second keys to tab ids", () => {
    expect(KEY_MAP).toEqual({
      s: "single",
      r: "recent",
      d: "domains",
      t: "tags",
      c: "cohorts",
      b: "batches",
      m: "metrics",
      ",": "settings",
    });
  });
});

describe("isEditableTarget", () => {
  it("returns true for input, textarea and select", () => {
    expect(isEditableTarget(el("INPUT"))).toBe(true);
    expect(isEditableTarget(el("textarea"))).toBe(true);
    expect(isEditableTarget(el("Select"))).toBe(true);
  });

  it("returns true for contenteditable elements", () => {
    expect(isEditableTarget(el("DIV", { isContentEditable: true }))).toBe(true);
  });

  it("returns false for non-editable elements and null", () => {
    expect(isEditableTarget(el("BUTTON"))).toBe(false);
    expect(isEditableTarget(el("DIV"))).toBe(false);
    expect(isEditableTarget(null)).toBe(false);
  });
});

describe("resolveShortcut guards", () => {
  it("ignores keystrokes while typing in an editable target", () => {
    expect(resolveShortcut(ev("n", { target: el("INPUT") }))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("g", { target: el("TEXTAREA") }))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("/", { target: el("DIV", { isContentEditable: true }) }))).toEqual({
      type: "none",
    });
  });

  it("ignores Ctrl/Meta/Alt chords so browser shortcuts are never hijacked", () => {
    expect(resolveShortcut(ev("n", { ctrlKey: true }))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("n", { metaKey: true }))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("n", { altKey: true }))).toEqual({ type: "none" });
  });

  it("ignores keystrokes during IME composition", () => {
    expect(resolveShortcut(ev("n", { isComposing: true }))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("?", { isComposing: true, shiftKey: true }))).toEqual({ type: "none" });
  });

  it("allows Shift, which is required for '?'", () => {
    expect(resolveShortcut(ev("?", { shiftKey: true }))).toEqual({ type: "help-toggle" });
  });
});

describe("resolveShortcut single keys", () => {
  it("toggles the help overlay on '?'", () => {
    expect(resolveShortcut(ev("?"))).toEqual({ type: "help-toggle" });
  });

  it("arms the g-prefix on 'g'", () => {
    expect(resolveShortcut(ev("g"))).toEqual({ type: "set-g" });
  });

  it("starts a new scan on 'n'", () => {
    expect(resolveShortcut(ev("n"))).toEqual({ type: "new-scan" });
  });

  it("focuses the filter on '/'", () => {
    expect(resolveShortcut(ev("/"))).toEqual({ type: "focus-filter" });
  });

  it("moves to the next row on 'j' and ArrowDown", () => {
    expect(resolveShortcut(ev("j"))).toEqual({ type: "row-next" });
    expect(resolveShortcut(ev("ArrowDown"))).toEqual({ type: "row-next" });
  });

  it("moves to the previous row on 'k' and ArrowUp", () => {
    expect(resolveShortcut(ev("k"))).toEqual({ type: "row-prev" });
    expect(resolveShortcut(ev("ArrowUp"))).toEqual({ type: "row-prev" });
  });

  it("closes on Escape", () => {
    expect(resolveShortcut(ev("Escape"))).toEqual({ type: "close" });
  });

  it("returns none for unmapped keys", () => {
    expect(resolveShortcut(ev("x"))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("1"))).toEqual({ type: "none" });
  });
});

describe("resolveShortcut g-sequence", () => {
  it("resolves a mapped second key to a navigate action", () => {
    expect(resolveShortcut(ev("s"), { pendingG: true })).toEqual({ type: "navigate", tab: "single" });
    expect(resolveShortcut(ev("r"), { pendingG: true })).toEqual({ type: "navigate", tab: "recent" });
    expect(resolveShortcut(ev(","), { pendingG: true })).toEqual({ type: "navigate", tab: "settings" });
  });

  it("disarms on an unmapped second key", () => {
    expect(resolveShortcut(ev("x"), { pendingG: true })).toEqual({ type: "clear-g" });
    expect(resolveShortcut(ev("g"), { pendingG: true })).toEqual({ type: "clear-g" });
  });

  it("still honours the editable guard while armed", () => {
    expect(resolveShortcut(ev("s", { target: el("INPUT") }), { pendingG: true })).toEqual({
      type: "none",
    });
  });
});

describe("resolveShortcut with the help overlay open", () => {
  it("suppresses every action except Escape", () => {
    expect(resolveShortcut(ev("n"), { overlayOpen: true })).toEqual({ type: "none" });
    expect(resolveShortcut(ev("g"), { overlayOpen: true })).toEqual({ type: "none" });
    expect(resolveShortcut(ev("?"), { overlayOpen: true, shiftKey: true })).toEqual({ type: "none" });
    expect(resolveShortcut(ev("j"), { overlayOpen: true })).toEqual({ type: "none" });
  });

  it("closes on Escape", () => {
    expect(resolveShortcut(ev("Escape"), { overlayOpen: true })).toEqual({ type: "close" });
  });
});
