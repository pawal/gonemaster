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
    const patched = [];
    const created = [];
    const actions = [];
    global.fetch.mockImplementation((url, requestOptions = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url);
      const method = requestOptions.method || "GET";

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

    expect(await screen.findByRole("cell", { name: "TLD" })).toBeInTheDocument();
    expect(await screen.findByRole("cell", { name: "Government" })).toBeInTheDocument();

    expect(screen.getByText("default")).toBeInTheDocument();
    expect(screen.getByText("ready")).toBeInTheDocument();
    expect(screen.getByText("failed")).toBeInTheDocument();
    expect(screen.getByText("boom happened")).toBeInTheDocument();
  });

  it("patches analysis_enabled when clicking the analysis toggle and clears dependent flags on disable", async () => {
    const handles = installFetch();
    render(AnalysisCohorts);

    const tldRow = (await screen.findByRole("cell", { name: "TLD" })).closest("tr");
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

    const govRow = (await screen.findByRole("cell", { name: "Government" })).closest("tr");
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

    const tldRow = (await screen.findByRole("cell", { name: "TLD" })).closest("tr");
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

    await screen.findByRole("cell", { name: "TLD" });

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

  it("blocks creation when source_tag is empty and keeps the form local", async () => {
    const handles = installFetch();
    render(AnalysisCohorts);

    await screen.findByRole("cell", { name: "TLD" });

    const createButton = screen.getByRole("button", { name: /Create cohort/i });
    await fireEvent.click(createButton);

    expect(handles.created).toHaveLength(0);
    expect(screen.getByText(/source_tag is required/i)).toBeInTheDocument();
  });
});
