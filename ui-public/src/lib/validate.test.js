import { describe, it, expect } from "vitest";
import { validateDomain, buildJobOpts, emptyNsRow, emptyDsRow } from "./validate.js";

describe("validateDomain", () => {
  it("returns null for a valid domain", () => {
    expect(validateDomain("example.com")).toBeNull();
    expect(validateDomain("sub.example.co.uk")).toBeNull();
    expect(validateDomain("xn--rksmrgs-5wao1o.se")).toBeNull();
  });

  it("returns error key for empty string", () => {
    expect(validateDomain("")).toBe("pub.error_domain_required");
    expect(validateDomain("   ")).toBe("pub.error_domain_required");
  });

  it("returns error key for null/undefined", () => {
    expect(validateDomain(null)).toBe("pub.error_domain_required");
    expect(validateDomain(undefined)).toBe("pub.error_domain_required");
  });

  it("returns error key for domain with whitespace", () => {
    expect(validateDomain("ex ample.com")).toBe("pub.error_domain_invalid");
  });

  it("returns error key for domain over 253 chars", () => {
    expect(validateDomain("a".repeat(254))).toBe("pub.error_domain_invalid");
  });

  it("returns error key for dots/hyphens only", () => {
    expect(validateDomain("...")).toBe("pub.error_domain_invalid");
    expect(validateDomain("---")).toBe("pub.error_domain_invalid");
  });
});

describe("buildJobOpts", () => {
  it("returns empty object for defaults with no NS/DS rows", () => {
    expect(buildJobOpts("default", [], [])).toEqual({});
  });

  it("sets ipv4_disabled for disable_ipv4 mode", () => {
    expect(buildJobOpts("disable_ipv4", [], [])).toEqual({ ipv4_disabled: true });
  });

  it("sets ipv6_disabled for disable_ipv6 mode", () => {
    expect(buildJobOpts("disable_ipv6", [], [])).toEqual({ ipv6_disabled: true });
  });

  it("includes nameservers with NS only when IP is blank", () => {
    const ns = [{ ns: "ns1.example.com", ip: "" }];
    expect(buildJobOpts("default", ns, [])).toEqual({
      nameservers: [{ ns: "ns1.example.com" }],
    });
  });

  it("includes nameservers with IP when provided", () => {
    const ns = [{ ns: "ns1.example.com", ip: "192.0.2.1" }];
    expect(buildJobOpts("default", ns, [])).toEqual({
      nameservers: [{ ns: "ns1.example.com", ip: "192.0.2.1" }],
    });
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
