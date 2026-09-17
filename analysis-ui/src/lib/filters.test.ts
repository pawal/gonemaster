import { appNavigation } from "../test/helpers";
import { describe, expect, it, vi } from "vitest";
import { applyFilterToParams, filterFromURL, searchToString, updateURLParam } from "./filters";

vi.mock("$app/navigation", () => appNavigation());

describe("filters", () => {
  it("filterFromURL picks up dataset_tag and search, ignores other keys", () => {
    const url = new URL("http://x/?dataset_tag=tld&search=nic&unknown=1");
    expect(filterFromURL(url)).toEqual({
      dataset_tag: "tld",
      search: "nic"
    });
  });

  it("filterFromURL forwards worst_level and grade to the API call", () => {
    // Overview health-bar segments deep-link with worst_level; grade-bar
    // segments deep-link with grade. The loader must include both so the
    // filter actually reaches the server.
    const url = new URL("http://x/?dataset_tag=tld&worst_level=ERROR&grade=B");
    expect(filterFromURL(url)).toEqual({
      dataset_tag: "tld",
      worst_level: "ERROR",
      grade: "B"
    });
  });

  it("filterFromURL forwards dnssec_posture so the posture drill-down filters", () => {
    // Dropping the key here would land the visitor on the unfiltered list.
    const url = new URL("http://x/?dataset_tag=tld&dnssec_posture=nsec3");
    expect(filterFromURL(url)).toEqual({
      dataset_tag: "tld",
      dnssec_posture: "nsec3"
    });
  });

  it("filterFromURL drops empty values", () => {
    const url = new URL("http://x/?dataset_tag=tld&search=");
    expect(filterFromURL(url)).toEqual({ dataset_tag: "tld" });
  });

  it("filterFromURL picks up snapshot so ?snapshot= reaches the API", () => {
    // Every entity chip preserves the snapshot pin across navigation;
    // the filter loader is where that slug actually becomes a server
    // filter. Dropping it would silently flip the page back to
    // auto-latest and hide the bug until an operator noticed the
    // numbers drifting.
    const url = new URL("http://x/?dataset_tag=tld&snapshot=2026-04-20");
    expect(filterFromURL(url)).toEqual({
      dataset_tag: "tld",
      snapshot: "2026-04-20"
    });
  });

  it("applyFilterToParams round-trips snapshot alongside dataset_tag", () => {
    const initial = new URLSearchParams("dataset_tag=tld");
    const next = applyFilterToParams(initial, { snapshot: "2026-04-20" });
    expect(next.get("snapshot")).toBe("2026-04-20");
    const cleared = applyFilterToParams(next, { snapshot: "" });
    expect(cleared.get("snapshot")).toBeNull();
  });

  it("applyFilterToParams sets and deletes keys", () => {
    const initial = new URLSearchParams("dataset_tag=tld&search=nic");
    const next = applyFilterToParams(initial, { dataset_tag: "media", search: "" });
    expect(next.get("dataset_tag")).toBe("media");
    expect(next.get("search")).toBeNull();
  });

  it("searchToString returns '' for empty params and a leading ? otherwise", () => {
    expect(searchToString(new URLSearchParams())).toBe("");
    expect(searchToString(new URLSearchParams("a=1"))).toBe("?a=1");
  });
});

describe("updateURLParam", () => {
  it("sets a new param and calls goto with keepFocus options", async () => {
    const { goto } = await import("$app/navigation");
    updateURLParam(new URL("http://localhost/domains?dataset_tag=tld"), "sort", "name_asc");
    expect(goto).toHaveBeenCalledWith("/domains?dataset_tag=tld&sort=name_asc", {
      replaceState: false,
      noScroll: false,
      keepFocus: true
    });
  });

  it("removes the param when value is empty", async () => {
    const { goto } = await import("$app/navigation");
    updateURLParam(new URL("http://localhost/domains?dataset_tag=tld&sort=name_asc"), "sort", "");
    expect(goto).toHaveBeenCalledWith("/domains?dataset_tag=tld", {
      replaceState: false,
      noScroll: false,
      keepFocus: true
    });
  });

  it("preserves unrelated params when setting a new one", async () => {
    const { goto } = await import("$app/navigation");
    updateURLParam(
      new URL("http://localhost/domains?dataset_tag=tld&snapshot=2026-04-20"),
      "limit",
      "50"
    );
    expect(goto).toHaveBeenCalledWith(
      "/domains?dataset_tag=tld&snapshot=2026-04-20&limit=50",
      expect.any(Object)
    );
  });
});
