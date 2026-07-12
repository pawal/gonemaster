import { describe, it, expect, vi, beforeEach } from "vitest";
import { API_BASE, createJob, getJob, getResult, getLocales, getDnssecChain } from "./api.js";

describe("API_BASE", () => {
  it("points to /pub/api/v1", () => {
    expect(API_BASE).toBe("/pub/api/v1");
  });
});

describe("createJob", () => {
  beforeEach(() => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true });
  });

  it("POSTs to /pub/api/v1/jobs", async () => {
    await createJob("example.com");
    expect(fetch).toHaveBeenCalledWith(
      "/pub/api/v1/jobs",
      expect.objectContaining({ method: "POST" })
    );
  });

  it("sends domain in JSON body", async () => {
    await createJob("example.com");
    const body = JSON.parse(fetch.mock.calls[0][1].body);
    expect(body.domain).toBe("example.com");
  });

  it("merges extra opts into the request body", async () => {
    await createJob("example.com", { ipv4_disabled: true });
    const body = JSON.parse(fetch.mock.calls[0][1].body);
    expect(body.ipv4_disabled).toBe(true);
  });

  it("sets Content-Type to application/json", async () => {
    await createJob("example.com");
    const headers = fetch.mock.calls[0][1].headers;
    expect(headers["Content-Type"]).toBe("application/json");
  });
});

describe("getJob", () => {
  beforeEach(() => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true });
  });

  it("GETs /pub/api/v1/jobs/:publicID", async () => {
    await getJob("abc12345");
    expect(fetch).toHaveBeenCalledWith("/pub/api/v1/jobs/abc12345");
  });

  it("URL-encodes the publicID", async () => {
    await getJob("abc/bad");
    expect(fetch).toHaveBeenCalledWith("/pub/api/v1/jobs/abc%2Fbad");
  });
});

describe("getResult", () => {
  beforeEach(() => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true });
  });

  it("GETs /pub/api/v1/jobs/:publicID/result with locale", async () => {
    await getResult("abc12345", "sv");
    expect(fetch).toHaveBeenCalledWith(
      "/pub/api/v1/jobs/abc12345/result?locale=sv"
    );
  });

  it("defaults locale to en", async () => {
    await getResult("abc12345");
    expect(fetch).toHaveBeenCalledWith(
      "/pub/api/v1/jobs/abc12345/result?locale=en"
    );
  });
});

describe("getLocales", () => {
  beforeEach(() => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true });
  });

  it("GETs /pub/api/v1/locales", async () => {
    await getLocales();
    expect(fetch).toHaveBeenCalledWith("/pub/api/v1/locales");
  });
});

describe("getDnssecChain", () => {
  beforeEach(() => {
    global.fetch = vi.fn().mockResolvedValue({ ok: true });
  });

  it("GETs /pub/api/v1/jobs/:publicID/dnssec-chain", async () => {
    await getDnssecChain("abc12345");
    expect(fetch).toHaveBeenCalledWith("/pub/api/v1/jobs/abc12345/dnssec-chain");
  });

  it("URL-encodes the publicID", async () => {
    await getDnssecChain("a b/c");
    expect(fetch).toHaveBeenCalledWith("/pub/api/v1/jobs/a%20b%2Fc/dnssec-chain");
  });
});
