import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import { load, type CohortsPageData } from "./+page";
import type { CatalogResponse } from "$lib/api";

const h = vi.hoisted(() => ({ url: new URL("http://localhost/analysis/cohorts") }));
vi.mock("$app/paths", () => ({ base: "/analysis" }));
vi.mock("$app/state", () => ({
  page: {
    get url() {
      return h.url;
    }
  }
}));

import CohortsPage from "./+page.svelte";

function evt(catalog: CatalogResponse | null, catalogError: string | null = null) {
  return {
    parent: async () => ({ catalog, catalogError })
  } as Parameters<typeof load>[0];
}

describe("/cohorts +page.load", () => {
  it("reuses the catalog from the layout instead of refetching", async () => {
    const catalog: CatalogResponse = {
      cohorts: [
        { dataset_tag: "tld", label: "TLD", is_default: true },
        { dataset_tag: "gov", label: "Government", is_default: false }
      ],
      selector_enabled: true,
      backend_supported: true
    };
    const data = await load(evt(catalog));
    expect(data.cohorts).toEqual(catalog.cohorts);
    expect(data.error).toBeNull();
  });

  it("surfaces the catalog error from the layout", async () => {
    const data = await load(evt(null, "HTTP 500"));
    expect(data.cohorts).toEqual([]);
    expect(data.error).toMatch(/HTTP 500/);
  });

  it("returns an empty list when the catalog has no cohorts", async () => {
    const data = await load(evt({ cohorts: [], selector_enabled: false, backend_supported: true }));
    expect(data.cohorts).toEqual([]);
    expect(data.error).toBeNull();
  });
});

function pageData(overrides: Partial<CohortsPageData> = {}): CohortsPageData {
  return {
    cohorts: [
      {
        dataset_tag: "tld",
        label: "TLD",
        is_default: true,
        snapshot_count: 4,
        default_snapshot: {
          slug: "2026-04-20",
          captured_at: "2026-04-20T00:00:00Z",
          last_run_at: "2026-04-20T00:00:00Z",
          run_count: 1,
          domain_count: 1234
        }
      }
    ],
    error: null,
    ...overrides
  };
}

describe("cohorts page rendering", () => {
  it("shows per-cohort domain, snapshot, and latest-date stats from the catalog", () => {
    render(CohortsPage, { data: pageData() });
    expect(screen.getByText("1,234")).toBeInTheDocument(); // domain count
    expect(screen.getByText("4")).toBeInTheDocument(); // snapshot count
    expect(screen.getByText("2026-04-20")).toBeInTheDocument(); // latest date
  });

  it("notes cohorts that have no captured snapshot yet", () => {
    render(CohortsPage, {
      data: pageData({
        cohorts: [{ dataset_tag: "new", label: "New", is_default: false }]
      })
    });
    expect(screen.getByText(/no captured snapshot yet/i)).toBeInTheDocument();
  });
});
