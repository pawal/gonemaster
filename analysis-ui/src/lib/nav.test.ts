import { describe, expect, it } from "vitest";
import { isActive, navItems } from "./nav";

describe("nav isActive", () => {
  it("matches only the exact path for the overview entry", () => {
    const overview = navItems.find((i) => i.href === "/")!;
    expect(isActive("/", overview)).toBe(true);
    expect(isActive("", overview)).toBe(true);
    expect(isActive("/domains", overview)).toBe(false);
  });

  it("matches descendant paths for non-end entries", () => {
    const domains = navItems.find((i) => i.href === "/domains")!;
    expect(isActive("/domains", domains)).toBe(true);
    expect(isActive("/domains/", domains)).toBe(true);
    expect(isActive("/domains/example.com", domains)).toBe(true);
    expect(isActive("/nameservers", domains)).toBe(false);
  });

  it("does not mistake distinct prefixes (domains vs domainsX)", () => {
    const domains = navItems.find((i) => i.href === "/domains")!;
    expect(isActive("/domainsX", domains)).toBe(false);
  });
});
