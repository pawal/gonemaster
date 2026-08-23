import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import BatchDeleteModal from "./BatchDeleteModal.svelte";
import { jsonResponse, noContentResponse, requestUrl } from "../test/helpers.js";

const samplePreview = (overrides = {}) => ({
  batch_id: "batch_xyz",
  tag: "tld",
  created_at: "2026-04-20T12:00:00Z",
  snapshot_intent: false,
  exists: true,
  queued_jobs: 0,
  running_jobs: 0,
  completed_runs: 5,
  entries: 120,
  fact_rows: 60,
  snapshots: [],
  ...overrides,
});

describe("BatchDeleteModal", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  const installFetch = (scenario = {}) => {
    const deleteCalls = [];
    global.fetch.mockImplementation((url, options = {}) => {
      const value = requestUrl(url);
      const method = options.method || "GET";
      if (method === "GET" && value.endsWith("/delete-preview")) {
        if (scenario.previewError) {
          return jsonResponse(
            { error: { message: scenario.previewError } },
            false,
            scenario.previewStatus || 404,
          );
        }
        return jsonResponse(scenario.preview || samplePreview());
      }
      if (method === "DELETE" && /\/batches\/[^/]+$/.test(value)) {
        deleteCalls.push(value);
        if (scenario.deleteError) {
          return jsonResponse(
            { error: { message: scenario.deleteError } },
            false,
            scenario.deleteStatus || 500,
          );
        }
        return noContentResponse();
      }
      return jsonResponse({});
    });
    return { deleteCalls };
  };

  it("fetches the preview when opened and renders impact counts", async () => {
    installFetch();
    render(BatchDeleteModal, {
      props: {
        open: true,
        batchId: "batch_xyz",
      },
    });

    expect(await screen.findByText("5")).toBeInTheDocument();
    expect(screen.getByText("120")).toBeInTheDocument();
    expect(screen.getByText("60")).toBeInTheDocument();
    expect(screen.getByText("batch_xyz")).toBeInTheDocument();
  });

  it("disables the delete button until the typed confirmation matches exactly", async () => {
    installFetch();
    render(BatchDeleteModal, {
      props: {
        open: true,
        batchId: "batch_xyz",
      },
    });

    await screen.findByText("batch_xyz");

    const buttons = screen.getAllByRole("button", { name: /Delete batch/i });
    const confirm = buttons[buttons.length - 1];
    expect(confirm).toBeDisabled();

    const input = await screen.findByLabelText(/Type batch_xyz to confirm/);
    await fireEvent.input(input, { target: { value: "batch_xy" } });
    expect(confirm).toBeDisabled();

    await fireEvent.input(input, { target: { value: "BATCH_XYZ" } });
    expect(confirm).toBeDisabled();

    await fireEvent.input(input, { target: { value: "batch_xyz" } });
    expect(confirm).not.toBeDisabled();
  });

  it("fires DELETE and invokes onDeleted when the typed confirmation matches", async () => {
    const handles = installFetch();
    const onDeleted = vi.fn();
    const onClose = vi.fn();
    render(BatchDeleteModal, {
      props: {
        open: true,
        batchId: "batch_xyz",
        onDeleted,
        onClose,
      },
    });

    const input = await screen.findByLabelText(/Type batch_xyz to confirm/);
    await fireEvent.input(input, { target: { value: "batch_xyz" } });

    const buttons = screen.getAllByRole("button", { name: /Delete batch/i });
    await fireEvent.click(buttons[buttons.length - 1]);

    await waitFor(() => {
      expect(handles.deleteCalls).toEqual([
        expect.stringContaining("/api/v1/batches/batch_xyz"),
      ]);
    });
    expect(onDeleted).toHaveBeenCalledWith("batch_xyz");
    expect(onClose).toHaveBeenCalled();
  });

  it("surfaces preview errors and hides the confirmation input", async () => {
    installFetch({ previewError: "batch not found", previewStatus: 404 });
    render(BatchDeleteModal, {
      props: { open: true, batchId: "missing" },
    });

    expect(await screen.findByText("batch not found")).toBeInTheDocument();
    expect(screen.queryByLabelText(/Type missing to confirm/)).toBeNull();
  });

  it("shows the server error when DELETE fails and keeps the modal open", async () => {
    installFetch({ deleteError: "jobs did not cancel", deleteStatus: 409 });
    const onClose = vi.fn();
    const onDeleted = vi.fn();
    render(BatchDeleteModal, {
      props: {
        open: true,
        batchId: "batch_xyz",
        onClose,
        onDeleted,
      },
    });

    const input = await screen.findByLabelText(/Type batch_xyz to confirm/);
    await fireEvent.input(input, { target: { value: "batch_xyz" } });

    const buttons = screen.getAllByRole("button", { name: /Delete batch/i });
    await fireEvent.click(buttons[buttons.length - 1]);

    expect(await screen.findByText("jobs did not cancel")).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    expect(onDeleted).not.toHaveBeenCalled();
  });

  it("lists affected cohort snapshots and warns when the default is removed", async () => {
    installFetch({
      preview: samplePreview({
        snapshots: [
          {
            cohort_id: 3,
            cohort_label: "Top Level Domains",
            snapshot_slug: "2026-04-20",
            snapshot_label: "April 2026",
            is_default: true,
          },
        ],
      }),
    });
    render(BatchDeleteModal, {
      props: { open: true, batchId: "batch_xyz" },
    });

    expect(await screen.findByText("Top Level Domains")).toBeInTheDocument();
    expect(screen.getByText("2026-04-20")).toBeInTheDocument();
    expect(
      screen.getByText(/Deleting the default snapshot resets the cohort to auto-latest/i),
    ).toBeInTheDocument();
  });

  it("warns when the batch has queued or running jobs", async () => {
    installFetch({
      preview: samplePreview({ queued_jobs: 2, running_jobs: 1 }),
    });
    render(BatchDeleteModal, {
      props: { open: true, batchId: "batch_xyz" },
    });

    expect(
      await screen.findByText(/3 job\(s\) still in flight/i),
    ).toBeInTheDocument();
  });

  it("invokes onClose when the backdrop is clicked", async () => {
    installFetch();
    const onClose = vi.fn();
    const { container } = render(BatchDeleteModal, {
      props: {
        open: true,
        batchId: "batch_xyz",
        onClose,
      },
    });

    await screen.findByText("batch_xyz");
    const backdrop = container.querySelector(".modal-backdrop");
    expect(backdrop).not.toBeNull();
    await fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalled();
  });
});
