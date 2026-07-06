import { describe, it, expect } from "vitest";
import {
  TABS,
  SETTINGS_SUBS,
  normalizeTab,
  normalizeSettingsSub,
  parseRoute,
  formatRoute,
  href,
} from "./router.svelte.js";

describe("normalizeTab", () => {
  it("maps every canonical tab id to itself", () => {
    for (const tab of TABS) {
      expect(normalizeTab(tab)).toBe(tab);
    }
  });

  it("resolves the documented aliases", () => {
    expect(normalizeTab("job")).toBe("single");
    expect(normalizeTab("jobs")).toBe("single");
    expect(normalizeTab("home")).toBe("single");
    expect(normalizeTab("tests")).toBe("recent");
    expect(normalizeTab("domain")).toBe("domains");
    expect(normalizeTab("tag")).toBe("tags");
    expect(normalizeTab("analysis")).toBe("cohorts");
    expect(normalizeTab("cohort")).toBe("cohorts");
    expect(normalizeTab("batch")).toBe("batches");
    expect(normalizeTab("metric")).toBe("metrics");
    expect(normalizeTab("setting")).toBe("settings");
  });

  it("is case-insensitive and strips leading slashes", () => {
    expect(normalizeTab("/Domains")).toBe("domains");
    expect(normalizeTab("BATCHES")).toBe("batches");
  });

  it("returns empty string for unknown or blank input", () => {
    expect(normalizeTab("nope")).toBe("");
    expect(normalizeTab("")).toBe("");
    expect(normalizeTab(null)).toBe("");
    expect(normalizeTab(undefined)).toBe("");
  });
});

describe("normalizeSettingsSub", () => {
  it("keeps the known sub-tabs", () => {
    for (const sub of SETTINGS_SUBS) {
      expect(normalizeSettingsSub(sub)).toBe(sub);
    }
  });

  it("falls back to system for unknown or missing values", () => {
    expect(normalizeSettingsSub("analysis")).toBe("system");
    expect(normalizeSettingsSub("")).toBe("system");
    expect(normalizeSettingsSub(undefined)).toBe("system");
  });
});

describe("parseRoute", () => {
  it("defaults to the single tab for empty or unknown hashes", () => {
    expect(parseRoute("").tab).toBe("single");
    expect(parseRoute("#/").tab).toBe("single");
    expect(parseRoute("#/nonsense").tab).toBe("single");
  });

  it("parses bare tab hashes with no detail segment", () => {
    expect(parseRoute("#/recent")).toMatchObject({ tab: "recent", jobId: null });
    expect(parseRoute("#/metrics").tab).toBe("metrics");
    expect(parseRoute("#/cohorts").tab).toBe("cohorts");
  });

  it("parses a job id from #/single/<id>", () => {
    expect(parseRoute("#/single/job_123")).toMatchObject({
      tab: "single",
      jobId: "job_123",
    });
  });

  it("parses a domain name from #/domains/<name>", () => {
    expect(parseRoute("#/domains/example.com")).toMatchObject({
      tab: "domains",
      domainName: "example.com",
      runId: null,
    });
  });

  it("parses domain name and run id from #/domains/<name>/runs/<runId>", () => {
    expect(parseRoute("#/domains/example.com/runs/run-9")).toMatchObject({
      tab: "domains",
      domainName: "example.com",
      runId: "run-9",
    });
  });

  it("parses a tag name from #/tags/<name>", () => {
    expect(parseRoute("#/tags/tld")).toMatchObject({ tab: "tags", tagName: "tld" });
  });

  it("parses a batch id from #/batches/<id>", () => {
    expect(parseRoute("#/batches/batch_42")).toMatchObject({
      tab: "batches",
      batchId: "batch_42",
    });
  });

  it("parses the settings sub-tab and defaults it to system", () => {
    expect(parseRoute("#/settings").settingsSub).toBe("system");
    expect(parseRoute("#/settings/profiles").settingsSub).toBe("profiles");
    expect(parseRoute("#/settings/scoring").settingsSub).toBe("scoring");
    expect(parseRoute("#/settings/bogus").settingsSub).toBe("system");
  });

  it("redirects the legacy #/settings/analysis bookmark to cohorts", () => {
    expect(parseRoute("#/settings/analysis").tab).toBe("cohorts");
  });

  it("decodes percent-encoded detail segments", () => {
    expect(parseRoute("#/domains/xn--caf-dma.example").domainName).toBe(
      "xn--caf-dma.example",
    );
    expect(parseRoute("#/tags/two%20words").tagName).toBe("two words");
  });

  it("resolves tab aliases in the hash", () => {
    expect(parseRoute("#/analysis").tab).toBe("cohorts");
    expect(parseRoute("#/job/job_9")).toMatchObject({ tab: "single", jobId: "job_9" });
  });
});

describe("formatRoute", () => {
  it("round-trips every route shape through parseRoute", () => {
    const hashes = [
      "#/single",
      "#/single/job_1",
      "#/recent",
      "#/domains",
      "#/domains/example.com",
      "#/domains/example.com/runs/run-2",
      "#/tags",
      "#/tags/tld",
      "#/cohorts",
      "#/batches",
      "#/batches/batch_7",
      "#/metrics",
      "#/settings",
      "#/settings/profiles",
      "#/settings/scoring",
    ];
    for (const hash of hashes) {
      expect(formatRoute(parseRoute(hash))).toBe(hash);
    }
  });

  it("collapses the system sub-tab to the bare settings hash", () => {
    expect(formatRoute({ tab: "settings", settingsSub: "system" })).toBe("#/settings");
  });

  it("percent-encodes detail segments", () => {
    expect(formatRoute({ tab: "tags", tagName: "two words" })).toBe("#/tags/two%20words");
  });

  it("defaults a missing route to single", () => {
    expect(formatRoute(null)).toBe("#/single");
    expect(formatRoute({})).toBe("#/single");
  });
});

describe("href", () => {
  it("accepts a bare tab id", () => {
    expect(href("recent")).toBe("#/recent");
    expect(href("domains")).toBe("#/domains");
  });

  it("accepts a tab id plus detail fields", () => {
    expect(href("domains", { domainName: "example.com" })).toBe("#/domains/example.com");
    expect(href("batches", { batchId: "batch_1" })).toBe("#/batches/batch_1");
    expect(href("single", { jobId: "job_5" })).toBe("#/single/job_5");
  });

  it("accepts a full route object", () => {
    expect(href({ tab: "tags", tagName: "tld" })).toBe("#/tags/tld");
  });
});
