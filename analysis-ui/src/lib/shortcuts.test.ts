import { describe, it, expect } from "vitest";
import { KEY_MAP, isEditableTarget, resolveShortcut, shortcutNavRows } from "./shortcuts";
import { navItems } from "./nav";

// Minimal event stub; only the fields resolveShortcut reads.
const ev = (key: string, overrides: Record<string, unknown> = {}) => ({
  key,
  target: null,
  isComposing: false,
  ctrlKey: false,
  metaKey: false,
  altKey: false,
  shiftKey: false,
  ...overrides
});

const el = (tagName: string, extra: Record<string, unknown> = {}) => ({
  tagName,
  isContentEditable: false,
  ...extra
});

describe("KEY_MAP", () => {
  it("maps the g-sequence second keys to nav section hrefs", () => {
    expect(KEY_MAP).toEqual({
      o: "/",
      c: "/cohorts",
      d: "/domains",
      n: "/nameservers",
      a: "/endpoints",
      s: "/asns",
      t: "/tags",
      r: "/trends",
      f: "/diff"
    });
  });

  it("only targets hrefs that exist in the nav", () => {
    for (const href of Object.values(KEY_MAP)) {
      expect(navItems.some((i) => i.href === href)).toBe(true);
    }
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
    expect(resolveShortcut(ev("g", { target: el("INPUT") }))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("g", { target: el("TEXTAREA") }))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("/", { target: el("DIV", { isContentEditable: true }) }))).toEqual({
      type: "none"
    });
  });

  it("ignores Ctrl/Meta/Alt chords so browser shortcuts are never hijacked", () => {
    expect(resolveShortcut(ev("g", { ctrlKey: true }))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("g", { metaKey: true }))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("g", { altKey: true }))).toEqual({ type: "none" });
  });

  it("ignores keystrokes during IME composition", () => {
    expect(resolveShortcut(ev("g", { isComposing: true }))).toEqual({ type: "none" });
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

  it("focuses the filter on '/'", () => {
    expect(resolveShortcut(ev("/"))).toEqual({ type: "focus-filter" });
  });

  it("closes on Escape", () => {
    expect(resolveShortcut(ev("Escape"))).toEqual({ type: "close" });
  });

  it("returns none for unmapped keys, including admin-only shortcuts", () => {
    // No new-scan or row navigation in the read-only analysis UI.
    expect(resolveShortcut(ev("n"))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("j"))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("k"))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("ArrowDown"))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("x"))).toEqual({ type: "none" });
    expect(resolveShortcut(ev("1"))).toEqual({ type: "none" });
  });
});

describe("resolveShortcut g-sequence", () => {
  it("resolves a mapped second key to a navigate action carrying the href", () => {
    expect(resolveShortcut(ev("d"), { pendingG: true })).toEqual({ type: "navigate", href: "/domains" });
    expect(resolveShortcut(ev("o"), { pendingG: true })).toEqual({ type: "navigate", href: "/" });
    expect(resolveShortcut(ev("f"), { pendingG: true })).toEqual({ type: "navigate", href: "/diff" });
  });

  it("disarms on an unmapped second key", () => {
    expect(resolveShortcut(ev("x"), { pendingG: true })).toEqual({ type: "clear-g" });
    expect(resolveShortcut(ev("g"), { pendingG: true })).toEqual({ type: "clear-g" });
  });

  it("still honours the editable guard while armed", () => {
    expect(resolveShortcut(ev("d", { target: el("INPUT") }), { pendingG: true })).toEqual({
      type: "none"
    });
  });
});

describe("resolveShortcut with the help overlay open", () => {
  it("suppresses every action except Escape", () => {
    expect(resolveShortcut(ev("g"), { overlayOpen: true })).toEqual({ type: "none" });
    expect(resolveShortcut(ev("/"), { overlayOpen: true })).toEqual({ type: "none" });
    expect(resolveShortcut(ev("?", { shiftKey: true }), { overlayOpen: true })).toEqual({ type: "none" });
    expect(resolveShortcut(ev("d"), { overlayOpen: true, pendingG: true })).toEqual({ type: "none" });
  });

  it("closes on Escape", () => {
    expect(resolveShortcut(ev("Escape"), { overlayOpen: true })).toEqual({ type: "close" });
  });
});

describe("shortcutNavRows", () => {
  it("pairs each g-key with its nav label in KEY_MAP order", () => {
    const rows = shortcutNavRows(navItems);
    expect(rows).toEqual([
      { key: "o", href: "/", label: "Overview" },
      { key: "c", href: "/cohorts", label: "Cohorts" },
      { key: "d", href: "/domains", label: "Domains" },
      { key: "n", href: "/nameservers", label: "Nameservers" },
      { key: "a", href: "/endpoints", label: "Addresses" },
      { key: "s", href: "/asns", label: "ASNs" },
      { key: "t", href: "/tags", label: "Tags" },
      { key: "r", href: "/trends", label: "Trends" },
      { key: "f", href: "/diff", label: "Diff" }
    ]);
  });

  it("never falls back to a raw href, proving every key resolves to a nav item", () => {
    for (const row of shortcutNavRows(navItems)) {
      expect(row.label).not.toBe(row.href);
    }
  });
});
