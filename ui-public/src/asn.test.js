import { describe, expect, it } from "vitest";
import { asnsOf } from "./asn.js";

describe("asnsOf", () => {
  it.each([
    ["scalar", { asn: 199973 }, [199973]],
    ["list", { asns: [13335, 1299] }, [1299, 13335]],
    ["both, deduplicated", { asn: 1299, asns: [8674, 1299] }, [1299, 8674]],
    ["missing", { domain: "example.com" }, []],
    ["no args", undefined, []],
    ["junk", { asn: "1299", asns: [0, -5, 1.5, null, "x", 4294967296, [42]] }, []],
    ["upper bound", { asns: [4294967295] }, [4294967295]],
  ])("%s", (_, args, want) => {
    expect(asnsOf(args)).toEqual(want);
  });
});
