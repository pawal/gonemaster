import { describe, expect, it } from "vitest";
import { idnToUnicode } from "./idn";

describe("idnToUnicode", () => {
  it.each([
    ["returns ASCII domains unchanged", "example.com", "example.com"],
    ["decodes a single IDN label", "xn--vermgensberater-ctb", "vermögensberater"],
    ["decodes IDN labels mixed with ASCII labels", "xn--bcher-kva.example", "bücher.example"],
    [
      "decodes multiple IDN labels",
      "xn--bcher-kva.xn--vermgensberater-ctb",
      "bücher.vermögensberater"
    ],
    ["handles uppercase XN-- prefix", "XN--bcher-kva.example", "bücher.example"],
    ["returns the original label when punycode is malformed", "xn--!!!.example", "xn--!!!.example"],
    ["returns empty input unchanged", "", ""]
  ])("%s", (_name, input, want) => {
    expect(idnToUnicode(input)).toBe(want);
  });
});
