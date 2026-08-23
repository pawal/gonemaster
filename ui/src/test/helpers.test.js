import { beforeEach, describe, expect, it, vi } from "vitest";
import { installFetchRoutes, jsonResponse, requestUrl } from "./helpers.js";

describe("requestUrl", () => {
  it.each([
    ["a string", "/api/v1/jobs?limit=5", "/api/v1/jobs?limit=5"],
    ["a Request", { url: "/api/v1/jobs" }, "/api/v1/jobs"],
    ["a URL", new URL("http://localhost/api/v1/tags"), "http://localhost/api/v1/tags"],
    ["nothing", undefined, ""]
  ])("normalizes %s", (_name, input, want) => {
    expect(requestUrl(input)).toBe(want);
  });
});

describe("installFetchRoutes", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  it("answers in declaration order and records every URL", async () => {
    const calls = installFetchRoutes({
      "/api/v1/batches?": { items: [] },
      "/api/v1/batches/": { batch_id: "b1" }
    });

    expect(await (await global.fetch("/api/v1/batches?limit=5")).json()).toEqual({ items: [] });
    expect(await (await global.fetch("/api/v1/batches/b1")).json()).toEqual({ batch_id: "b1" });
    expect(calls).toEqual(["/api/v1/batches?limit=5", "/api/v1/batches/b1"]);
  });

  it("falls through when a route function returns undefined", async () => {
    installFetchRoutes({
      "/api/v1/jobs": (_url, options) => (options.method === "POST" ? { id: "j1" } : undefined),
      "/api/v1/jobs?": { items: [] }
    });

    expect(await (await global.fetch("/api/v1/jobs", { method: "POST" })).json()).toEqual({ id: "j1" });
    expect(await (await global.fetch("/api/v1/jobs?limit=5")).json()).toEqual({ items: [] });
  });

  it("passes a Response value through and uses the fallback for unrouted URLs", async () => {
    installFetchRoutes({ "/api/v1/tags": jsonResponse({ error: "no" }, false) }, { items: [], total: 0 });

    const failed = await global.fetch("/api/v1/tags");
    expect(failed.ok).toBe(false);
    expect(await (await global.fetch("/api/v1/whatever")).json()).toEqual({ items: [], total: 0 });
  });
});
