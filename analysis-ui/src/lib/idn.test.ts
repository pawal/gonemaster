import { describe, expect, it } from "vitest";
import { idnToUnicode } from "./idn";

describe("idnToUnicode", () => {
  it("returns ASCII domains unchanged", () => {
    expect(idnToUnicode("example.com")).toBe("example.com");
  });

  it("decodes a single IDN label", () => {
    expect(idnToUnicode("xn--vermgensberater-ctb")).toBe("vermögensberater");
  });

  it("decodes IDN labels mixed with ASCII labels", () => {
    expect(idnToUnicode("xn--bcher-kva.example")).toBe("bücher.example");
  });

  it("decodes multiple IDN labels", () => {
    expect(idnToUnicode("xn--bcher-kva.xn--vermgensberater-ctb")).toBe(
      "bücher.vermögensberater"
    );
  });

  it("handles uppercase XN-- prefix", () => {
    expect(idnToUnicode("XN--bcher-kva.example")).toBe("bücher.example");
  });

  it("returns the original label when punycode is malformed", () => {
    expect(idnToUnicode("xn--!!!.example")).toBe("xn--!!!.example");
  });

  it("returns empty input unchanged", () => {
    expect(idnToUnicode("")).toBe("");
  });
});
