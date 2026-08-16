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

    const findTldRow = async () => (await screen.findByText("tld")).closest("tr");

    const rebuildButton = within(await findTldRow()).getByRole("button", { name: /Rebuild/i });
    await fireEvent.click(rebuildButton);
    // Wait for the success notice, which is set only after the POST and the
    // follow-up loadCohorts() both complete; this guarantees busyCohortId has
    // been reset and the row's Clear button is no longer disabled.
    await screen.findByText(/Rebuild triggered for cohort tld/);
    expect(handles.actions).toContainEqual({ id: 1, action: "rebuild" });

    const clearButton = within(await findTldRow()).getByRole("button", { name: /Clear/i });
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
    const handles = installFetch();
    render(AnalysisCohorts);

    await screen.findByText("gov");
    const govRow = screen.getByText("gov").closest("tr");
    await fireEvent.click(within(govRow).getByRole("button", { name: /^Delete$/ }));

    const dialog = await screen.findByRole("dialog");
    await fireEvent.click(within(dialog).getByRole("button", { name: /^Delete$/ }));

    await waitFor(() => {
      expect(screen.queryByText("gov")).toBeNull();
    });
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

  it("opens the run-options menu via the caret and submits promote_snapshot_default", async () => {
    const handles = installSnapshotFetch();
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    // The menu item only exists once the caret opens the menu.
    expect(within(tldRow).queryByRole("menuitem")).toBeNull();

    await fireEvent.click(within(tldRow).getByRole("button", { name: /More run options/i }));
    const menuItem = await within(tldRow).findByRole("menuitem", { name: /Run \+ set as default/i });
    await fireEvent.click(menuItem);

    await waitFor(() => expect(handles.submittedBatches).toHaveLength(1));
    expect(handles.submittedBatches[0]).toEqual({
      from_tag: "tld",
      snapshot_intent: true,
      promote_snapshot_default: true,
    });
  });

  it("closes the run-options menu on Escape", async () => {
    installSnapshotFetch();
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /More run options/i }));
    await within(tldRow).findByRole("menuitem", { name: /Run \+ set as default/i });

    await fireEvent.keyDown(document.body, { key: "Escape" });

    await waitFor(() => expect(within(tldRow).queryByRole("menuitem")).toBeNull());
  });

  it("closes the run-options menu on an outside click", async () => {
    installSnapshotFetch();
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /More run options/i }));
    await within(tldRow).findByRole("menuitem", { name: /Run \+ set as default/i });

    await fireEvent.click(document.body);

    await waitFor(() => expect(within(tldRow).queryByRole("menuitem")).toBeNull());
  });

  // jsdom performs no layout, so the geometry the placement logic reads is faked
  // per element class: the section is the positioning origin the menu's top/left
  // are relative to, the split button is the anchor the menu hangs off, and the
  // menu supplies its own size. Everything else keeps jsdom's all-zero rect.
  const fakeRects = (byClass) => {
    const original = Element.prototype.getBoundingClientRect;
    Element.prototype.getBoundingClientRect = function fake() {
      for (const [className, box] of Object.entries(byClass)) {
        if (this.classList.contains(className)) {
          const { top, bottom, left = 0, right = 0 } = box;
          return {
            top, bottom, left, right,
            height: bottom - top, width: right - left,
            x: left, y: top, toJSON() { return this; },
          };
        }
      }
      return original.call(this);
    };
    return () => { Element.prototype.getBoundingClientRect = original; };
  };

  it("places the run-options menu below its button, relative to the section", async () => {
    installSnapshotFetch();
    // Plenty of room below the anchor within the 768px jsdom viewport.
    const restoreRects = fakeRects({
      "cohort-section": { top: 100, bottom: 400, left: 20, right: 900 },
      "run-split": { top: 160, bottom: 190, left: 700, right: 860 },
      "run-menu": { top: 0, bottom: 40, left: 0, right: 150 },
    });
    try {
      render(AnalysisCohorts);

      const tldRow = (await screen.findByText("tld")).closest("tr");
      await fireEvent.click(within(tldRow).getByRole("button", { name: /More run options/i }));
      await within(tldRow).findByRole("menuitem", { name: /Run \+ set as default/i });

      const menu = tldRow.querySelector(".run-menu");
      // 190 (anchor bottom) + 4 (gap) - 100 (section top), and the menu's right
      // edge lines up with the button's: 860 - 150 (width) - 20 (section left).
      expect(menu.style.top).toBe("94px");
      expect(menu.style.left).toBe("690px");
    } finally {
      restoreRects();
    }
  });

  it("places the run-options menu above its button when the viewport bottom is close", async () => {
    installSnapshotFetch();
    // jsdom's viewport is 768px tall; the anchor sits just above its bottom edge.
    const restoreRects = fakeRects({
      "cohort-section": { top: 100, bottom: 760, left: 20, right: 900 },
      "run-split": { top: 700, bottom: 740, left: 700, right: 860 },
      "run-menu": { top: 0, bottom: 40, left: 0, right: 150 },
    });
    try {
      render(AnalysisCohorts);

      const tldRow = (await screen.findByText("tld")).closest("tr");
      await fireEvent.click(within(tldRow).getByRole("button", { name: /More run options/i }));
      await within(tldRow).findByRole("menuitem", { name: /Run \+ set as default/i });

      const menu = tldRow.querySelector(".run-menu");
      // 700 (anchor top) - 4 (gap) - 40 (menu height) - 100 (section top).
      expect(menu.style.top).toBe("556px");
    } finally {
      restoreRects();
    }
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

  it("links a snapshot's source batch to the batches route", async () => {
    const snapshotsByCohort = {
      1: [{
        id: 100, slug: "2026-04-20", label: "", captured_at: "2026-04-20T12:00:00Z",
        profile_name: "strict", run_count: 3, domain_count: 3, batch_id: "batch_src",
        status: "captured", is_public: true, is_default: false
      }],
    };
    installSnapshotFetch({ snapshotsByCohort });
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /Snapshots/i }));

    const link = await screen.findByText("batch_src");
    expect(link.tagName).toBe("A");
    expect(link.getAttribute("href")).toBe("#/batches/batch_src");
  });

  it("retires a snapshot via POST status=retired after confirming", async () => {
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

    const dialog = await screen.findByRole("dialog");
    await fireEvent.click(within(dialog).getByRole("button", { name: /^Retire$/i }));

    await waitFor(() => expect(handles.snapshotPatches).toHaveLength(1));
    expect(handles.snapshotPatches[0].body).toEqual({ status: "retired", is_public: false });
  });

  it("purges a snapshot via DELETE?purge=true after confirming", async () => {
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
    await fireEvent.click(await screen.findByRole("button", { name: /^Purge snapshot$/i }));

    // Purge is irreversible: the dialog requires typing the snapshot slug.
    const dialog = await screen.findByRole("dialog");
    await fireEvent.input(within(dialog).getByRole("textbox"), { target: { value: "2026-04-20" } });
    await fireEvent.click(within(dialog).getByRole("button", { name: /^Purge snapshot$/i }));

    await waitFor(() => expect(handles.snapshotDeletes).toHaveLength(1));
    expect(handles.snapshotDeletes[0]).toEqual({ id: 1, slug: "2026-04-20", purge: true });
  });

  it("shows a determinate progress bar driven by materialization status and clears it when ready", async () => {
    // Tier-2 flow: the rematerialize POST returns 202 and flips the snapshot to
    // materialization_status "pending"; the admin UI then polls the snapshots
    // list and renders a determinate bar from materialization_done/total. We
    // model the async server with a single mutable snapshot object so the poll
    // loop can observe pending (0/4 -> 2/4) and finally ready, at which point
    // the bar must disappear. The label span is queried directly so the numeric
    // "done/total" text can be asserted independent of node splitting.
    const snap = {
      id: 100, slug: "2026-04-20", label: "", captured_at: "2026-04-20T12:00:00Z",
      profile_name: "strict", run_count: 3, domain_count: 3,
      status: "captured", is_public: true, is_default: false,
      source_runs_available: true,
      materialization_status: "", materialization_done: 0, materialization_total: 0,
    };
    let rematerializeCalls = 0;

    global.fetch.mockImplementation((url, requestOptions = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url);
      const method = requestOptions.method || "GET";
      if (value === "/api/v1/tags?limit=500" && method === "GET") return jsonResponse([]);
      if (value === "/api/v1/analysis/cohorts" && method === "GET") return jsonResponse(sampleCohorts());
      if (value === "/api/v1/analysis/status" && method === "GET") return jsonResponse({ backend_supported: true });
      const listSnaps = value.match(/^\/api\/v1\/analysis\/cohorts\/(\d+)\/snapshots$/);
      if (listSnaps && method === "GET") return jsonResponse([{ ...snap }]);
      const remat = value.match(/^\/api\/v1\/analysis\/cohorts\/(\d+)\/snapshots\/([^/?]+)\/rematerialize$/);
      if (remat && method === "POST") {
        rematerializeCalls += 1;
        // Server accepts (202) and marks the snapshot pending.
        snap.materialization_status = "pending";
        snap.materialization_done = 0;
        snap.materialization_total = 4;
        return jsonResponse({ ...snap });
      }
      return jsonResponse({});
    });

    render(AnalysisCohorts);
    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /Snapshots/i }));

    const snapRow = (await screen.findByText("2026-04-20")).closest("tr");
    await fireEvent.click(within(snapRow).getByRole("button", { name: /Rebuild aggregates/i }));

    expect(rematerializeCalls).toBe(1);
    // The determinate bar and the numeric "0/4" label appear from the pending
    // status returned by the 202 and the follow-up snapshots reload.
    await screen.findByRole("progressbar");
    const label = () => document.querySelector(".snapshot-rebuilding-label");
    expect(label()).toBeTruthy();
    expect(label().textContent).toMatch(/0\/4/);

    // The poll re-fetches the list; advancing done is reflected in the label.
    snap.materialization_done = 2;
    await waitFor(() => expect(label()?.textContent).toMatch(/2\/4/));

    // Completion: the poll observes ready and the bar/label disappears.
    snap.materialization_status = "ready";
    snap.materialization_done = 4;
    await waitFor(() => expect(document.querySelector(".snapshot-rebuilding-label")).toBeNull());
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  const countCohortListGets = () => global.fetch.mock.calls.filter(
    (args) => args[0] === "/api/v1/analysis/cohorts" && (args[1]?.method || "GET") === "GET",
  ).length;

  const pendingCohort = (extra) => ([{
    ...sampleCohorts()[0],
    materialization_status: "pending",
    materialization_done: 0,
    materialization_total: 0,
    last_materialized_at: "",
    ...extra,
  }]);

  it("does not poll a cohort left pending because it was never materialized", async () => {
    // The server also uses "pending" for a cohort that was just created or
    // cleared. Nothing is running in that state, so an open page must not keep
    // requesting the cohort list.
    installSnapshotFetch({ initialCohorts: pendingCohort() });
    render(AnalysisCohorts);
    await screen.findByText("tld");

    const before = countCohortListGets();
    await new Promise((resolve) => setTimeout(resolve, 700));

    expect(countCohortListGets()).toBe(before);
  });

  it("polls while a cohort rebuild reports a work total", async () => {
    installSnapshotFetch({
      initialCohorts: pendingCohort({ materialization_done: 1, materialization_total: 4 }),
    });
    render(AnalysisCohorts);
    await screen.findByText("tld");

    const before = countCohortListGets();
    await waitFor(() => expect(countCohortListGets()).toBeGreaterThan(before));
  });

  it("polls after starting a rebuild, before the server reports a work total", async () => {
    // A rebuild is dispatched server-side and only publishes its total on the
    // first progress write; until then the row looks exactly like an idle one.
    installSnapshotFetch({ initialCohorts: pendingCohort() });
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /Rebuild/i }));
    // Wait for the action's own reload to land, so what follows is the poll.
    await screen.findByText(/Rebuild triggered for cohort tld/i);

    const before = countCohortListGets();
    await waitFor(() => expect(countCohortListGets()).toBeGreaterThan(before + 1));
  });

  it("sorts the snapshot table when a column header is clicked", async () => {
    // Two captured snapshots; default order is newest-captured-first. Clicking
    // the Snapshot header sorts by display name (slug) ascending, and the
    // ascending order here is the reverse of the default captured-desc order,
    // so asserting the first data row's slug proves the click re-sorted.
    const snapshotsByCohort = {
      1: [
        {
          id: 100, slug: "2026-04-20-later", label: "", captured_at: "2026-04-20T12:00:00Z",
          profile_name: "strict", run_count: 3, domain_count: 3,
          status: "captured", is_public: true, is_default: false, source_runs_available: true,
        },
        {
          id: 101, slug: "2026-04-10-earlier", label: "", captured_at: "2026-04-10T12:00:00Z",
          profile_name: "strict", run_count: 1, domain_count: 1,
          status: "captured", is_public: true, is_default: false, source_runs_available: true,
        },
      ],
    };
    installSnapshotFetch({ snapshotsByCohort });
    render(AnalysisCohorts);

    const tldRow = (await screen.findByText("tld")).closest("tr");
    await fireEvent.click(within(tldRow).getByRole("button", { name: /Snapshots/i }));
    await screen.findByText("2026-04-20-later");

    const firstSlug = () =>
      document.querySelector(".snapshot-table tbody tr .snapshot-id")?.textContent?.trim();
    // Default: captured desc -> the April 20 snapshot leads.
    expect(firstSlug()).toBe("2026-04-20-later");

    // Sort by Snapshot name ascending -> the April 10 slug sorts first.
    // Anchored name avoids matching the "Snapshots" expand button.
    await fireEvent.click(screen.getByRole("button", { name: /^Snapshot$/ }));
    await waitFor(() => expect(firstSlug()).toBe("2026-04-10-earlier"));
  });

  // ── Delete-batch action ──────────────────────────────────────────────────

  it("renders a Delete source batch button per snapshot row when onDeleteBatch is provided", async () => {
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

    await screen.findByText("Source batch");
    await screen.findByText("batch_abc");

    const deleteBatchButton = await screen.findByRole("button", { name: /^Delete source batch$/i });
    await fireEvent.click(deleteBatchButton);

    expect(onDeleteBatch).toHaveBeenCalledWith("batch_abc");
  });

  it("hides the Delete source batch button when onDeleteBatch is not provided", async () => {
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

    expect(screen.queryByRole("button", { name: /^Delete source batch$/i })).toBeNull();
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
