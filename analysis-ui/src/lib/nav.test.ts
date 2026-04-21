import { describe, expect, it } from "vitest";
import { isActive, navItems, visibleNavItems } from "./nav";

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

describe("nav visibleNavItems", () => {
  it("hides the cohorts tab when only one cohort is available", () => {
    const result = visibleNavItems(navItems, 1);
    expect(result.find((i) => i.href === "/cohorts")).toBeUndefined();
    // Other tabs stay intact.
    expect(result.find((i) => i.href === "/domains")).toBeDefined();
    expect(result.find((i) => i.href === "/")).toBeDefined();
    expect(result.length).toBe(navItems.length - 1);
  });

  it("hides the cohorts tab when zero cohorts are available", () => {
    const result = visibleNavItems(navItems, 0);
    expect(result.find((i) => i.href === "/cohorts")).toBeUndefined();
  });

  it("shows the cohorts tab when more than one cohort is available", () => {
    const result = visibleNavItems(navItems, 2);
    expect(result.find((i) => i.href === "/cohorts")).toBeDefined();
    expect(result.length).toBe(navItems.length);
  });

  it("preserves the original order of the remaining items", () => {
    const result = visibleNavItems(navItems, 1);
    const expected = navItems
      .filter((i) => i.href !== "/cohorts")
      .map((i) => i.href);
    expect(result.map((i) => i.href)).toEqual(expected);
  });

  it("does not mutate the input array", () => {
    const before = navItems.map((i) => i.href);
    visibleNavItems(navItems, 1);
    expect(navItems.map((i) => i.href)).toEqual(before);
  });
});
