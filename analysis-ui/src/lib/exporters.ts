// Helpers for exporting list-page rows as CSV or JSON files from the browser.

// The list endpoints cap a single response at this many rows, so an export
// can never cover more than this regardless of how many rows match.
export const EXPORT_ROW_CAP = 500;

export type ExportColumn<T> = {
  key: string;
  label: string;
  value: (row: T) => string | number | null | undefined;
};

export type ExportScope = {
  count: number; // rows the export will actually contain
  total: number; // rows matching the current filter
  capped: boolean; // the cap truncates the set (count < total)
};

// Describe what an export of `total` matching rows will contain, so the UI can
// say so honestly before the user clicks.
export function exportScope(total: number, cap = EXPORT_ROW_CAP): ExportScope {
  const safe = Number.isFinite(total) && total > 0 ? Math.floor(total) : 0;
  return { count: Math.min(safe, cap), total: safe, capped: safe > cap };
}

// One-line caption for the export controls.
export function exportCaption(scope: ExportScope): string {
  if (scope.total === 0) return "Nothing to export";
  if (scope.capped) {
    return `Exports the first ${scope.count} of ${scope.total} matching rows (server limit)`;
  }
  const noun = scope.count === 1 ? "row" : "rows";
  return `Exports all ${scope.count} matching ${noun}`;
}

// Filename marker so a truncated export is obvious on disk, not just on screen.
export function exportScopeSuffix(scope: ExportScope): string {
  return scope.capped ? `-first-${scope.count}` : "";
}

// Gather the rows to export, from the top of the filtered set up to the cap.
// Reuses the loaded page only when it already starts at offset 0 and spans the
// whole target; otherwise fetches the set fresh via `fetchUpTo`.
export async function collectExportRows<T>(
  loaded: T[],
  offset: number,
  total: number,
  fetchUpTo: (limit: number) => Promise<T[]>,
  cap = EXPORT_ROW_CAP
): Promise<T[]> {
  const target = Math.min(Number.isFinite(total) && total > 0 ? total : 0, cap);
  if (target <= 0) return [];
  if (offset === 0 && loaded.length >= target) return loaded.slice(0, target);
  const fetched = await fetchUpTo(target);
  return fetched.slice(0, target);
}

function csvCell(value: unknown): string {
  if (value === null || value === undefined) return "";
  const str = String(value);
  if (/[",\n\r]/.test(str)) {
    return `"${str.replace(/"/g, '""')}"`;
  }
  return str;
}

export function rowsToCSV<T>(rows: T[], columns: ExportColumn<T>[]): string {
  const header = columns.map((c) => csvCell(c.label)).join(",");
  const body = rows.map((row) =>
    columns.map((c) => csvCell(c.value(row))).join(",")
  );
  return [header, ...body].join("\n") + "\n";
}

export function rowsToJSON<T>(rows: T[], columns: ExportColumn<T>[]): string {
  const projected = rows.map((row) => {
    const out: Record<string, string | number | null | undefined> = {};
    for (const c of columns) out[c.key] = c.value(row);
    return out;
  });
  return JSON.stringify(projected, null, 2) + "\n";
}

export function downloadBlob(filename: string, mime: string, contents: string): void {
  const blob = new Blob([contents], { type: `${mime};charset=utf-8` });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export function downloadCSV<T>(
  filename: string,
  rows: T[],
  columns: ExportColumn<T>[]
): void {
  downloadBlob(filename, "text/csv", rowsToCSV(rows, columns));
}

export function downloadJSON<T>(
  filename: string,
  rows: T[],
  columns: ExportColumn<T>[]
): void {
  downloadBlob(filename, "application/json", rowsToJSON(rows, columns));
}
