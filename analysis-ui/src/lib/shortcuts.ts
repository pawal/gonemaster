// Keyboard-shortcut resolution for the analysis UI. Pure and DOM-free so the
// decision logic is unit-testable; the layout performs the effects.

import type { NavItem } from "$lib/nav";

// Second key of a g-sequence -> nav section href (relative to the SvelteKit
// base). Letters mirror the admin UI where they overlap (d/t/c) so muscle
// memory carries across dashboards; the rest are mnemonic.
export const KEY_MAP: Record<string, string> = {
  o: "/",
  c: "/cohorts",
  d: "/domains",
  n: "/nameservers",
  a: "/endpoints",
  s: "/asns",
  t: "/tags",
  r: "/trends",
  f: "/diff"
};

export type ShortcutAction =
  | { type: "none" }
  | { type: "help-toggle" }
  | { type: "close" }
  | { type: "set-g" }
  | { type: "clear-g" }
  | { type: "focus-filter" }
  | { type: "navigate"; href: string };

// Minimal shape read off a keydown; a real KeyboardEvent satisfies it, and
// tests can pass a plain stub.
export type ShortcutEvent = {
  key: string;
  target?: unknown;
  isComposing?: boolean;
  ctrlKey?: boolean;
  metaKey?: boolean;
  altKey?: boolean;
  shiftKey?: boolean;
};

// True when a keystroke targets an editable element and should be left alone.
export function isEditableTarget(el: unknown): boolean {
  if (!el || typeof el !== "object") return false;
  const node = el as { tagName?: unknown; isContentEditable?: unknown };
  const tag = String(node.tagName ?? "").toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return true;
  return node.isContentEditable === true;
}

// Resolve a keydown into an action descriptor. `pendingG` is whether the g
// prefix is armed; `overlayOpen` is whether the help overlay is open.
export function resolveShortcut(
  event: ShortcutEvent,
  { pendingG = false, overlayOpen = false }: { pendingG?: boolean; overlayOpen?: boolean } = {}
): ShortcutAction {
  const none: ShortcutAction = { type: "none" };

  // Never interrupt IME composition (e.g. Japanese input).
  if (event.isComposing) return none;

  // Browser/OS chords are off-limits; Shift is allowed (needed for "?").
  if (event.ctrlKey || event.metaKey || event.altKey) return none;

  const key = event.key;

  // When the help overlay owns the screen, only Esc is handled.
  if (overlayOpen) {
    return key === "Escape" ? { type: "close" } : none;
  }

  // Typing in a field must never trigger a shortcut.
  if (isEditableTarget(event.target)) return none;

  // Second key of a g-sequence: navigate on a mapped key, otherwise disarm.
  if (pendingG) {
    const href = KEY_MAP[key];
    return href ? { type: "navigate", href } : { type: "clear-g" };
  }

  switch (key) {
    case "?":
      return { type: "help-toggle" };
    case "g":
      return { type: "set-g" };
    case "/":
      return { type: "focus-filter" };
    case "Escape":
      return { type: "close" };
    default:
      return none;
  }
}

// Pair each g-sequence key with its nav section, for the help overlay. Labels
// come from the nav list so the two never drift; a key whose href has no nav
// item falls back to the href, which the coherency test forbids.
export function shortcutNavRows(items: NavItem[]): { key: string; href: string; label: string }[] {
  return Object.entries(KEY_MAP).map(([key, href]) => {
    const item = items.find((i) => i.href === href);
    return { key, href, label: item ? item.label : href };
  });
}
