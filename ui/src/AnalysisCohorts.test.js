import { render, screen, fireEvent, waitFor, within, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import AnalysisCohorts from "./AnalysisCohorts.svelte";

const jsonResponse = (data, ok = true) => ({
  ok,
  statusText: ok ? "OK" : "Bad Request",
  headers: { get: () => "application/json" },
  json: async () => data,
  text: async () => JSON.stringify(data),
});

describe("AnalysisCohorts", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    global.fetch = vi.fn();
  });

  afterEach(() => {
    cleanup();
  });

  const sampleCohorts = () => ([
    {
      id: 1,
      source_type: "tag",
      source_tag: "tld",
      label: "TLD",
      description: "Top-level domains",
      analysis_enabled: true,
      public_enabled: true,
      is_default: true,
      sort_order: 10,
      materialization_status: "ready",
      last_materialized_at: "2026-04-17T12:00:00Z",
      last_materialization_error: "",
    },
    {
      id: 2,
      source_type: "tag",
      source_tag: "gov",
      label: "Government",
      description: "",
      analysis_enabled: false,
      public_enabled: false,
      is_default: false,
      sort_order: 20,
      materialization_status: "failed",
      last_materialized_at: "",
      last_materialization_error: "boom happened",
    },
  ]);

  const installFetch = (scenario = {}) => {
    let cohorts = scenario.initialCohorts || sampleCohorts();
    const existingTags = scenario.existingTags || [];
    const patched = [];
    const created = [];
    const actions = [];
    global.fetch.mockImplementation((url, requestOptions = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url);
      const method = requestOptions.method || "GET";

      if (value === "/api/v1/tags?limit=500" && method === "GET") {
        return jsonResponse(existingTags.map((name) => ({ name })));
      }
      if (value === "/api/v1/analysis/cohorts" && method === "GET") {
        return jsonResponse(cohorts);
      }
      if (value === "/api/v1/analysis/cohorts" && method === "POST") {
        const body = JSON.parse(requestOptions.body);
        created.push(body);
        if (scenario.createError) {
          return jsonResponse({ error: { message: scenario.createError } }, false);
        }
        const next = {
          id: cohorts.length + 1,
          source_type: "tag",
          sort_order: 0,
          materialization_status: "pending",
          last_materialized_at: "",
          last_materialization_error: "",
          ...body,
        };
        cohorts = [...cohorts, next];
        return jsonResponse(next);
      }
      const patchMatch = value.match(/^\/api\/v1\/analysis\/cohorts\/(\d+)$/);
      if (patchMatch && method === "PATCH") {
        const id = Number(patchMatch[1]);
        const body = JSON.parse(requestOptions.body);
        patched.push({ id, body });
        cohorts = cohorts.map((c) => (c.id === id ? { ...c, ...body } : c));
        return jsonResponse(cohorts.find((c) => c.id === id));
      }
      const deleteMatch = value.match(/^\/api\/v1\/analysis\/cohorts\/(\d+)$/);
      if (deleteMatch && method === "DELETE") {
        const id = Number(deleteMatch[1]);
        cohorts = cohorts.filter((c) => c.id !== id);
        return { ok: true, statusText: "No Content", headers: { get: () => "" }, json: async () => ({}), text: async () => "" };
      }
      const actionMatch = value.match(/^\/api\/v1\/analysis\/cohorts\/(\d+)\/(rebuild|clear)$/);
      if (actionMatch && method === "POST") {
        actions.push({ id: Number(actionMatch[1]), action: actionMatch[2] });
        if (actionMatch[2] === "clear") {
          cohorts = cohorts.map((c) =>
            c.id === Number(actionMatch[1])
              ? { ...c, materialization_status: "pending", last_materialized_at: "" }
              : c
          );
        } else {
          cohorts = cohorts.map((c) =>
            c.id === Number(actionMatch[1])
              ? {
                  ...c,
                  materialization_status: "ready",
                  last_materialized_at: "2026-04-17T13:00:00Z",
                }
              : c
          );
        }
        return jsonResponse(cohorts.find((c) => c.id === Number(actionMatch[1])));
      }
      return jsonResponse({});
    });
    return { created, patched, actions, getCohorts: () => cohorts };
  };

  it("lists cohorts with status, default marker, and last-error detail", async () => {
    installFetch();
    render(AnalysisCohorts);

    expect(await screen.findByText("tld")).toBeInTheDocument();
    expect(await screen.findByText("gov")).toBeInTheDocument();

    expect(screen.getByText("default")).toBeInTheDocument();
    expect(screen.getByText("ready")).toBeInTheDocument();
    expect(screen.getByText("failed")).toBeInTheDocument();
    expect(screen.getByText("boom happened")).toBeInTheDocument();
  });

  it("labels a ready cohort without rows as never materialized", async () => {
    installFetch({
      initialCohorts: [{
        ...sampleCohorts()[0],
        materialization_status: "ready",
        last_materialized_at: "",
      }],
    });
    render(AnalysisCohorts);

    const row = (await screen.findByText("tld")).closest("tr");
    expect(within(row).getByText("never materialized")).toBeInTheDocument();
    expect(within(row).queryByText("ready")).toBeNull();
  });

  it("patches analysis_enabled when clicking the analysis toggle and clears dependent flags on disable", async () => {
    const handles = installFetch();
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    const toggles = within(tldRow).getAllByRole("button", { name: /^on$/ });
    await fireEvent.click(toggles[0]);

    await waitFor(() => expect(handles.patched).toHaveLength(1));
    expect(handles.patched[0]).toEqual({
      id: 1,
      body: { analysis_enabled: false, public_enabled: false, is_default: false },
    });
  });

  it("sends is_default:true when marking a non-default cohort as default after enabling analysis and public", async () => {
    const govWithPublic = sampleCohorts().map((c) =>
      c.source_tag === "gov"
        ? { ...c, analysis_enabled: true, public_enabled: true, materialization_status: "ready" }
        : c
    );
    const handles = installFetch({ initialCohorts: govWithPublic });
    render(AnalysisCohorts);

    const govRow = (await screen.findByText("gov")).closest("tr");
    const makeDefaultButton = within(govRow).getByRole("button", { name: /Make default/i });
    await fireEvent.click(makeDefaultButton);

    await waitFor(() => expect(handles.patched).toHaveLength(1));
    expect(handles.patched[0]).toEqual({
      id: 2,
      body: { is_default: true },
    });
  });

  it("triggers rebuild and clear actions for a cohort", async () => {
    const handles = installFetch();
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    const rebuildButton = within(tldRow).getByRole("button", { name: /Rebuild/i });
    await fireEvent.click(rebuildButton);
    await waitFor(() => expect(handles.actions).toContainEqual({ id: 1, action: "rebuild" }));

    const clearButton = within(tldRow).getByRole("button", { name: /Clear/i });
    await fireEvent.click(clearButton);
    await waitFor(() => expect(handles.actions).toContainEqual({ id: 1, action: "clear" }));
  });

  it("creates a new cohort when required fields are provided", async () => {
    const handles = installFetch();
    render(AnalysisCohorts);

    await screen.findByText("tld");

    const tagInput = screen.getByPlaceholderText("tld");
    await fireEvent.input(tagInput, { target: { value: "anycast" } });

    const createButton = screen.getByRole("button", { name: /Create cohort/i });
    await fireEvent.click(createButton);

    await waitFor(() => expect(handles.created).toHaveLength(1));
    expect(handles.created[0]).toMatchObject({
      source_type: "tag",
      source_tag: "anycast",
      analysis_enabled: true,
      public_enabled: false,
      is_default: false,
    });
  });

  it("enters edit mode, PATCHes label and description, and exits", async () => {
    const handles = installFetch();
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    const editButton = within(tldRow).getByRole("button", { name: /^Edit$/ });
    await fireEvent.click(editButton);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /Edit cohort/i })).toBeInTheDocument();
    });

    const labelInput = screen.getByDisplayValue("TLD");
    await fireEvent.input(labelInput, { target: { value: "Top-Level Domains" } });
    const descInput = screen.getByDisplayValue("Top-level domains");
    await fireEvent.input(descInput, { target: { value: "All ICANN-managed TLDs" } });

    await fireEvent.click(screen.getByRole("button", { name: /^Save$/ }));

    await waitFor(() => expect(handles.patched).toHaveLength(1));
    expect(handles.patched[0]).toEqual({
      id: 1,
      body: {
        label: "Top-Level Domains",
        description: "All ICANN-managed TLDs",
        sort_order: 10,
      },
    });
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /^Create cohort$/i })).toBeInTheDocument();
    });
  });

  it("deletes a cohort after confirming and removes it from the table", async () => {
    global.confirm = vi.fn(() => true);
    const handles = installFetch();
    render(AnalysisCohorts);

    await screen.findByText("gov");
    const govRow = screen.getByText("gov").closest("tr");
    const deleteButton = within(govRow).getByRole("button", { name: /^Delete$/ });
    await fireEvent.click(deleteButton);

    await waitFor(() => {
      expect(screen.queryByText("gov")).toBeNull();
    });
    expect(global.confirm).toHaveBeenCalled();
    expect(handles.getCohorts().some((c) => c.source_tag === "gov")).toBe(false);
  });

  it("skips deletion when the confirm dialog is cancelled", async () => {
    global.confirm = vi.fn(() => false);
    installFetch();
    render(AnalysisCohorts);

    const govRow = (await screen.findByText("gov")).closest("tr");
    await fireEvent.click(within(govRow).getByRole("button", { name: /^Delete$/ }));

    expect(await screen.findByText("gov")).toBeInTheDocument();
  });

  it("shows an existing-tag indicator when the input matches a known tag", async () => {
    installFetch({ existingTags: ["tld", "gov"] });
    render(AnalysisCohorts);

    await screen.findByText("tld");

    const tagInput = screen.getByPlaceholderText("tld");
    await fireEvent.input(tagInput, { target: { value: "anycast" } });
    expect(screen.getByText(/will be created/i)).toBeInTheDocument();

    await fireEvent.input(tagInput, { target: { value: "tld" } });
    await waitFor(() => expect(screen.getByText(/existing tag/i)).toBeInTheDocument());
    expect(screen.queryByText(/will be created/i)).toBeNull();
  });

  it("blocks creation when source_tag is empty and keeps the form local", async () => {
    const handles = installFetch();
    render(AnalysisCohorts);

    await screen.findByText("tld");

    const createButton = screen.getByRole("button", { name: /Create cohort/i });
    await fireEvent.click(createButton);

    expect(handles.created).toHaveLength(0);
    expect(screen.getByText(/source_tag is required/i)).toBeInTheDocument();
  });

  // ── Snapshot sub-panel (Phase 6) ─────────────────────────────────────────

  const installSnapshotFetch = (scenario = {}) => {
    let cohorts = scenario.initialCohorts || sampleCohorts();
    let snapshotsByCohort = scenario.snapshotsByCohort || {};
    const submittedBatches = [];
    const snapshotPatches = [];
    const snapshotDeletes = [];
    global.fetch.mockImplementation((url, requestOptions = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url);
      const method = requestOptions.method || "GET";
      if (value === "/api/v1/tags?limit=500" && method === "GET") return jsonResponse([]);
      if (value === "/api/v1/analysis/cohorts" && method === "GET") return jsonResponse(cohorts);
      if (value === "/api/v1/analysis/status" && method === "GET") {
        return jsonResponse({ backend_supported: true });
      }
      const listSnaps = value.match(/^\/api\/v1\/analysis\/cohorts\/(\d+)\/snapshots$/);
      if (listSnaps && method === "GET") {
        return jsonResponse(snapshotsByCohort[Number(listSnaps[1])] || []);
      }
      const snapPatch = value.match(/^\/api\/v1\/analysis\/cohorts\/(\d+)\/snapshots\/([^/?]+)$/);
      if (snapPatch && method === "POST") {
        const id = Number(snapPatch[1]);
        const slug = decodeURIComponent(snapPatch[2]);
        snapshotPatches.push({ id, slug, body: JSON.parse(requestOptions.body) });
        snapshotsByCohort[id] = (snapshotsByCohort[id] || []).map((s) =>
          s.slug === slug ? { ...s, ...JSON.parse(requestOptions.body) } : s
        );
        return jsonResponse(snapshotsByCohort[id].find((s) => s.slug === slug));
      }
      const snapDelete = value.match(/^\/api\/v1\/analysis\/cohorts\/(\d+)\/snapshots\/([^/?]+)/);
      if (snapDelete && method === "DELETE") {
        const id = Number(snapDelete[1]);
        const slug = decodeURIComponent(snapDelete[2]);
        const purge = value.includes("purge=true");
        snapshotDeletes.push({ id, slug, purge });
        snapshotsByCohort[id] = (snapshotsByCohort[id] || []).filter((s) => s.slug !== slug);
        return {
          ok: true, statusText: "No Content", headers: { get: () => "" },
          json: async () => ({}), text: async () => ""
        };
      }
      if (value === "/api/v1/jobs/batch" && method === "POST") {
        submittedBatches.push(JSON.parse(requestOptions.body));
        return jsonResponse({ batch_id: "batch-new", job_ids: [] });
      }
      return jsonResponse({});
    });
    return {
      submittedBatches,
      snapshotPatches,
      snapshotDeletes,
      getSnapshots: (id) => snapshotsByCohort[id] || [],
    };
  };

  it("submits a snapshot-intent batch when Run new snapshot is clicked", async () => {
    const handles = installSnapshotFetch();
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /Run new snapshot/i }));

    await waitFor(() => expect(handles.submittedBatches).toHaveLength(1));
    expect(handles.submittedBatches[0]).toEqual({
      from_tag: "tld",
      snapshot_intent: true,
    });
  });

  it("lists snapshots with a mixed-profile banner when one is flagged", async () => {
    const snapshotsByCohort = {
      1: [
        {
          id: 100, slug: "2026-04-20-a", label: "", captured_at: "2026-04-20T12:00:00Z",
          profile_name: "strict", run_count: 3, domain_count: 3,
          status: "captured", is_public: true, is_default: false
        },
        {
          id: 101, slug: "2026-04-10-mixed", label: "", captured_at: "",
          profile_name: "", run_count: 0, domain_count: 0,
          status: "failed_mixed_profiles", is_public: false, is_default: false
        },
      ],
    };
    installSnapshotFetch({ snapshotsByCohort });
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /Snapshots/i }));

    expect(await screen.findByText("2026-04-20-a")).toBeInTheDocument();
    expect(screen.getByText("2026-04-10-mixed")).toBeInTheDocument();
    expect(screen.getByText(/mixed profiles/i)).toBeInTheDocument();
  });

  it("sets a snapshot as default via POST is_default=true", async () => {
    const snapshotsByCohort = {
      1: [{
        id: 100, slug: "2026-04-20", label: "", captured_at: "2026-04-20T12:00:00Z",
        profile_name: "strict", run_count: 3, domain_count: 3,
        status: "captured", is_public: true, is_default: false
      }],
    };
    const handles = installSnapshotFetch({ snapshotsByCohort });
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /Snapshots/i }));

    const snapRow = (await screen.findByText("2026-04-20")).closest("tr");
    await fireEvent.click(within(snapRow).getByRole("button", { name: /Make default/i }));
    await waitFor(() => expect(handles.snapshotPatches).toHaveLength(1));
    expect(handles.snapshotPatches[0]).toMatchObject({
      id: 1, slug: "2026-04-20", body: { is_default: true }
    });
  });

  it("retires a snapshot via POST status=retired after confirming", async () => {
    global.confirm = vi.fn(() => true);
    const snapshotsByCohort = {
      1: [{
        id: 100, slug: "2026-04-20", label: "", captured_at: "2026-04-20T12:00:00Z",
        profile_name: "strict", run_count: 3, domain_count: 3,
        status: "captured", is_public: true, is_default: false
      }],
    };
    const handles = installSnapshotFetch({ snapshotsByCohort });
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /Snapshots/i }));
    await fireEvent.click(await screen.findByRole("button", { name: /^Retire$/i }));

    await waitFor(() => expect(handles.snapshotPatches).toHaveLength(1));
    expect(handles.snapshotPatches[0].body).toEqual({ status: "retired", is_public: false });
  });

  it("purges a snapshot via DELETE?purge=true after confirming", async () => {
    global.confirm = vi.fn(() => true);
    const snapshotsByCohort = {
      1: [{
        id: 100, slug: "2026-04-20", label: "", captured_at: "2026-04-20T12:00:00Z",
        profile_name: "strict", run_count: 3, domain_count: 3,
        status: "captured", is_public: true, is_default: false
      }],
    };
    const handles = installSnapshotFetch({ snapshotsByCohort });
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /Snapshots/i }));
    await fireEvent.click(await screen.findByRole("button", { name: /^Purge$/i }));

    await waitFor(() => expect(handles.snapshotDeletes).toHaveLength(1));
    expect(handles.snapshotDeletes[0]).toEqual({ id: 1, slug: "2026-04-20", purge: true });
  });

  // ── Delete-batch action ──────────────────────────────────────────────────

  it("renders a Delete batch button per snapshot row when onDeleteBatch is provided", async () => {
    const snapshotsByCohort = {
      1: [{
        id: 100, batch_id: "batch_abc", slug: "2026-04-20", label: "",
        captured_at: "2026-04-20T12:00:00Z", profile_name: "strict",
        run_count: 3, domain_count: 3, status: "captured",
        is_public: true, is_default: false,
      }],
    };
    installSnapshotFetch({ snapshotsByCohort });
    const onDeleteBatch = vi.fn();
    render(AnalysisCohorts, { props: { onDeleteBatch } });

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /Snapshots/i }));

    const deleteBatchButton = await screen.findByRole("button", { name: /^Delete batch$/i });
    await fireEvent.click(deleteBatchButton);

    expect(onDeleteBatch).toHaveBeenCalledWith("batch_abc");
  });

  it("hides the Delete batch button when onDeleteBatch is not provided", async () => {
    const snapshotsByCohort = {
      1: [{
        id: 100, batch_id: "batch_abc", slug: "2026-04-20", label: "",
        captured_at: "2026-04-20T12:00:00Z", profile_name: "strict",
        run_count: 3, domain_count: 3, status: "captured",
        is_public: true, is_default: false,
      }],
    };
    installSnapshotFetch({ snapshotsByCohort });
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /Snapshots/i }));
    await screen.findByText("2026-04-20");

    expect(screen.queryByRole("button", { name: /^Delete batch$/i })).toBeNull();
  });

  it("refetches snapshots for expanded cohorts when refreshSignal changes", async () => {
    const snapshotsByCohort = {
      1: [{
        id: 100, batch_id: "batch_abc", slug: "2026-04-20", label: "",
        captured_at: "2026-04-20T12:00:00Z", profile_name: "strict",
        run_count: 3, domain_count: 3, status: "captured",
        is_public: true, is_default: false,
      }],
    };
    installSnapshotFetch({ snapshotsByCohort });
    const { rerender } = render(AnalysisCohorts, { props: { refreshSignal: 0 } });

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /Snapshots/i }));
    await screen.findByText("2026-04-20");

    const beforeCalls = global.fetch.mock.calls.filter(
      (args) => typeof args[0] === "string" && args[0].endsWith("/snapshots"),
    ).length;

    await rerender({ refreshSignal: 1 });

    await waitFor(() => {
      const afterCalls = global.fetch.mock.calls.filter(
        (args) => typeof args[0] === "string" && args[0].endsWith("/snapshots"),
      ).length;
      expect(afterCalls).toBeGreaterThan(beforeCalls);
    });
  });
});
