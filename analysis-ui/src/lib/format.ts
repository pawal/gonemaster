// Small presentation helpers shared across page components. Keeping them as
// pure functions lets us unit test them without touching the SvelteKit
// runtime.

export function formatTimestamp(value: string | null | undefined): string {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "";
  if (parsed.getUTCFullYear() < 1970) return ""; // Go zero-time guard.
  return parsed.toLocaleString();
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
