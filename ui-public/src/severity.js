// Severity levels in ascending order.
export const LEVELS = ["DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL"];

// Maps an uppercase level name to its CSS modifier class for .level-pill.
const CLASS_MAP = {
  DEBUG:    "severity-debug",
  INFO:     "severity-info",
  NOTICE:   "severity-notice",
  WARNING:  "severity-warning",
  ERROR:    "severity-error",
  CRITICAL: "severity-critical",
};

// Maps an uppercase level name to the CSS modifier for a .status-banner.
const BANNER_MAP = {
  DEBUG:    "ok",
  INFO:     "ok",
  NOTICE:   "ok",
  WARNING:  "warning",
  ERROR:    "error",
  CRITICAL: "critical",
};

/** Returns the CSS modifier class for a level pill, e.g. "severity-warning". */
export function levelClass(level) {
  return CLASS_MAP[level?.toUpperCase()] ?? "";
}

/** Returns the CSS modifier class for a status banner based on worst level. */
export function bannerClass(level) {
  return BANNER_MAP[level?.toUpperCase()] ?? "ok";
}

/**
 * Returns the worst severity level name from an array of result entries.
 * Each entry is expected to have a `level` string property.
 * Returns "INFO" when entries is empty.
 */
export function worstLevel(entries) {
  if (!entries || entries.length === 0) return "INFO";
  let worst = -1;
  for (const e of entries) {
    const idx = LEVELS.indexOf(e.level?.toUpperCase());
    if (idx > worst) worst = idx;
  }
  return worst >= 0 ? LEVELS[worst] : "INFO";
}
