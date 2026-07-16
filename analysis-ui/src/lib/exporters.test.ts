import { describe, it, expect, vi } from "vitest";
import {
  EXPORT_ROW_CAP,
  collectExportRows,
  exportCaption,
  exportScope,
  exportScopeSuffix,
  rowsToCSV,
  rowsToJSON,
  type ExportColumn
} from "./exporters";

type Row = { name: string; count: number };
const columns: ExportColumn<Row>[] = [
  { key: "name", label: "Name", value: (r) => r.name },
  { key: "count", label: "Count", value: (r) => r.count }
];

describe("rowsToCSV", () => {
  it("quotes cells containing commas, quotes and newlines", () => {
    const csv = rowsToCSV([{ name: 'a,"b"\nc', count: 1 }], columns);
    expect(csv).toBe('Name,Count\n"a,""b""\nc",1\n');
  });
});

describe("rowsToJSON", () => {
  it("projects rows through the column keys", () => {
    const json = rowsToJSON([{ name: "x", count: 2 }], columns);
    expect(JSON.parse(json)).toEqual([{ name: "x", count: 2 }]);
  });
});

describe("exportScope", () => {
  it("reports the full set when total is within the cap", () => {
    expect(exportScope(120)).toEqual({ count: 120, total: 120, capped: false });
  });

  it("caps count at the row cap when total exceeds it", () => {
    expect(exportScope(1234)).toEqual({ count: EXPORT_ROW_CAP, total: 1234, capped: true });
  });

  it("treats the cap boundary as not capped", () => {
    expect(exportScope(EXPORT_ROW_CAP)).toEqual({ count: 500, total: 500, capped: false });
  });

  it("normalizes zero, negative and non-finite totals to empty", () => {
    expect(exportScope(0)).toEqual({ count: 0, total: 0, capped: false });
    expect(exportScope(-5)).toEqual({ count: 0, total: 0, capped: false });
    expect(exportScope(Number.NaN)).toEqual({ count: 0, total: 0, capped: false });
  });
});

describe("exportCaption", () => {
  it("names the full set when nothing is truncated", () => {
    expect(exportCaption(exportScope(42))).toBe("Exports all 42 matching rows");
  });

  it("uses the singular noun for a single row", () => {
    expect(exportCaption(exportScope(1))).toBe("Exports all 1 matching row");
  });

  it("spells out the truncation when capped", () => {
    expect(exportCaption(exportScope(1234))).toBe(
      "Exports the first 500 of 1234 matching rows (server limit)"
    );
  });

  it("reports nothing to export when empty", () => {
    expect(exportCaption(exportScope(0))).toBe("Nothing to export");
  });
});

describe("exportScopeSuffix", () => {
  it("is empty for a full export and marks the count when capped", () => {
    expect(exportScopeSuffix(exportScope(120))).toBe("");
    expect(exportScopeSuffix(exportScope(1234))).toBe("-first-500");
  });
});

describe("collectExportRows", () => {
  const loaded: Row[] = [
    { name: "a", count: 1 },
    { name: "b", count: 2 }
  ];

  it("reuses the loaded page when it starts at offset 0 and covers the total", async () => {
    const fetchUpTo = vi.fn();
    const rows = await collectExportRows(loaded, 0, 2, fetchUpTo);
    expect(rows).toEqual(loaded);
    expect(fetchUpTo).not.toHaveBeenCalled();
  });

  it("fetches the whole set when the loaded page is only a slice of it", async () => {
    const full = Array.from({ length: 200 }, (_, i) => ({ name: `n${i}`, count: i }));
    const fetchUpTo = vi.fn().mockResolvedValue(full);
    const rows = await collectExportRows(loaded, 0, 200, fetchUpTo);
    expect(fetchUpTo).toHaveBeenCalledWith(200);
    expect(rows).toHaveLength(200);
  });

  it("refetches from the top when the loaded page is offset into the set", async () => {
    // Page 2 (offset 50) holds rows 50..99; an export must start at row 0.
    const full = Array.from({ length: 80 }, (_, i) => ({ name: `n${i}`, count: i }));
    const fetchUpTo = vi.fn().mockResolvedValue(full);
    const rows = await collectExportRows(loaded, 50, 80, fetchUpTo);
    expect(fetchUpTo).toHaveBeenCalledWith(80);
    expect(rows).toHaveLength(80);
  });

  it("limits the fetch to the cap and trims to it", async () => {
    const full = Array.from({ length: 600 }, (_, i) => ({ name: `n${i}`, count: i }));
    const fetchUpTo = vi.fn().mockResolvedValue(full);
    const rows = await collectExportRows(loaded, 0, 1234, fetchUpTo);
    expect(fetchUpTo).toHaveBeenCalledWith(EXPORT_ROW_CAP);
    expect(rows).toHaveLength(EXPORT_ROW_CAP);
  });

  it("returns nothing and never fetches when the total is zero", async () => {
    const fetchUpTo = vi.fn();
    const rows = await collectExportRows([], 0, 0, fetchUpTo);
    expect(rows).toEqual([]);
    expect(fetchUpTo).not.toHaveBeenCalled();
  });
});
