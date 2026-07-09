import { LEVEL_ORDER, normalizeLevel } from "./jobUtils.js";

export const moduleLevels = ["NOTICE", "WARNING", "ERROR", "CRITICAL"];

export const CAT_ORDER = ["dnssec", "nameserver_health", "connectivity", "zone_consistency"];
export const CAT_LABELS = {
  dnssec: "DNSSEC",
  nameserver_health: "Nameserver",
  connectivity: "Connectivity",
  zone_consistency: "Zone",
};
export const BONUS_HIDDEN = new Set(["no_warnings_or_errors"]);

export const worstLevel = (entries) => {
  if (!entries?.length) return "INFO";
  let worst = 0;
  for (const e of entries) {
    const idx = LEVEL_ORDER.indexOf(normalizeLevel(e.level));
    if (idx > worst) worst = idx;
  }
  return LEVEL_ORDER[worst] ?? "INFO";
};

export const bannerClass = (level) => {
  const l = normalizeLevel(level);
  if (l === "CRITICAL") return "critical";
  if (l === "ERROR") return "error";
  if (l === "WARNING") return "warning";
  return "ok";
};

export const isNoticeOrAbove = (level) =>
  LEVEL_ORDER.indexOf(normalizeLevel(level)) >= LEVEL_ORDER.indexOf("NOTICE");

export const hasScore = (item) => item?.score != null && item?.grade != null;

export const chipGrade = (item) => item?.grade ?? null;
export const chipScore = (item) => item?.score ?? null;
export const resultScore = (result) => result?.score ?? null;

export const formatSeconds = (value) => {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return "0.00";
  return numeric.toFixed(2);
};

export const formatTimingMs = (value) => {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return "0";
  return Math.round(numeric).toString();
};

// nsRowStatus treats any status other than "unreachable"/"unresolved" as ok so
// old rows (status unset) keep rendering as before.
export const nsRowStatus = (item) => {
  if (item?.status === "unreachable" || item?.status === "unresolved") {
    return item.status;
  }
  return "ok";
};

// nsTimingCell picks the cell content by row status: "∞" for a reachable
// address that never answered, "-" for a name that never resolved, otherwise
// the formatted milliseconds.
export const nsTimingCell = (item, value) => {
  const s = nsRowStatus(item);
  if (s === "unreachable") return "∞";
  if (s === "unresolved") return "-";
  return formatTimingMs(value);
};

export const nsSamplesCell = (item) => {
  const s = nsRowStatus(item);
  if (s === "unreachable") return "0";
  if (s === "unresolved") return "-";
  return `${item?.count ?? 0}`;
};

// nsStatusLabelKey maps a row status to its i18n badge key, or "" when the row
// is ok and needs no badge.
export const nsStatusLabelKey = (item) => {
  switch (nsRowStatus(item)) {
    case "unreachable":
      return "ns_timing_status_unreachable";
    case "unresolved":
      return "ns_timing_status_unresolved";
    default:
      return "";
  }
};

export const entryMessage = (entry) => {
  if (!entry) return "";
  if (entry.message) return entry.message;
  if (entry.raw) return entry.raw;
  return [entry.module, entry.testcase, entry.tag].filter(Boolean).join(":");
};

export const moduleId = (key) =>
  `module-${String(key).toLowerCase().replace(/[^a-z0-9]+/g, "-")}`;

export const summaryRows = (summary) => {
  const levels = summary?.levels || {};
  return moduleLevels
    .map((level) => ({ level, count: Number(levels[level] || 0) }))
    .filter((entry) => entry.count > 0);
};

export const okTestcaseCount = (group) =>
  group.testcasesArr.filter(
    (tcg) => !moduleLevels.includes(normalizeLevel(tcg.level)),
  ).length;

export const groupRawEntries = (raw) => {
  const entries = raw?.entries || [];
  const modules = new Map();
  entries.forEach((entry) => {
    const name = entry.module || "Unspecified";
    const key = name.toUpperCase();
    if (!modules.has(key)) {
      modules.set(key, {
        key,
        name,
        entries: [],
        testcases: new Map(),
        ungrouped: [],
        counts: {},
      });
    }
    const mod = modules.get(key);
    mod.entries.push(entry);
    const level = normalizeLevel(entry.level);
    mod.counts[level] = (mod.counts[level] || 0) + 1;
    const tc = entry.testcase && entry.testcase !== "Unspecified" ? entry.testcase : "";
    if (tc) {
      if (!mod.testcases.has(tc)) mod.testcases.set(tc, { tc, entries: [] });
      mod.testcases.get(tc).entries.push(entry);
    } else {
      mod.ungrouped.push(entry);
    }
  });
  for (const mod of modules.values()) {
    for (const tcg of mod.testcases.values()) tcg.level = worstLevel(tcg.entries);
    mod.testcasesArr = Array.from(mod.testcases.values());
  }
  const arr = Array.from(modules.values());
  arr.sort((a, b) => {
    if (a.key === "SYSTEM") return -1;
    if (b.key === "SYSTEM") return 1;
    return 0;
  });
  return arr;
};
