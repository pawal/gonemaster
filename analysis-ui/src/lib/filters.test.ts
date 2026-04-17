import { describe, expect, it } from "vitest";
import { applyFilterToParams, filterFromURL, searchToString } from "./filters";

describe("filters", () => {
  it("filterFromURL ignores missing and empty keys", () => {
    const url = new URL("http://x/?dataset_tag=tld&search=&scope_mode=batch&batch_id=b1");
    expect(filterFromURL(url)).toEqual({
      dataset_tag: "tld",
      scope_mode: "batch",
      batch_id: "b1"
    });
  });

  it("applyFilterToParams removes batch fields when switching to latest_global", () => {
    const initial = new URLSearchParams("scope_mode=batch&batch_id=b1&from=2026-01-01");
    const next = applyFilterToParams(initial, { scope_mode: "latest_global" });
    expect(next.get("scope_mode")).toBe("latest_global");
    expect(next.get("batch_id")).toBeNull();
    expect(next.get("from")).toBeNull();
  });

  it("applyFilterToParams keeps batch_id when scope is batch or latest_in_batch", () => {
    const initial = new URLSearchParams("scope_mode=batch&batch_id=b1&from=2026-01-01");
    const next = applyFilterToParams(initial, { scope_mode: "latest_in_batch" });
    expect(next.get("batch_id")).toBe("b1");
    // from/to should be cleared because they don't apply to latest_in_batch.
    expect(next.get("from")).toBeNull();
  });

  it("applyFilterToParams clears batch_id when switching to time_window", () => {
    const initial = new URLSearchParams("scope_mode=batch&batch_id=b1");
    const next = applyFilterToParams(initial, { scope_mode: "time_window", from: "2026-01-01" });
    expect(next.get("scope_mode")).toBe("time_window");
    expect(next.get("batch_id")).toBeNull();
    expect(next.get("from")).toBe("2026-01-01");
  });

  it("deleting a field via empty string removes it from the URL", () => {
    const initial = new URLSearchParams("family=ipv4&level=ERROR");
    const next = applyFilterToParams(initial, { family: "" });
    expect(next.get("family")).toBeNull();
    expect(next.get("level")).toBe("ERROR");
  });

  it("searchToString returns '' for empty params and a leading ? otherwise", () => {
    expect(searchToString(new URLSearchParams())).toBe("");
    expect(searchToString(new URLSearchParams("a=1"))).toBe("?a=1");
  });
});
