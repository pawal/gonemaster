import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  loadEntries,
  addEntry,
  setGrade,
  removeEntry,
  clearEntries,
  MAX_ENTRIES,
  STORAGE_KEY,
} from "./history.js";

// Convenience factory for a valid entry with a distinguishing suffix.
function entry(n, extra = {}) {
  return {
    id: `id${n}`,
    domain: `example${n}.com`,
    finishedAt: `2026-08-07T09:0${n % 10}:00Z`,
    ...extra,
  };
}

beforeEach(() => {
  window.localStorage.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("loadEntries", () => {
  it("returns an empty array when nothing is stored", () => {
    const got = loadEntries();
    expect(Array.isArray(got)).toBe(true);
    expect(got).toEqual([]);
  });

  it("treats malformed stored JSON as empty", () => {
    window.localStorage.setItem(STORAGE_KEY, "{not json");
    expect(loadEntries()).toEqual([]);
  });

  it("treats a stored non-array JSON value as empty", () => {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ id: "x" }));
    expect(loadEntries()).toEqual([]);
  });

  it("filters out members that do not have the entry shape", () => {
    // A hand-edited or corrupted list may hold junk alongside valid entries.
    // Only the valid ones should survive the read.
    const valid = entry(1);
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify([
        valid,
        null,
        "string",
        42,
        { id: "", domain: "example.com", finishedAt: null },
        { id: "ok", domain: "", finishedAt: null },
        { id: "ok", domain: "example.com", finishedAt: 12345 },
        { id: "ok", domain: "example.com", finishedAt: null, grade: 7 },
      ])
    );
    expect(loadEntries()).toEqual([valid]);
  });

  it("accepts a null finishedAt and an absent grade", () => {
    const e = { id: "a", domain: "example.com", finishedAt: null };
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify([e]));
    expect(loadEntries()).toEqual([e]);
  });

  it("caps an oversized stored list at MAX_ENTRIES", () => {
    // The module never writes more than MAX_ENTRIES, but the stored value
    // is not under our control; reads must apply the cap too.
    const many = Array.from({ length: MAX_ENTRIES + 5 }, (_, i) => entry(i));
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(many));
    expect(loadEntries()).toHaveLength(MAX_ENTRIES);
  });

  it("returns an empty array when localStorage.getItem throws", () => {
    vi.spyOn(window.localStorage, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(loadEntries()).toEqual([]);
  });
});

describe("addEntry", () => {
  it("stores newest first and returns the list", () => {
    addEntry(entry(1));
    const got = addEntry(entry(2));
    expect(got.map((e) => e.id)).toEqual(["id2", "id1"]);
    expect(loadEntries().map((e) => e.id)).toEqual(["id2", "id1"]);
  });

  it("persists only the known fields", () => {
    const got = addEntry({ ...entry(1), status: "succeeded", extra: true });
    expect(got).toEqual([entry(1)]);
    expect(loadEntries()).toEqual([entry(1)]);
  });

  it("keeps the grade when the input carries one", () => {
    addEntry(entry(1, { grade: "A+" }));
    expect(loadEntries()[0].grade).toBe("A+");
  });

  it("normalizes a missing finishedAt to null", () => {
    addEntry({ id: "a", domain: "example.com" });
    expect(loadEntries()).toEqual([{ id: "a", domain: "example.com", finishedAt: null }]);
  });

  it("dedupes by id: re-adding moves the entry to the front, replaced", () => {
    addEntry(entry(1));
    addEntry(entry(2));
    // Same id as the first add, but a newer finish time. The old copy must
    // be dropped rather than duplicated, and the fresh data wins.
    const rerun = { id: "id1", domain: "example1.com", finishedAt: "2026-08-07T12:00:00Z" };
    const got = addEntry(rerun);
    expect(got.map((e) => e.id)).toEqual(["id1", "id2"]);
    expect(got[0]).toEqual(rerun);
    expect(got).toHaveLength(2);
  });

  it("enforces the cap by dropping the oldest entry", () => {
    for (let i = 0; i < MAX_ENTRIES; i++) addEntry(entry(i));
    const got = addEntry(entry(99));
    expect(got).toHaveLength(MAX_ENTRIES);
    expect(got[0].id).toBe("id99");
    // entry(0) was the oldest and must be gone.
    const hasOldest = got.some((e) => e.id === "id0");
    expect(hasOldest).toBe(false);
  });

  it("ignores input without a usable id or domain", () => {
    addEntry(entry(1));
    expect(addEntry(null)).toEqual([entry(1)]);
    expect(addEntry({ domain: "example.com" })).toEqual([entry(1)]);
    expect(addEntry({ id: "x", domain: "" })).toEqual([entry(1)]);
    expect(loadEntries()).toHaveLength(1);
  });

  it("returns the new list even when localStorage.setItem throws", () => {
    // Quota errors or private-browsing restrictions must not surface to the
    // caller; the in-memory result is still useful for rendering.
    vi.spyOn(window.localStorage, "setItem").mockImplementation(() => {
      throw new Error("quota");
    });
    const got = addEntry(entry(1));
    expect(got).toEqual([entry(1)]);
  });
});

describe("setGrade", () => {
  it("updates the grade of an existing entry and persists it", () => {
    addEntry(entry(1));
    addEntry(entry(2));
    const got = setGrade("id1", "B");
    expect(got.find((e) => e.id === "id1").grade).toBe("B");
    expect(loadEntries().find((e) => e.id === "id1").grade).toBe("B");
    // The other entry is untouched and order is preserved.
    const hasGrade = "grade" in loadEntries().find((e) => e.id === "id2");
    expect(hasGrade).toBe(false);
    expect(loadEntries().map((e) => e.id)).toEqual(["id2", "id1"]);
  });

  it("does not create an entry for an unknown id", () => {
    addEntry(entry(1));
    const got = setGrade("missing", "A");
    expect(got).toHaveLength(1);
    expect(loadEntries()).toHaveLength(1);
  });

  it("ignores an empty or non-string grade", () => {
    addEntry(entry(1));
    setGrade("id1", "");
    setGrade("id1", 5);
    const hasGrade = "grade" in loadEntries()[0];
    expect(hasGrade).toBe(false);
  });
});

describe("removeEntry", () => {
  it("removes the entry with the given id and persists", () => {
    addEntry(entry(1));
    addEntry(entry(2));
    const got = removeEntry("id1");
    expect(got.map((e) => e.id)).toEqual(["id2"]);
    expect(loadEntries().map((e) => e.id)).toEqual(["id2"]);
  });

  it("is a no-op for an unknown id", () => {
    addEntry(entry(1));
    expect(removeEntry("missing")).toEqual([entry(1)]);
    expect(loadEntries()).toEqual([entry(1)]);
  });
});

describe("clearEntries", () => {
  it("empties the list and persists the removal", () => {
    addEntry(entry(1));
    addEntry(entry(2));
    expect(clearEntries()).toEqual([]);
    expect(loadEntries()).toEqual([]);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
  });

  it("returns an empty array even when localStorage.removeItem throws", () => {
    vi.spyOn(window.localStorage, "removeItem").mockImplementation(() => {
      throw new Error("denied");
    });
    expect(clearEntries()).toEqual([]);
  });
});
