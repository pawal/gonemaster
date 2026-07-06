import { render, screen, fireEvent, waitFor, within, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import BatchesPanel from "./BatchesPanel.svelte";

const baseProps = (overrides = {}) => ({
  apiFetch: vi.fn().mockResolvedValue({ items: [] }),
  batchSortOptions: [
    { id: "started_at_desc", labelKey: "sort_started_at_desc" },
  ],
  batchStatuses: ["", "queued", "running", "succeeded"],
  listPageSizes: [10, 20, 50, 100],
  selectedBatchId: "",
  batchSort: "started_at_desc",
  batchPageSize: 20,
  batchStatusFilter: "",
  batchDomainFilter: "",
  batchCursor: 0,
  ...overrides,
});

describe("BatchesPanel", () => {
  let apiFetch;

  beforeEach(() => {
    apiFetch = vi.fn().mockImplementation((path) => {
      if (path.startsWith("/jobs?")) return Promise.resolve({ items: [] });
      if (path === "/metrics?window=1h&include=health") return Promise.resolve({ health: { queue_paused: false } });
      if (path === "/jobs/batch") return Promise.resolve({ batch_id: "batch_new" });
      if (path.startsWith("/batches/")) {
        return Promise.resolve({
          batch_id: "batch_new",
          tag: "",
          total: 1,
          status_counts: { running: 1 },
          items: [{ id: "job_1", domain: "example.com", status: "running", progress: 50 }],
          created_at: "2026-04-01T00:00:00Z",
          next_cursor: "",
          prev_cursor: "",
          offset: 0,
        });
      }
      return Promise.resolve({});
    });
  });

  afterEach(() => cleanup());

  it("loads recent batches and active batches on mount", async () => {
    render(BatchesPanel, { props: baseProps({ apiFetch }) });
    await waitFor(() => {
      expect(apiFetch).toHaveBeenCalledWith(expect.stringContaining("/jobs?"));
      expect(apiFetch).toHaveBeenCalledWith(expect.stringContaining("/metrics?"));
    });
  });

  it("submits a batch from the domains form and selects the new id", async () => {
    render(BatchesPanel, { props: baseProps({ apiFetch }) });
    await waitFor(() => expect(apiFetch).toHaveBeenCalled());
    const textarea = screen.getByLabelText(/Domains/i);
    await fireEvent.input(textarea, { target: { value: "example.com\nalpha.test" } });
    await fireEvent.click(screen.getByRole("button", { name: /Run batch/i }));
    await waitFor(() => {
      expect(apiFetch).toHaveBeenCalledWith("/jobs/batch", expect.objectContaining({ method: "POST" }));
    });
    expect(await screen.findByText(/batch_new/)).toBeInTheDocument();
  });

  it("warns when submitting a batch with no domains", async () => {
    const setStatus = vi.fn();
    render(BatchesPanel, { props: baseProps({ apiFetch, setStatus }) });
    await waitFor(() => expect(apiFetch).toHaveBeenCalled());
    await fireEvent.click(screen.getByRole("button", { name: /Run batch/i }));
    expect(setStatus).toHaveBeenCalledWith(expect.any(String), "warn");
  });

  it("loads the selected batch when mounted with a selectedBatchId", async () => {
    render(BatchesPanel, { props: baseProps({ apiFetch, selectedBatchId: "batch_existing" }) });
    await waitFor(() => {
      expect(apiFetch).toHaveBeenCalledWith(expect.stringMatching(/^\/batches\/batch_existing\?/));
    });
  });

  it("invokes onOpenBatchDelete when the delete button is clicked", async () => {
    const onOpenBatchDelete = vi.fn();
    render(BatchesPanel, {
      props: baseProps({ apiFetch, selectedBatchId: "batch_existing", onOpenBatchDelete }),
    });
    await waitFor(() => expect(apiFetch).toHaveBeenCalled());
    await fireEvent.click(screen.getByRole("button", { name: /Delete batch/i }));
    expect(onOpenBatchDelete).toHaveBeenCalledWith("batch_existing");
  });

  it("shows severity pills and a grade chip on graduated batch job rows and navigates on click", async () => {
    const onNavigateJob = vi.fn();
    const gradedFetch = vi.fn().mockImplementation((path) => {
      if (path.startsWith("/batches/")) {
        return Promise.resolve({
          batch_id: "batch_graded",
          tag: "",
          total: 1,
          status_counts: { succeeded: 1 },
          items: [{
            id: "job_done",
            domain: "graded.example",
            status: "succeeded",
            progress: 100,
            severity_totals: { NOTICE: 0, WARNING: 2, ERROR: 1, CRITICAL: 0 },
            grade: "C",
            score: 71,
          }],
          created_at: "2026-04-01T00:00:00Z",
          next_cursor: "",
          prev_cursor: "",
          offset: 0,
        });
      }
      return Promise.resolve({ items: [] });
    });
    render(BatchesPanel, {
      props: baseProps({ apiFetch: gradedFetch, selectedBatchId: "batch_graded", scoringEnabled: true, onNavigateJob }),
    });

    const row = (await screen.findByText("job_done")).closest(".list-item");
    expect(row).not.toBeNull();
    expect(within(row).getByText(/WARNING 2/)).toBeInTheDocument();
    expect(within(row).getByText(/ERROR 1/)).toBeInTheDocument();
    expect(within(row).getByText("C")).toBeInTheDocument();

    await fireEvent.click(row);
    expect(onNavigateJob).toHaveBeenCalledWith("job_done");
  });
});
