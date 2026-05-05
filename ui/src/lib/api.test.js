import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { apiCall } from "./api.js";

const createApiFetch = (apiBase) => (path, options) => apiCall(apiBase, path, options);

function jsonResponse(body, init = {}) {
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: { "content-type": "application/json", ...(init.headers || {}) },
  });
}

function textResponse(body, init = {}) {
  return new Response(body, {
    status: init.status ?? 200,
    headers: { "content-type": "text/plain", ...(init.headers || {}) },
  });
}

describe("createApiFetch", () => {
  let originalFetch;

  beforeEach(() => {
    originalFetch = globalThis.fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it("prefixes leading-slash paths with apiBase", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true }));
    globalThis.fetch = fetchMock;
    const apiFetch = createApiFetch("/api/v1");

    const result = await apiFetch("/jobs");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/jobs");
    expect(result).toEqual({ ok: true });
  });

  it("inserts a slash between apiBase and a bare path", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({}));
    globalThis.fetch = fetchMock;
    const apiFetch = createApiFetch("/api/v1");

    await apiFetch("jobs");

    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/jobs");
  });

  it("auto-sets Content-Type: application/json when a body is present", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({}));
    globalThis.fetch = fetchMock;
    const apiFetch = createApiFetch("/api/v1");

    await apiFetch("/jobs", { method: "POST", body: JSON.stringify({ a: 1 }) });

    const init = fetchMock.mock.calls[0][1];
    expect(init.method).toBe("POST");
    expect(init.headers["Content-Type"]).toBe("application/json");
  });

  it("does not overwrite an explicit Content-Type", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({}));
    globalThis.fetch = fetchMock;
    const apiFetch = createApiFetch("/api/v1");

    await apiFetch("/jobs", {
      method: "POST",
      body: "raw",
      headers: { "Content-Type": "text/plain" },
    });

    expect(fetchMock.mock.calls[0][1].headers["Content-Type"]).toBe("text/plain");
  });

  it("does not set Content-Type when there is no body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({}));
    globalThis.fetch = fetchMock;
    const apiFetch = createApiFetch("/api/v1");

    await apiFetch("/jobs");

    expect(fetchMock.mock.calls[0][1].headers["Content-Type"]).toBeUndefined();
  });

  it("returns a parsed JSON payload on 2xx", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(jsonResponse({ id: 7 }));
    const apiFetch = createApiFetch("/api/v1");

    expect(await apiFetch("/jobs/7")).toEqual({ id: 7 });
  });

  it("returns text on 2xx when content-type is not JSON", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(textResponse("hello"));
    const apiFetch = createApiFetch("/api/v1");

    expect(await apiFetch("/ping")).toBe("hello");
  });

  it("throws with payload.error.message on non-2xx JSON", async () => {
    globalThis.fetch = vi
      .fn()
      .mockResolvedValue(jsonResponse({ error: { message: "boom" } }, { status: 400 }));
    const apiFetch = createApiFetch("/api/v1");

    await expect(apiFetch("/jobs")).rejects.toThrow("boom");
  });

  it("throws with payload.message when error.message is absent", async () => {
    globalThis.fetch = vi
      .fn()
      .mockResolvedValue(jsonResponse({ message: "nope" }, { status: 500 }));
    const apiFetch = createApiFetch("/api/v1");

    await expect(apiFetch("/jobs")).rejects.toThrow("nope");
  });

  it("falls back to statusText on a non-JSON error", async () => {
    globalThis.fetch = vi
      .fn()
      .mockResolvedValue(textResponse("server fell over", { status: 502 }));
    const apiFetch = createApiFetch("/api/v1");

    await expect(apiFetch("/jobs")).rejects.toThrow();
  });
});
