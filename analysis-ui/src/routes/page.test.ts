import { render, screen, waitFor, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Page from "./+page.svelte";

const jsonResponse = (data: unknown, ok = true): Response =>
  ({
    ok,
    status: ok ? 200 : 500,
    statusText: ok ? "OK" : "Server Error",
    headers: new Headers({ "content-type": "application/json" }),
    json: async () => data,
    text: async () => JSON.stringify(data)
  }) as Response;

describe("analysis landing page", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    globalThis.fetch = vi.fn();
  });

  afterEach(() => cleanup());

  it("renders catalog cohorts after a successful fetch", async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation(async (url: string) => {
      if (url === "/pub/api/v1/analysis/catalog") {
        return jsonResponse({
          default_tag: "tld",
          selector_enabled: true,
          cohorts: [
            { dataset_tag: "tld", label: "TLD", is_default: true },
            { dataset_tag: "gov", label: "Government", is_default: false }
          ]
        });
      }
      return jsonResponse({}, false);
    });

    render(Page);
    await waitFor(() => {
      expect(screen.getByText("TLD")).toBeInTheDocument();
      expect(screen.getByText("Government")).toBeInTheDocument();
      expect(screen.getByText("default")).toBeInTheDocument();
    });
  });

  it("shows an empty-state message when there are no public cohorts", async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation(async () =>
      jsonResponse({ selector_enabled: false, cohorts: [] })
    );

    render(Page);
    await waitFor(() => {
      expect(screen.getByText(/No public analysis cohorts/i)).toBeInTheDocument();
    });
  });

  it("surfaces fetch errors", async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation(async () =>
      jsonResponse({ error: { message: "boom" } }, false)
    );

    render(Page);
    await waitFor(() => {
      expect(screen.getByText(/Failed to load catalog/i)).toBeInTheDocument();
    });
  });
});
