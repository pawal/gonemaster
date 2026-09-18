import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { describe, expect, it, vi } from "vitest";
import RecentJobsPanel from "./RecentJobsPanel.svelte";

const sampleJobs = () => ({
  items: [
    { id: "job_a", domain: "example.com", status: "running", progress: 40, severity_totals: { NOTICE: 0, WARNING: 1, ERROR: 0, CRITICAL: 0 } },
    { id: "job_b", domain: "alpha.test", status: "succeeded", progress: 100, severity_totals: { NOTICE: 0, WARNING: 0, ERROR: 1, CRITICAL: 0 } },
  ],
  total: 2,
  next_cursor: "",
  prev_cursor: "",
  offset: 0,
});

const baseProps = (overrides = {}) => ({
  apiFetch: vi.fn().mockResolvedValue(sampleJobs()),
  severityFilters: [
    { id: "all", labelKey: "sev_all" },
    { id: "warnings_plus", labelKey: "sev_warnings_plus" },
    { id: "errors_only", labelKey: "sev_errors_only" },
  ],
  jobSortOptions: [
    { id: "started_at_desc", labelKey: "sort_started_at_desc" },
    { id: "domain_asc", labelKey: "sort_domain_asc" },
  ],
  listPageSizes: [10, 20, 50, 100],
  scoringEnabled: false,
  jobSort: "started_at_desc",
  severityFilter: "all",
  jobBatchFilter: "",
  recentDomainFilter: "",
  recentPageSize: 20,
  recentCursor: 0,
  ...overrides,
});

describe("RecentJobsPanel", () => {
  it("loads and renders jobs on mount", async () => {
    const apiFetch = vi.fn().mockResolvedValue(sampleJobs());
    render(RecentJobsPanel, { props: baseProps({ apiFetch }) });
    expect(await screen.findByText("job_a")).toBeInTheDocument();
    expect(screen.getByText("job_b")).toBeInTheDocument();
  });

  it("includes filters and sort in the jobs query", async () => {
    const apiFetch = vi.fn().mockResolvedValue(sampleJobs());
    render(RecentJobsPanel, {
      props: baseProps({
        apiFetch,
        jobSort: "domain_asc",
        jobBatchFilter: "batch_x",
        recentDomainFilter: "example.com",
        recentPageSize: 50,
        severityFilter: "errors_only",
      }),
    });
    await waitFor(() => expect(apiFetch).toHaveBeenCalled());
    const url = apiFetch.mock.calls[0][0];
    expect(url).toMatch(/sort=domain_asc/);
    expect(url).toMatch(/batch_id=batch_x/);
    expect(url).toMatch(/domain=example.com/);
    expect(url).toMatch(/limit=50/);
    expect(url).toMatch(/severity=errors_only/);
  });

  it("invokes onNavigateJob when a job row is clicked", async () => {
    const onNavigateJob = vi.fn();
    render(RecentJobsPanel, { props: baseProps({ onNavigateJob }) });
    await screen.findByText("job_a");
    await fireEvent.click(screen.getByText("job_a"));
    expect(onNavigateJob).toHaveBeenCalledWith("job_a");
  });

  it("renders exactly the jobs the server returns (severity filtering is server-side)", async () => {
    // The server already applied the filter, so the panel must not drop rows.
    const apiFetch = vi.fn().mockResolvedValue(sampleJobs());
    render(RecentJobsPanel, {
      props: baseProps({ apiFetch, severityFilter: "errors_only" }),
    });
    expect(await screen.findByText("job_a")).toBeInTheDocument();
    expect(screen.getByText("job_b")).toBeInTheDocument();
  });

  it("shows the severity-specific empty message when a filter returns nothing", async () => {
    const apiFetch = vi.fn().mockResolvedValue({ items: [], total: 0, offset: 0 });
    render(RecentJobsPanel, {
      props: baseProps({ apiFetch, severityFilter: "warnings_plus" }),
    });
    expect(await screen.findByText(/No jobs match the selected severity filter/i)).toBeInTheDocument();
  });

  it("re-queries when Apply filters is clicked", async () => {
    const apiFetch = vi.fn().mockResolvedValue(sampleJobs());
    render(RecentJobsPanel, { props: baseProps({ apiFetch }) });
    await screen.findByText("job_a");
    apiFetch.mockClear();
    await fireEvent.click(screen.getByRole("button", { name: /Apply filters/i }));
    await waitFor(() => expect(apiFetch).toHaveBeenCalled());
  });

  it("renders the job id as a real anchor to the single-job route", async () => {
    const onNavigateJob = vi.fn();
    render(RecentJobsPanel, { props: baseProps({ onNavigateJob }) });
    const link = await screen.findByText("job_a");
    expect(link.tagName).toBe("A");
    expect(link.getAttribute("href")).toBe("#/single/job_a");
    await fireEvent.click(link);
    expect(onNavigateJob).toHaveBeenCalledWith("job_a");
  });

  it("shows how long ago a job ran and hides the progress bar once it is done", async () => {
    const finished = new Date(Date.now() - 3 * 3600 * 1000).toISOString();
    const apiFetch = vi.fn().mockResolvedValue({
      items: [{ id: "job_done", domain: "example.com", status: "succeeded", progress: 100, finished_at: finished, severity_totals: {} }],
      total: 1,
      offset: 0,
    });
    const { container } = render(RecentJobsPanel, { props: baseProps({ apiFetch }) });
    expect(await screen.findByText("3h ago")).toBeInTheDocument();
    expect(container.querySelector(".recent-progress")).toBeNull();
  });

  it("keeps the progress bar on a job still in flight", async () => {
    const apiFetch = vi.fn().mockResolvedValue({
      items: [{ id: "job_run", domain: "example.com", status: "running", progress: 40 }],
      total: 1,
      offset: 0,
    });
    const { container } = render(RecentJobsPanel, { props: baseProps({ apiFetch }) });
    await screen.findByText("job_run");
    expect(container.querySelector(".recent-progress")).not.toBeNull();
  });

  it("reports the page position against the server total", async () => {
    const apiFetch = vi.fn().mockResolvedValue({ ...sampleJobs(), total: 26063, offset: 40 });
    render(RecentJobsPanel, { props: baseProps({ apiFetch }) });
    expect(await screen.findByText("41-42 of 26,063")).toBeInTheDocument();
  });

  it("links a job's batch id to the batches route", async () => {
    const onNavigateBatch = vi.fn();
    const withBatch = () => ({ items: [{ id: "job_a", domain: "example.com", status: "succeeded", progress: 100, batch_id: "batch_x", severity_totals: {} }], total: 1, offset: 0 });
    render(RecentJobsPanel, { props: baseProps({ apiFetch: vi.fn().mockResolvedValue(withBatch()), onNavigateBatch }) });
    const link = await screen.findByText("batch_x");
    expect(link.tagName).toBe("A");
    expect(link.getAttribute("href")).toBe("#/batches/batch_x");
    await fireEvent.click(link);
    expect(onNavigateBatch).toHaveBeenCalledWith("batch_x");
  });
});
