import { describe, expect, it, vi } from "vitest";
import { load } from "./+page";

function stubResponse(body: unknown, ok = true): Response {
  return {
    ok,
    status: ok ? 200 : 500,
    statusText: ok ? "OK" : "Server Error",
    headers: new Headers({ "content-type": "application/json" }),
    json: async () => body,
    text: async () => JSON.stringify(body)
  } as unknown as Response;
}

describe("/cohorts +page.load", () => {
  it("returns the cohort list from the public API", async () => {
    const cohorts = [
      { dataset_tag: "tld", label: "TLD", is_default: true },
      { dataset_tag: "gov", label: "Government", is_default: false }
    ];
    const fetchFn = vi.fn().mockResolvedValue(stubResponse(cohorts));
    const data = await load({ fetch: fetchFn as unknown as typeof fetch });

    expect(data.cohorts).toEqual(cohorts);
    expect(data.error).toBeNull();
    expect(fetchFn).toHaveBeenCalledWith("/pub/api/v1/analysis/cohorts");
  });

  it("returns an empty list with an error message when the fetch fails", async () => {
    const fetchFn = vi.fn().mockResolvedValue(stubResponse({}, false));
    const data = await load({ fetch: fetchFn as unknown as typeof fetch });
    expect(data.cohorts).toEqual([]);
    expect(data.error).toMatch(/HTTP 500/);
  });
});
