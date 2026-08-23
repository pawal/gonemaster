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
  it.each([
    ["?", "help-toggle"],
    ["g", "set-g"],
    ["n", "new-scan"],
    ["/", "focus-filter"],
    ["j", "row-next"],
    ["ArrowDown", "row-next"],
    ["k", "row-prev"],
    ["ArrowUp", "row-prev"],
    ["Escape", "close"],
    ["x", "none"],
    ["1", "none"]
  ])("resolves %s to %s", (key, type) => {
    expect(resolveShortcut(ev(key))).toEqual({ type });
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
