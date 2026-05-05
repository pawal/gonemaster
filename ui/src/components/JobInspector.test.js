import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";
import JobInspector from "./JobInspector.svelte";

describe("JobInspector", () => {
  afterEach(() => cleanup());

  it("renders the job inspector heading when no job is selected", () => {
    render(JobInspector, {
      props: { selectedJob: null, selectedJobResult: null, selectedRun: null },
    });
    expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent(/Job Inspector/i);
  });

  it("renders the run inspector heading when the job is in a result-ready state", () => {
    render(JobInspector, {
      props: {
        selectedJob: { id: "j1", status: "succeeded", domain: "example.com", created_at: "2026-01-01T00:00:00Z" },
        selectedJobResult: null,
        selectedRun: null,
      },
    });
    expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent(/Run Inspector/i);
  });

  it("invokes onRefresh when the Refresh button is clicked", async () => {
    const onRefresh = vi.fn();
    render(JobInspector, { props: { onRefresh } });
    await fireEvent.click(screen.getByRole("button", { name: /^Refresh$/i }));
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });

  it("shows 'Load result' when the job is ready but no result has loaded", () => {
    render(JobInspector, {
      props: {
        selectedJob: { id: "j1", status: "succeeded", domain: "example.com", created_at: "2026-01-01T00:00:00Z" },
        selectedJobResult: null,
      },
    });
    expect(screen.getByRole("button", { name: /Load result/i })).toBeInTheDocument();
  });

  it("invokes onLoadResult when the Load result button is clicked", async () => {
    const onLoadResult = vi.fn();
    render(JobInspector, {
      props: {
        selectedJob: { id: "j1", status: "succeeded", domain: "example.com", created_at: "2026-01-01T00:00:00Z" },
        selectedJobResult: null,
        onLoadResult,
      },
    });
    await fireEvent.click(screen.getByRole("button", { name: /Load result/i }));
    expect(onLoadResult).toHaveBeenCalledTimes(1);
  });

  it("invokes onNavigateDomain with the domain name when the domain link is clicked", async () => {
    const onNavigateDomain = vi.fn();
    render(JobInspector, {
      props: {
        selectedJob: { id: "j1", status: "running", domain: "example.com", created_at: "2026-01-01T00:00:00Z" },
        onNavigateDomain,
      },
    });
    await fireEvent.click(screen.getByRole("button", { name: "example.com" }));
    expect(onNavigateDomain).toHaveBeenCalledWith("example.com");
  });

  it("renders the result summary and module list when a result is provided", () => {
    const selectedJobResult = {
      job_id: "j1",
      summary: { levels: { NOTICE: 0, WARNING: 1, ERROR: 0, CRITICAL: 0 } },
      raw: {
        entries: [
          { module: "DNSSEC", level: "WARNING", testcase: "dnssec01", message: "warn-msg" },
        ],
      },
    };
    render(JobInspector, {
      props: {
        selectedJob: { id: "j1", status: "succeeded", domain: "example.com", created_at: "2026-01-01T00:00:00Z" },
        selectedJobResult,
      },
    });
    expect(screen.getByText("WARNING")).toBeInTheDocument();
    expect(screen.getByText("DNSSEC")).toBeInTheDocument();
  });

  it("expands a module when its toggle is clicked, revealing its entries", async () => {
    const selectedJobResult = {
      job_id: "j1",
      summary: { levels: { NOTICE: 0, WARNING: 1, ERROR: 0, CRITICAL: 0 } },
      raw: {
        entries: [
          { module: "DNSSEC", level: "WARNING", testcase: "dnssec01", message: "warn-msg" },
        ],
      },
    };
    render(JobInspector, {
      props: {
        selectedJob: { id: "j1", status: "succeeded", domain: "example.com", created_at: "2026-01-01T00:00:00Z" },
        selectedJobResult,
      },
    });
    expect(screen.queryByText("warn-msg")).toBeNull();
    await fireEvent.click(screen.getByRole("button", { expanded: false, name: /DNSSEC/i }));
    expect(screen.getByText("warn-msg")).toBeInTheDocument();
  });

  it("resolves a profile name from availableProfiles when only profile_id is set on the job", () => {
    render(JobInspector, {
      props: {
        selectedJob: { id: "j1", status: "running", domain: "example.com", created_at: "2026-01-01T00:00:00Z", profile_id: 42 },
        availableProfiles: [{ id: 42, name: "strict" }],
      },
    });
    expect(screen.getByText("strict")).toBeInTheDocument();
  });

  it("falls back to '#<id>' when the profile_id is not in availableProfiles", () => {
    render(JobInspector, {
      props: {
        selectedJob: { id: "j1", status: "running", domain: "example.com", created_at: "2026-01-01T00:00:00Z", profile_id: 99 },
        availableProfiles: [],
      },
    });
    expect(screen.getByText("#99")).toBeInTheDocument();
  });
});
