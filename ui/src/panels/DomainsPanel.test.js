import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import DomainsPanel from "./DomainsPanel.svelte";

const sampleDomains = () => ({
  items: [
    { id: 1, name: "example.com", tags: ["tld"], latest_level: "INFO", latest_run_at: "2026-04-01T00:00:00Z", run_count: 3 },
    { id: 2, name: "alpha.test", tags: [], latest_level: "WARNING", latest_run_at: "2026-04-02T00:00:00Z", run_count: 1 },
  ],
  total: 2,
});

const sampleRuns = () => ({
  items: [
    { id: "run_1", finished_at: "2026-04-03T12:00:00Z", worst_level: "INFO", duration_ms: 200, entry_count: 5 },
  ],
  total: 1,
});

const sampleResult = () => ({
  job_id: "run_1",
  summary: { levels: { NOTICE: 1, WARNING: 0, ERROR: 0, CRITICAL: 0 } },
  raw: { entries: [] },
});

describe("DomainsPanel", () => {
  let apiFetch;

  beforeEach(() => {
    apiFetch = vi.fn().mockImplementation((path) => {
      if (path.startsWith("/domains?")) return Promise.resolve(sampleDomains());
      if (path.match(/^\/domains\/\d+\/runs/)) return Promise.resolve(sampleRuns());
      if (path.match(/^\/jobs\/.+\/result/)) return Promise.resolve(sampleResult());
      if (path === "/jobs") return Promise.resolve({ id: "job_new" });
      return Promise.resolve({});
    });
  });

  it("loads the domains list on mount when no domain is selected", async () => {
    render(DomainsPanel, { props: { apiFetch } });
    expect(await screen.findByText("example.com")).toBeInTheDocument();
    expect(screen.getByText("alpha.test")).toBeInTheDocument();
  });

  it("renders the detail view when a domain name is routed", async () => {
    render(DomainsPanel, {
      props: {
        apiFetch,
        routeDomainName: "example.com",
      },
    });
    expect(await screen.findByRole("heading", { name: "example.com" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Back to domains/i })).toBeInTheDocument();
  });

  it("loads runs when mounted with a routed domain name", async () => {
    render(DomainsPanel, {
      props: { apiFetch, routeDomainName: "example.com" },
    });
    await waitFor(() => {
      expect(apiFetch).toHaveBeenCalledWith(expect.stringMatching(/^\/domains\/1\/runs/));
    });
  });

  it("clicking Back clears the selection and requests navigation to the list", async () => {
    const onCloseDomain = vi.fn();
    render(DomainsPanel, {
      props: { apiFetch, routeDomainName: "example.com", onCloseDomain },
    });
    await screen.findByRole("heading", { name: "example.com" });
    await fireEvent.click(screen.getByRole("button", { name: /Back to domains/i }));
    expect(onCloseDomain).toHaveBeenCalled();
    // List view shows the search input.
    expect(await screen.findByPlaceholderText(/Search/i)).toBeInTheDocument();
  });

  it("opens the detail view and requests navigation when a row is clicked", async () => {
    const onOpenDomain = vi.fn();
    render(DomainsPanel, { props: { apiFetch, routeDomainName: null, onOpenDomain } });
    const row = (await screen.findByText("example.com")).closest("tr");
    await fireEvent.click(row);
    expect(onOpenDomain).toHaveBeenCalledWith("example.com");
    // Detail header should appear.
    expect(await screen.findByRole("heading", { name: "example.com" })).toBeInTheDocument();
  });

  it("posts a /jobs request and calls onNavigateJob when Re-test is clicked", async () => {
    const onNavigateJob = vi.fn();
    render(DomainsPanel, {
      props: {
        apiFetch,
        routeDomainName: "example.com",
        onNavigateJob,
      },
    });
    const retest = await screen.findByRole("button", { name: /Re-test/i });
    await fireEvent.click(retest);
    await waitFor(() => {
      expect(onNavigateJob).toHaveBeenCalledWith("job_new");
    });
  });
});
