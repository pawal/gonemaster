import { describe, expect, it } from "vitest";
import { testcaseTitle } from "./testcaseTitles";

describe("testcaseTitle", () => {
  // Regression guard: zone14 and dnssec21 exist in the engine and in the
  // admin UI catalog (ui/src/i18n/en.json tc.* keys) but were missing here,
  // so their chips fell back to the raw id.
  it("resolves zone14 and dnssec21", () => {
    expect(testcaseTitle("zone14")).toBe("ZONEMD record at zone apex");
    expect(testcaseTitle("dnssec21")).toBe(
      "Parent zone's DS RRset is signed by a valid DNSKEY"
    );
  });

  it("is case-insensitive", () => {
    expect(testcaseTitle("ZONE14")).toBe("ZONEMD record at zone apex");
  });

  it("returns null for unknown or empty input", () => {
    expect(testcaseTitle("zone99")).toBe(null);
    expect(testcaseTitle("")).toBe(null);
    expect(testcaseTitle(null)).toBe(null);
    expect(testcaseTitle(undefined)).toBe(null);
  });
});
