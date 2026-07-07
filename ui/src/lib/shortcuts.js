// Keyboard-shortcut resolution for the admin UI. Pure and DOM-free so the
// decision logic is unit-testable; the App component performs the effects.

// Second key of a g-sequence -> tab id.
export const KEY_MAP = {
  s: "single",
  r: "recent",
  d: "domains",
  t: "tags",
  c: "cohorts",
  b: "batches",
  m: "metrics",
  ",": "settings",
};

// True when a keystroke targets an editable element and should be left alone.
export const isEditableTarget = (el) => {
  if (!el) return false;
  const tag = String(el.tagName || "").toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return true;
  return el.isContentEditable === true;
};

// Resolve a keydown into an action descriptor. `pendingG` is whether the g
// prefix is armed; `overlayOpen` is whether the help overlay is open.
export const resolveShortcut = (event, { pendingG = false, overlayOpen = false } = {}) => {
  const none = { type: "none" };

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
    const tab = KEY_MAP[key];
    return tab ? { type: "navigate", tab } : { type: "clear-g" };
  }

  switch (key) {
    case "?":
      return { type: "help-toggle" };
    case "g":
      return { type: "set-g" };
    case "n":
      return { type: "new-scan" };
    case "/":
      return { type: "focus-filter" };
    case "j":
    case "ArrowDown":
      return { type: "row-next" };
    case "k":
    case "ArrowUp":
      return { type: "row-prev" };
    case "Escape":
      return { type: "close" };
    default:
      return none;
  }
};
