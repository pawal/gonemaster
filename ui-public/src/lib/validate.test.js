import { describe, it, expect } from "vitest";
import { validateDomain, buildJobOpts, emptyNsRow, emptyDsRow } from "./validate.js";

describe("validateDomain", () => {
  it.each([
    ["accepts a plain domain", "example.com", null],
    ["accepts a multi-label domain", "sub.example.co.uk", null],
    ["accepts a punycode domain", "xn--rksmrgs-5wao1o.se", null],
    ["requires a non-empty string", "", "pub.error_domain_required"],
    ["requires more than blanks", "   ", "pub.error_domain_required"],
    ["requires a value, not null", null, "pub.error_domain_required"],
    ["requires a value, not undefined", undefined, "pub.error_domain_required"],
    ["rejects embedded whitespace", "ex ample.com", "pub.error_domain_invalid"],
    ["rejects a domain over 253 chars", "a".repeat(254), "pub.error_domain_invalid"],
    ["rejects dots only", "...", "pub.error_domain_invalid"],
    ["rejects hyphens only", "---", "pub.error_domain_invalid"],
  ])("%s", (_name, input, want) => {
    expect(validateDomain(input)).toBe(want);
  });
});

describe("buildJobOpts", () => {
  it.each([
    ["default", {}],
    ["disable_ipv4", { ipv4_disabled: true }],
    ["disable_ipv6", { ipv6_disabled: true }],
  ])("maps the %s IP mode with no NS/DS rows", (mode, want) => {
    expect(buildJobOpts(mode, [], [])).toEqual(want);
  });

  it.each([
    ["NS only when IP is blank", { ns: "ns1.example.com", ip: "" }, { ns: "ns1.example.com" }],
    [
      "the IP when provided",
      { ns: "ns1.example.com", ip: "192.0.2.1" },
      { ns: "ns1.example.com", ip: "192.0.2.1" },
    ],
  ])("includes nameservers with %s", (_name, row, want) => {
    expect(buildJobOpts("default", [row], [])).toEqual({ nameservers: [want] });
  });

  it("skips NS rows where ns is blank", () => {
    const ns = [{ ns: "", ip: "192.0.2.1" }, { ns: "ns1.example.com", ip: "" }];
    const result = buildJobOpts("default", ns, []);
    expect(result.nameservers).toHaveLength(1);
    expect(result.nameservers[0].ns).toBe("ns1.example.com");
  });

  it("includes DS info when all fields are set", () => {
    const ds = [{ keytag: "12345", algorithm: "13", digtype: "2", digest: "ABCD" }];
    expect(buildJobOpts("default", [], ds)).toEqual({
      ds_info: [{ keytag: 12345, algorithm: 13, digtype: 2, digest: "ABCD" }],
    });
  });

  it("skips DS rows with missing fields", () => {
    const ds = [{ keytag: "12345", algorithm: "13", digtype: "", digest: "ABCD" }];
    const result = buildJobOpts("default", [], ds);
    expect(result.ds_info).toBeUndefined();
  });

  it("combines nameservers + DS + IP mode", () => {
    const ns = [{ ns: "ns1.example.com", ip: "" }];
    const ds = [{ keytag: "1", algorithm: "13", digtype: "2", digest: "FF" }];
    const result = buildJobOpts("disable_ipv6", ns, ds);
    expect(result.ipv6_disabled).toBe(true);
    expect(result.nameservers).toHaveLength(1);
    expect(result.ds_info).toHaveLength(1);
  });
});

describe("emptyNsRow / emptyDsRow", () => {
  it("emptyNsRow returns { ns, ip } with empty strings", () => {
    expect(emptyNsRow()).toEqual({ ns: "", ip: "" });
  });

  it("emptyDsRow returns { keytag, algorithm, digtype, digest } with empty strings", () => {
    expect(emptyDsRow()).toEqual({ keytag: "", algorithm: "", digtype: "", digest: "" });
  });
});
