import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/svelte";
import type { TagDetailPageData } from "./+page";

const h = vi.hoisted(() => ({ url: new URL("http://localhost/tags/DS10_NSEC_QUERY_RESPONSE_ERR") }));
vi.mock("$app/paths", () => ({ base: "/analysis" }));
vi.mock("$app/state", () => ({
  page: {
    get url() {
      return h.url;
    },
    data: {}
  }
}));

import TagDetailPage from "./+page.svelte";

function pageData(overrides: Partial<TagDetailPageData> = {}): TagDetailPageData {
  return {
    tag: "DS10_NSEC_QUERY_RESPONSE_ERR",
    datasetTag: "tld",
    snapshot: "2026-07-20-cdbee862ffe3",
    detail: null,
    history: [],
    notInSnapshot: false,
    error: null,
    ...overrides
  };
}

describe("tag detail absent-in-snapshot state", () => {
  it("renders a friendly note naming the snapshot instead of a raw error", () => {
    // The diff page's "cleared" tags used to 404; a stray link should now land
    // on an informational note, not the alarming "Failed to load tag" banner.
    render(TagDetailPage, { data: pageData({ notInSnapshot: true }) });
    expect(screen.getByText(/No findings with this tag in the selected snapshot/)).toBeInTheDocument();
    expect(screen.getByText("2026-07-20-cdbee862ffe3")).toBeInTheDocument();
    expect(screen.queryByText(/Failed to load tag/)).toBeNull();
  });

  it("still shows the history sparkline so the reader sees when the tag was present", () => {
    render(TagDetailPage, {
      data: pageData({
        notInSnapshot: true,
        history: [
          { slug: "s1", captured_at: "2026-07-14T00:00:00Z", present: true, domain_count: 3 },
          { slug: "s2", captured_at: "2026-07-20T00:00:00Z", present: true, domain_count: 0 }
        ]
      })
    });
    expect(screen.getByText("Domains over snapshots")).toBeInTheDocument();
  });

  it("keeps the raw error banner for genuine failures", () => {
    render(TagDetailPage, { data: pageData({ error: "kaboom" }) });
    expect(screen.getByText(/Failed to load tag: kaboom/)).toBeInTheDocument();
    expect(screen.queryByText(/No findings with this tag/)).toBeNull();
  });
});
