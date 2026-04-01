import { describe, it, expect } from "vitest";
import { parseHash, hashFor } from "./router.js";

describe("parseHash", () => {
  it.each([
    ["", { view: "home", publicID: null }],
    ["#", { view: "home", publicID: null }],
    ["#/", { view: "home", publicID: null }],
    ["#/anything-else", { view: "home", publicID: null }],
  ])("routes %j to home", (hash, expected) => {
    expect(parseHash(hash)).toEqual(expected);
  });

  it("routes #/result/:id to result view", () => {
    expect(parseHash("#/result/abc12345")).toEqual({ view: "result", publicID: "abc12345" });
  });

  it("accepts alphanumeric public IDs of various lengths", () => {
    expect(parseHash("#/result/Ab3Cd8Ef")).toEqual({ view: "result", publicID: "Ab3Cd8Ef" });
    expect(parseHash("#/result/x")).toEqual({ view: "result", publicID: "x" });
  });

  it("rejects #/result/ with no ID (routes to home)", () => {
    expect(parseHash("#/result/")).toEqual({ view: "home", publicID: null });
  });

  it("rejects IDs containing non-alphanumeric characters (routes to home)", () => {
    expect(parseHash("#/result/abc-123")).toEqual({ view: "home", publicID: null });
    expect(parseHash("#/result/abc/extra")).toEqual({ view: "home", publicID: null });
  });

  it("works without leading #", () => {
    expect(parseHash("/result/abc12345")).toEqual({ view: "result", publicID: "abc12345" });
  });
});

describe("hashFor", () => {
  it("returns #/ for home view", () => {
    expect(hashFor("home")).toBe("#/");
    expect(hashFor("home", null)).toBe("#/");
  });

  it("returns #/result/:id for result view with a publicID", () => {
    expect(hashFor("result", "abc12345")).toBe("#/result/abc12345");
  });

  it("falls back to #/ for result view without a publicID", () => {
    expect(hashFor("result", null)).toBe("#/");
    expect(hashFor("result")).toBe("#/");
  });
});
