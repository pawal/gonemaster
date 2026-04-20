import { describe, expect, it } from "vitest";
import { applyFilterToParams, filterFromURL, searchToString } from "./filters";

describe("filters", () => {
  it("filterFromURL picks up dataset_tag and search, ignores other keys", () => {
    const url = new URL("http://x/?dataset_tag=tld&search=nic&unknown=1");
    expect(filterFromURL(url)).toEqual({
      dataset_tag: "tld",
      search: "nic"
    });
  });

  it("filterFromURL drops empty values", () => {
    const url = new URL("http://x/?dataset_tag=tld&search=");
    expect(filterFromURL(url)).toEqual({ dataset_tag: "tld" });
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
