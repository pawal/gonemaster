// Helpers for exporting list-page rows as CSV or JSON files from the browser.

export type ExportColumn<T> = {
  key: string;
  label: string;
  value: (row: T) => string | number | null | undefined;
};

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
