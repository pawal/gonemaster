// Small presentation helpers shared across page components. Keeping them as
// pure functions lets us unit test them without touching the SvelteKit
// runtime.

// ISO 8601 (local time), e.g. "2026-04-19 11:53:26". The "sv-SE" locale
// happens to produce this canonical shape, which avoids the American
// m/d/yyyy default of toLocaleString() on most systems.
const ISO_LOCAL = new Intl.DateTimeFormat("sv-SE", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit"
});

const ISO_DATE = new Intl.DateTimeFormat("sv-SE", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit"
});

export type SnapshotDisplayFields = {
  slug?: string;
  label?: string;
  first_run_at?: string;
  last_run_at?: string;
  captured_at?: string;
};

export function formatTimestamp(value: string | null | undefined): string {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "";
  if (parsed.getUTCFullYear() < 1970) return ""; // Go zero-time guard.
  return ISO_LOCAL.format(parsed);
}

export function formatDate(value: string | null | undefined): string {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "";
  if (parsed.getUTCFullYear() < 1970) return "";
  return ISO_DATE.format(parsed);
}

export function snapshotSlugDate(slug: string | null | undefined): string {
  const match = String(slug ?? "").match(/^(\d{4}-\d{2}-\d{2})(?:-|$)/);
  return match?.[1] ?? "";
}

export function snapshotSourceDate(snapshot: SnapshotDisplayFields | null | undefined): string {
  if (!snapshot) return "";
  return (
    formatDate(snapshot.last_run_at) ||
    formatDate(snapshot.first_run_at) ||
    snapshotSlugDate(snapshot.slug)
  );
}

export function snapshotDisplayLabel(snapshot: SnapshotDisplayFields | null | undefined): string {
  if (!snapshot) return "Snapshot";
  const explicit = String(snapshot.label ?? "").trim();
  if (explicit) return explicit;
  return snapshotSourceDate(snapshot) || snapshot.slug || "Snapshot";
}

export function snapshotOptionLabel(snapshot: SnapshotDisplayFields | null | undefined): string {
  const primary = snapshotDisplayLabel(snapshot);
  const sourceDate = snapshotSourceDate(snapshot);
  if (snapshot?.label?.trim() && sourceDate) return `${primary} (${sourceDate})`;
  return primary;
}

export function formatCount(n: number | null | undefined): string {
  if (n === null || n === undefined || !Number.isFinite(n)) return "—";
  return new Intl.NumberFormat().format(n);
}

export function levelTone(level: string | null | undefined): string {
  switch (String(level ?? "").toUpperCase()) {
    case "CRITICAL":
      return "critical";
    case "ERROR":
      return "error";
    case "WARNING":
      return "warning";
    case "NOTICE":
      return "notice";
    default:
      return "neutral";
  }
}

export function gradeTone(grade: string | null | undefined): string {
  switch (String(grade ?? "").toUpperCase()) {
    case "A+":
      return "aplus";
    case "A":
      return "a";
    case "B":
      return "b";
    case "C":
      return "c";
    case "D":
      return "d";
    case "F":
      return "f";
    default:
      return "neutral";
  }
}
