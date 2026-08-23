import { render, screen, fireEvent, waitFor, within } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import TagsPanel from "./TagsPanel.svelte";

const sampleTags = () => [
  { name: "tld", description: "Top-level", domain_count: 5, default_profile_id: null },
  { name: "gov", description: "", domain_count: 1, default_profile_id: 7 },
];

const sampleSummary = () => ({ ok: 4, notice: 0, warning: 1, error: 0, critical: 0 });

const sampleTagDomains = () => ({
  items: [{ name: "alpha.test", latest_level: "INFO", latest_run_at: "2026-04-01T00:00:00Z" }],
  total: 1,
});

const sampleTagBatches = () => ({
  items: [{ id: "batch_1", created_at: "2026-04-01T00:00:00Z", domain_count: 3 }],
  total: 1,
});

describe("TagsPanel", () => {
  let apiFetch;

  beforeEach(() => {
    apiFetch = vi.fn().mockImplementation((path, options = {}) => {
      const method = options.method || "GET";
      if (path === "/tags" && method === "GET") return Promise.resolve(sampleTags());
      if (path === "/tags" && method === "POST") return Promise.resolve({ name: "new" });
      if (path.match(/^\/tags\/[^/]+\/summary$/)) return Promise.resolve(sampleSummary());
      if (path.match(/^\/tags\/[^/]+\/domains/)) return Promise.resolve(sampleTagDomains());
      if (path.match(/^\/tags\/[^/]+\/batches/)) return Promise.resolve(sampleTagBatches());
      return Promise.resolve({});
    });
  });

  it("loads and renders the tag list on mount", async () => {
    render(TagsPanel, { props: { apiFetch } });
    expect(await screen.findByText("tld")).toBeInTheDocument();
    expect(screen.getByText("gov")).toBeInTheDocument();
  });

  it("distinguishes a load error from an empty list and offers a retry", async () => {
    let attempt = 0;
    const failing = vi.fn().mockImplementation((path) => {
      if (path === "/tags") {
        attempt += 1;
        if (attempt === 1) return Promise.reject(new Error("boom"));
        return Promise.resolve([]);
      }
      return Promise.resolve({ items: [], total: 0 });
    });
    render(TagsPanel, { props: { apiFetch: failing } });

    // Error state, not the "no tags" empty state.
    const retry = await screen.findByRole("button", { name: /Retry/i });
    expect(screen.queryByText(/No tags/i)).toBeNull();

    // Retrying succeeds and now shows the genuine empty state.
    await fireEvent.click(retry);
    expect(await screen.findByText(/No tags/i)).toBeInTheDocument();
  });

  it("creates a new tag when the form is submitted", async () => {
    render(TagsPanel, { props: { apiFetch } });
    await screen.findByText("tld");
    await fireEvent.input(screen.getByPlaceholderText("my-tag"), { target: { value: "newtag" } });
    await fireEvent.click(screen.getByRole("button", { name: /^Create$/i }));
    await waitFor(() => {
      expect(apiFetch).toHaveBeenCalledWith("/tags", expect.objectContaining({ method: "POST" }));
    });
  });

  it("renders the detail view when a tag name is routed", async () => {
    render(TagsPanel, {
      props: { apiFetch, routeTagName: "tld" },
    });
    expect(await screen.findByRole("heading", { level: 2, name: /tld/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Back to tags/i })).toBeInTheDocument();
  });

  it("loads tag summary, domains, and batches when a tag name is routed", async () => {
    render(TagsPanel, {
      props: { apiFetch, routeTagName: "tld" },
    });
    await waitFor(() => {
      expect(apiFetch).toHaveBeenCalledWith(expect.stringContaining("/tags/tld/summary"));
      expect(apiFetch).toHaveBeenCalledWith(expect.stringContaining("/tags/tld/domains"));
      expect(apiFetch).toHaveBeenCalledWith(expect.stringContaining("/tags/tld/batches"));
    });
  });

  it("shows an explicit confirm step before deleting a tag", async () => {
    render(TagsPanel, {
      props: { apiFetch, routeTagName: "tld" },
    });
    await screen.findByRole("heading", { level: 2, name: /tld/ });
    await fireEvent.click(screen.getByRole("button", { name: /^Delete tag$/i }));
    expect(await screen.findByRole("button", { name: /Confirm delete/i })).toBeInTheDocument();
  });

  it("requires typing the tag name before purging runs (typed confirm)", async () => {
    render(TagsPanel, { props: { apiFetch, routeTagName: "tld" } });
    await screen.findByRole("heading", { level: 2, name: /tld/ });
    await fireEvent.click(screen.getByRole("button", { name: /^Purge runs$/i }));

    const dialog = await screen.findByRole("dialog");
    const confirm = within(dialog).getByRole("button", { name: /Confirm purge/i });
    expect(confirm).toBeDisabled();
    await fireEvent.input(within(dialog).getByRole("textbox"), { target: { value: "tld" } });
    expect(confirm).not.toBeDisabled();
  });

  it("invokes onSetTab when the cohort link is clicked", async () => {
    const onSetTab = vi.fn();
    const tagCohortByName = new Map([["tld", { source_tag: "tld", label: "TLD cohort" }]]);
    render(TagsPanel, {
      props: { apiFetch, routeTagName: "tld", tagCohortByName, onSetTab },
    });
    await screen.findByText(/TLD cohort/);
    await fireEvent.click(screen.getByRole("button", { name: /TLD cohort/ }));
    expect(onSetTab).toHaveBeenCalledWith("cohorts");
  });
});
