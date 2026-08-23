import { render, screen, fireEvent, waitFor, within } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProfileSettings from "./ProfileSettings.svelte";
import { emptyResponse, jsonResponse, profileFixture, requestUrl } from "../test/helpers.js";

describe("ProfileSettings", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
    global.confirm = vi.fn(() => true);
  });

  const findLibraryRow = async (name) => {
    const rows = await screen.findAllByRole("listitem");
    return rows.find((row) => within(row).queryByText(name));
  };

  const sampleDefaultProfile = () => ({
    id: 0,
    name: "default",
    description: "Server base profile",
    config: {
      net: { ipv4: true, ipv6: false },
      resolver: { defaults: { timeout: 7 } }
    },
    public: false,
    created_at: "0001-01-01T00:00:00Z",
    updated_at: "0001-01-01T00:00:00Z"
  });

  const sampleProfiles = () => [
    profileFixture({ id: 1, name: "alpha", description: "Public baseline", config: { net: { ipv4: true } }, public: true }),
    profileFixture({
      id: 2,
      name: "beta",
      description: "Strict resolver profile",
      config: { resolver: { defaults: { timeout: 5 } } },
      created_at: "2026-04-01T11:00:00Z",
      updated_at: "2026-04-03T09:30:00Z"
    })
  ];

  const sampleTags = () => ([
    { name: "ops", default_profile_id: 2 },
    { name: "prod", default_profile_id: 2 }
  ]);

  const compatResultCompatible = () => ({
    compatible: true,
    reviewed: false,
    schema_version: "v1.0.0",
    current_version: "v1.0.0",
    issues: []
  });

  const compatResultIncompatible = () => ({
    compatible: false,
    reviewed: false,
    schema_version: "v0.9.0",
    current_version: "v1.0.0",
    issues: [
      {
        type: "missing_test_case",
        detail: "Profile sets test_cases but is missing: zone14",
        suggestion: "Add zone14 to test_cases, or remove test_cases to inherit all defaults."
      }
    ]
  });

  // A profile stamped with the current engine version: compatible, but the
  // review is waiving a real gap.
  const compatResultReviewed = () => ({
    compatible: true,
    reviewed: true,
    schema_version: "v1.0.0",
    current_version: "v1.0.0",
    issues: [],
    waived_issues: [
      {
        type: "missing_test_case",
        detail: "Profile sets test_cases but is missing: zone15",
        suggestion: "Add zone15 to test_cases, or remove test_cases to inherit all defaults."
      }
    ]
  });

  const sampleCompatSummaries = (incompatibleId = null, waivedId = null) => sampleProfiles().map((p) => ({
    id: p.id,
    name: p.name,
    compatible: p.id !== incompatibleId,
    issue_count: p.id === incompatibleId ? 1 : 0,
    waived_count: p.id === waivedId ? 3 : 0
  }));

  const sampleDiff = () => ({
    summary: { deviations: 2, redundant: 2, missing: 1, unknown: 0 },
    properties: [
      {
        path: "resolver.defaults.nameserver_max_total_ms",
        kind: "changed",
        redundant: false,
        default: 0,
        value: 60000
      },
      { path: "badkeys.path", kind: "redundant", redundant: true },
      {
        path: "test_levels",
        kind: "map",
        redundant: false,
        wholesale: true,
        modules: [
          {
            module: "CONSISTENCY",
            changed: [{ key: "EXTRA_ADDRESS_CHILD", default: "NOTICE", value: "WARNING" }],
            missing: ["MISSING_ADDRESS_CHILD"],
            unknown: []
          }
        ]
      },
      { path: "test_cases_vars", kind: "map", redundant: true, wholesale: false, modules: [] }
    ]
  });

  const installProfileFetch = (scenario = {}) => {
    let profiles = sampleProfiles();
    let compatSummaries = sampleCompatSummaries(
      scenario.incompatibleProfileId || null,
      scenario.waivedProfileId || null
    );
    const tags = sampleTags();
    const createdBodies = [];
    const updatedBodies = [];
    const patchedBodies = [];
    const diffBodies = [];
    let markAllReviewedCalls = 0;
    const reviewedIds = new Set(scenario.reviewedProfileId ? [scenario.reviewedProfileId] : []);
    const clearedIds = new Set();

    global.fetch.mockImplementation((url, requestOptions = {}) => {
      const value = requestUrl(url);
      const method = requestOptions.method || "GET";

      if (value === "/api/v1/profiles/default") return jsonResponse(sampleDefaultProfile());
      if (value === "/api/v1/profiles" && method === "GET") return jsonResponse(profiles);
      if (value === "/api/v1/profiles" && method === "POST") {
        const body = JSON.parse(requestOptions.body);
        createdBodies.push(body);
        if (scenario.createError) {
          return jsonResponse({ error: { message: scenario.createError } }, false);
        }
        const created = {
          id: 3,
          ...body,
          created_at: "2026-04-04T09:00:00Z",
          updated_at: "2026-04-04T09:00:00Z"
        };
        profiles = [...profiles, created];
        return jsonResponse(created);
      }
      if (value === "/api/v1/profiles/2" && method === "PUT") {
        const body = JSON.parse(requestOptions.body);
        updatedBodies.push(body);
        if (scenario.updateError) {
          return jsonResponse({ error: { message: scenario.updateError } }, false);
        }
        const updated = {
          id: 2,
          ...body,
          created_at: "2026-04-01T11:00:00Z",
          updated_at: "2026-04-05T09:30:00Z"
        };
        profiles = profiles.map((profile) => profile.id === 2 ? updated : profile);
        return jsonResponse(updated);
      }
      if (value === "/api/v1/profiles/2" && method === "PATCH") {
        const body = JSON.parse(requestOptions.body);
        patchedBodies.push(body);
        if (scenario.patchError) {
          return jsonResponse({ error: { message: scenario.patchError } }, false);
        }
        if (body.op === "clear_reviewed") {
          // The stamp is gone, so the waived issues come back as real ones.
          reviewedIds.delete(2);
          clearedIds.add(2);
          const cleared = { ...profiles.find((p) => p.id === 2), schema_version: "" };
          profiles = profiles.map((p) => p.id === 2 ? cleared : p);
          compatSummaries = sampleCompatSummaries(2);
          return jsonResponse(cleared);
        }
        const patched = { ...profiles.find((p) => p.id === 2), schema_version: "v1.0.0" };
        profiles = profiles.map((p) => p.id === 2 ? patched : p);
        compatSummaries = sampleCompatSummaries(null);
        return jsonResponse(patched);
      }
      if (value === "/api/v1/profiles/diff" && method === "POST") {
        diffBodies.push(JSON.parse(requestOptions.body));
        if (scenario.diffError) {
          return jsonResponse({ error: { message: scenario.diffError } }, false);
        }
        return jsonResponse(scenario.diff || sampleDiff());
      }
      if (value === "/api/v1/profiles/3" && method === "DELETE") {
        profiles = profiles.filter((profile) => profile.id !== 3);
        return emptyResponse();
      }
      if (value === "/api/v1/profiles/compatibility" && method === "GET") {
        return jsonResponse(compatSummaries);
      }
      if (value === "/api/v1/profiles/mark-all-reviewed" && method === "POST") {
        markAllReviewedCalls++;
        if (scenario.markAllError) {
          return jsonResponse({ error: { message: scenario.markAllError } }, false);
        }
        const updated = compatSummaries.filter((s) => !s.compatible).length;
        compatSummaries = sampleCompatSummaries(null);
        return jsonResponse({ updated });
      }
      if (/\/api\/v1\/profiles\/\d+\/compatibility$/.test(value)) {
        const id = parseInt(value.match(/\/profiles\/(\d+)\//)?.[1]);
        if (clearedIds.has(id)) return jsonResponse(compatResultIncompatible());
        if (reviewedIds.has(id)) return jsonResponse(compatResultReviewed());
        if (id === scenario.incompatibleProfileId) return jsonResponse(compatResultIncompatible());
        return jsonResponse(compatResultCompatible());
      }
      if (value === "/api/v1/tags?limit=500") return jsonResponse(tags);
      return jsonResponse({});
    });

    return {
      createdBodies,
      updatedBodies,
      patchedBodies,
      diffBodies,
      getMarkAllReviewedCalls: () => markAllReviewedCalls,
      getProfiles: () => profiles
    };
  };

  it("loads the server default and stored profiles into the library", async () => {
    installProfileFetch();

    render(ProfileSettings);

    const defaultRow = await findLibraryRow("default");
    const alphaRow = await findLibraryRow("alpha");
    const betaRow = await findLibraryRow("beta");
    expect(defaultRow).toBeTruthy();
    expect(alphaRow).toBeTruthy();
    expect(betaRow).toBeTruthy();
    expect(within(defaultRow).getByText("Default")).toBeInTheDocument();
    expect(within(defaultRow).getByText("Server base profile")).toBeInTheDocument();
    expect(screen.getByText("In use · 2")).toBeInTheDocument();
    expect(within(alphaRow).getByText("Public")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "default" })).toBeInTheDocument();
  });

  it("seeds new drafts from the server default profile", async () => {
    installProfileFetch();

    render(ProfileSettings);
    await findLibraryRow("default");

    await fireEvent.click(screen.getByRole("button", { name: "New profile" }));

    expect(screen.getByRole("heading", { name: "New profile draft" })).toBeInTheDocument();
    expect(screen.getByText("Seeded from default")).toBeInTheDocument();
    expect(screen.getByLabelText("Config JSON").value).toContain('"timeout": 7');
    expect(screen.getByLabelText("Config JSON").value).toContain('"ipv6": false');
  });

  it("updates an existing profile from the editor", async () => {
    const state = installProfileFetch();

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    await fireEvent.input(screen.getByLabelText("Description"), {
      target: { value: "Updated strict resolver profile" }
    });
    await fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(state.updatedBodies).toHaveLength(1);
    });
    expect(state.updatedBodies[0]).toEqual({
      name: "beta",
      description: "Updated strict resolver profile",
      public: false,
      config: { resolver: { defaults: { timeout: 5 } } }
    });
    expect(await screen.findByText("Profile saved.")).toBeInTheDocument();
    expect(state.getProfiles().find((profile) => profile.id === 2)?.description).toBe("Updated strict resolver profile");
  });

  it("shows the server error message when saving fails", async () => {
    installProfileFetch({ createError: "profile name already exists" });

    render(ProfileSettings);

    await findLibraryRow("default");
    await fireEvent.click(screen.getByRole("button", { name: "New profile" }));
    await fireEvent.input(screen.getByLabelText("Name"), { target: { value: "gamma" } });
    await fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Failed to save profile: profile name already exists")).toBeInTheDocument();
  });

  it("creates a new profile from default and allows deleting it afterwards", async () => {
    const state = installProfileFetch();
    const { container } = render(ProfileSettings);

    await findLibraryRow("default");
    await fireEvent.click(screen.getByRole("button", { name: "New profile" }));
    await fireEvent.input(screen.getByLabelText("Name"), { target: { value: "gamma" } });
    await fireEvent.input(screen.getByLabelText("Description"), { target: { value: "Created from default" } });
    await fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(state.createdBodies).toHaveLength(1);
    });
    expect(state.createdBodies[0]).toEqual({
      name: "gamma",
      description: "Created from default",
      public: false,
      config: sampleDefaultProfile().config
    });
    expect(await screen.findByText("Profile created.")).toBeInTheDocument();
    expect(await findLibraryRow("gamma")).toBeTruthy();

    const workspace = container.querySelector(".profile-workspace");
    await fireEvent.click(within(workspace).getByRole("button", { name: "Delete" }));

    const dialog = await screen.findByRole("dialog", { name: 'Delete profile "gamma"?' });
    await fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(state.getProfiles().some((profile) => profile.name === "gamma")).toBe(false);
    });
    await waitFor(() => {
      expect(screen.queryByText("gamma")).not.toBeInTheDocument();
    });
    expect(screen.getByText("Profile deleted.")).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "default" })).toBeInTheDocument();
  });

  it("disables save button until draft is dirty and valid", async () => {
    installProfileFetch();

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    // Save should be disabled with no changes.
    const saveButton = screen.getByRole("button", { name: "Save" });
    expect(saveButton.disabled).toBe(true);

    // Make a change - save should become enabled.
    await fireEvent.input(screen.getByLabelText("Description"), {
      target: { value: "Changed description" }
    });
    expect(saveButton.disabled).toBe(false);
  });

  it("prompts to discard unsaved changes when switching profiles", async () => {
    installProfileFetch();
    global.confirm.mockReturnValue(false);

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    // Make a change to create dirty state.
    await fireEvent.input(screen.getByLabelText("Description"), {
      target: { value: "Dirty change" }
    });

    // Try to switch to alpha - confirm should be called and switch blocked.
    const alphaRow = await findLibraryRow("alpha");
    await fireEvent.click(within(alphaRow).getByRole("button", { name: /alpha/i }));

    expect(global.confirm).toHaveBeenCalledWith("Discard unsaved changes?");
    // Editor should still show beta's edited description.
    expect(screen.getByLabelText("Description").value).toBe("Dirty change");
  });

  it("formats JSON in the config editor", async () => {
    installProfileFetch();

    render(ProfileSettings);

    await findLibraryRow("default");
    await fireEvent.click(screen.getByRole("button", { name: "New profile" }));

    // Set compact (unformatted) JSON.
    await fireEvent.input(screen.getByLabelText("Config JSON"), {
      target: { value: '{"a":1,"b":2}' }
    });

    await fireEvent.click(screen.getByRole("button", { name: "Format JSON" }));

    const expected = JSON.stringify({ a: 1, b: 2 }, null, 2);
    expect(screen.getByLabelText("Config JSON").value).toBe(expected);
  });

  it("resets changes for an existing profile", async () => {
    installProfileFetch();

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    // Save should be disabled (no changes yet).
    expect(screen.getByRole("button", { name: "Save" }).disabled).toBe(true);

    await fireEvent.input(screen.getByLabelText("Description"), { target: { value: "Changed" } });

    // Save enabled after change - draft is dirty.
    expect(screen.getByRole("button", { name: "Save" }).disabled).toBe(false);

    // Reset button should be enabled when dirty.
    const resetButton = screen.getByRole("button", { name: "Reset changes" });
    expect(resetButton.disabled).toBe(false);

    await fireEvent.click(resetButton);

    // After reset, save should be disabled again (draft matches seed).
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Save" }).disabled).toBe(true);
    });
  });

  it("shows cancel button for new drafts and reset button is disabled when clean", async () => {
    installProfileFetch();

    render(ProfileSettings);

    await findLibraryRow("default");
    await fireEvent.click(screen.getByRole("button", { name: "New profile" }));

    // New draft: should show "Cancel new profile" instead of "Reset changes".
    expect(screen.getByRole("button", { name: "Cancel new profile" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reset changes" })).not.toBeInTheDocument();
  });

  it("shows validation error for empty name", async () => {
    installProfileFetch();

    render(ProfileSettings);

    await findLibraryRow("default");
    await fireEvent.click(screen.getByRole("button", { name: "New profile" }));

    // Leave name empty, type something in description to make draft dirty.
    await fireEvent.input(screen.getByLabelText("Description"), {
      target: { value: "Some description" }
    });

    // Validation error should appear for empty name.
    expect(screen.getByText("Name is required.")).toBeInTheDocument();
    // Save should be disabled because of validation error.
    expect(screen.getByRole("button", { name: "Save" }).disabled).toBe(true);
  });

  it("shows validation error for duplicate profile name", async () => {
    installProfileFetch();

    render(ProfileSettings);

    await findLibraryRow("default");
    await fireEvent.click(screen.getByRole("button", { name: "New profile" }));

    // Enter a name that already exists.
    await fireEvent.input(screen.getByLabelText("Name"), {
      target: { value: "alpha" }
    });

    expect(screen.getByText("A profile with that name already exists.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" }).disabled).toBe(true);
  });

  it("shows validation error for malformed JSON", async () => {
    installProfileFetch();

    render(ProfileSettings);

    await findLibraryRow("default");
    await fireEvent.click(screen.getByRole("button", { name: "New profile" }));

    await fireEvent.input(screen.getByLabelText("Name"), { target: { value: "new-profile" } });
    await fireEvent.input(screen.getByLabelText("Config JSON"), {
      target: { value: "not valid json" }
    });

    expect(screen.getByText("Config must be valid JSON.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" }).disabled).toBe(true);
  });

  it("shows validation error when config is a JSON array instead of object", async () => {
    installProfileFetch();

    render(ProfileSettings);

    await findLibraryRow("default");
    await fireEvent.click(screen.getByRole("button", { name: "New profile" }));

    await fireEvent.input(screen.getByLabelText("Name"), { target: { value: "arr-profile" } });
    await fireEvent.input(screen.getByLabelText("Config JSON"), {
      target: { value: "[1, 2, 3]" }
    });

    expect(screen.getByText("Config must be a JSON object.")).toBeInTheDocument();
  });

  it("deletes a profile from the library row action buttons", async () => {
    const state = installProfileFetch();

    render(ProfileSettings);

    const alphaRow = await findLibraryRow("alpha");
    await fireEvent.click(within(alphaRow).getByRole("button", { name: "Delete" }));

    // Row action opens the shared confirm dialog naming the target profile.
    expect(await screen.findByRole("dialog", { name: 'Delete profile "alpha"?' })).toBeInTheDocument();
  });

  it("duplicates a profile with a copy name", async () => {
    installProfileFetch();

    render(ProfileSettings);

    const alphaRow = await findLibraryRow("alpha");
    await fireEvent.click(within(alphaRow).getByRole("button", { name: "Duplicate" }));

    expect(screen.getByRole("heading", { name: "Duplicate draft" })).toBeInTheDocument();
    expect(screen.getByLabelText("Name").value).toBe("alpha copy");
    expect(screen.getByLabelText("Config JSON").value).toContain('"ipv4": true');
  });

  it("filters profiles by search query", async () => {
    installProfileFetch();

    render(ProfileSettings);

    await findLibraryRow("alpha");
    expect(await findLibraryRow("beta")).toBeTruthy();

    await fireEvent.input(screen.getByLabelText("Search profiles"), {
      target: { value: "strict" }
    });

    await waitFor(() => {
      const rows = screen.getAllByRole("listitem");
      expect(rows).toHaveLength(1);
    });
    expect(await findLibraryRow("beta")).toBeTruthy();
    expect(screen.queryByText("alpha")).not.toBeInTheDocument();
  });

  it("filters profiles by public filter", async () => {
    installProfileFetch();

    render(ProfileSettings);

    await findLibraryRow("alpha");

    const filterSelect = screen.getByLabelText("Filter");
    await fireEvent.change(filterSelect, { target: { value: "public" } });

    await waitFor(() => {
      const rows = screen.getAllByRole("listitem");
      expect(rows).toHaveLength(1);
    });
    expect(await findLibraryRow("alpha")).toBeTruthy();
  });

  it("filters profiles by in-use filter", async () => {
    installProfileFetch();

    render(ProfileSettings);

    await findLibraryRow("alpha");

    const filterSelect = screen.getByLabelText("Filter");
    await fireEvent.change(filterSelect, { target: { value: "in_use" } });

    await waitFor(() => {
      const rows = screen.getAllByRole("listitem");
      expect(rows).toHaveLength(1);
    });
    expect(await findLibraryRow("beta")).toBeTruthy();
  });

  it("after save, editor stays on the saved profile", async () => {
    const state = installProfileFetch();

    render(ProfileSettings);

    await findLibraryRow("default");
    await fireEvent.click(screen.getByRole("button", { name: "New profile" }));
    await fireEvent.input(screen.getByLabelText("Name"), { target: { value: "gamma" } });
    await fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(state.createdBodies).toHaveLength(1);
    });

    // After creation, the editor should show "Edit profile" for the newly created profile.
    expect(await screen.findByRole("heading", { name: "Edit profile" })).toBeInTheDocument();
  });

  // ── compatibility banner ───────────────────────────────────────────────────

  it("shows no compatibility banner for a compatible profile", async () => {
    installProfileFetch(); // all profiles return compatible by default

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    // Banner should not be present.
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("shows compatibility banner with issue detail for an incompatible profile", async () => {
    // beta (id=2) is incompatible
    installProfileFetch({ incompatibleProfileId: 2 });

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.getByText(/1 compatibility issue/i)).toBeInTheDocument();
    expect(screen.getAllByText(/zone14/).length).toBeGreaterThan(0);
  });

  it("banner shows action buttons for missing_test_case issues", async () => {
    installProfileFetch({ incompatibleProfileId: 2 });

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    await screen.findByRole("alert");
    expect(screen.getByRole("button", { name: "Add missing test cases" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reset test_cases to inherit" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Mark as reviewed" })).toBeInTheDocument();
  });

  it("compatibility banner disappears after switching to default profile", async () => {
    installProfileFetch({ incompatibleProfileId: 2 });

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));
    await screen.findByRole("alert");

    // Switch to default profile - banner should disappear.
    const defaultRow = await findLibraryRow("default");
    await fireEvent.click(within(defaultRow).getByRole("button", { name: /default/i }));

    await waitFor(() => {
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });
  });

  it("action buttons are disabled when draft has unsaved changes", async () => {
    installProfileFetch({ incompatibleProfileId: 2 });

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    await screen.findByRole("alert");

    // Make a change in the editor.
    await fireEvent.input(screen.getByLabelText("Description"), { target: { value: "changed" } });

    expect(screen.getByRole("button", { name: "Mark as reviewed" }).disabled).toBe(true);
    expect(screen.getByRole("button", { name: "Add missing test cases" }).disabled).toBe(true);
  });

  it("clicking 'Mark as reviewed' sends PATCH mark_reviewed op", async () => {
    const state = installProfileFetch({ incompatibleProfileId: 2 });

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    await screen.findByRole("alert");
    await fireEvent.click(screen.getByRole("button", { name: "Mark as reviewed" }));

    await waitFor(() => {
      expect(state.patchedBodies).toHaveLength(1);
    });
    expect(state.patchedBodies[0]).toEqual({ op: "mark_reviewed" });
  });

  it("clicking 'Add missing test cases' sends PATCH add_missing_test_cases op", async () => {
    const state = installProfileFetch({ incompatibleProfileId: 2 });

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    await screen.findByRole("alert");
    await fireEvent.click(screen.getByRole("button", { name: "Add missing test cases" }));

    await waitFor(() => {
      expect(state.patchedBodies).toHaveLength(1);
    });
    expect(state.patchedBodies[0]).toEqual({ op: "add_missing_test_cases" });
  });

  it("shows error notice when PATCH fix fails", async () => {
    installProfileFetch({ incompatibleProfileId: 2, patchError: "server unavailable" });

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    await screen.findByRole("alert");
    await fireEvent.click(screen.getByRole("button", { name: "Mark as reviewed" }));

    expect(await screen.findByText(/Failed to apply fix: server unavailable/i)).toBeInTheDocument();
  });

  // ── profile list: compat badges and summary ────────────────────────────────

  it("shows no 'needs review' warning when all profiles are compatible", async () => {
    installProfileFetch(); // no incompatibleProfileId - all compatible

    render(ProfileSettings);
    await findLibraryRow("alpha");

    expect(screen.queryByText("Needs review")).not.toBeInTheDocument();
    expect(screen.queryByText(/profile\(s\) need review/i)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /mark all as reviewed/i })).not.toBeInTheDocument();
  });

  it("shows 'Needs review' badge on incompatible profiles", async () => {
    installProfileFetch({ incompatibleProfileId: 2 });

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    expect(await within(betaRow).findByText("Needs review")).toBeInTheDocument();

    // alpha should not have the badge
    const alphaRow = await findLibraryRow("alpha");
    expect(within(alphaRow).queryByText("Needs review")).not.toBeInTheDocument();
  });

  it("shows summary line with count when profiles need review", async () => {
    installProfileFetch({ incompatibleProfileId: 2 });

    render(ProfileSettings);
    await findLibraryRow("alpha");

    expect(await screen.findByText(/1 profile\(s\) need review/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /mark all as reviewed/i })).toBeInTheDocument();
  });

  it("clicking 'Mark all as reviewed' asks for confirmation before posting", async () => {
    const state = installProfileFetch({ incompatibleProfileId: 2 });

    render(ProfileSettings);
    await findLibraryRow("alpha");

    const btn = await screen.findByRole("button", { name: /mark all as reviewed/i });
    await fireEvent.click(btn);

    // The confirmation names every profile it is about to waive issues on.
    expect(await screen.findByText("Mark all profiles as reviewed?")).toBeInTheDocument();
    expect(screen.getByText("beta: 1 issue(s)")).toBeInTheDocument();
    expect(state.getMarkAllReviewedCalls()).toBe(0);

    await fireEvent.click(screen.getByRole("button", { name: /mark all as reviewed/i }));

    await waitFor(() => {
      expect(state.getMarkAllReviewedCalls()).toBe(1);
    });
  });

  it("cancelling the mark-all confirmation sends nothing and restores the banner", async () => {
    const state = installProfileFetch({ incompatibleProfileId: 2 });

    render(ProfileSettings);
    await findLibraryRow("alpha");

    await fireEvent.click(await screen.findByRole("button", { name: /mark all as reviewed/i }));
    await screen.findByText("Mark all profiles as reviewed?");

    await fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(await screen.findByText(/1 profile\(s\) need review/i)).toBeInTheDocument();
    expect(screen.queryByText("Mark all profiles as reviewed?")).not.toBeInTheDocument();
    expect(state.getMarkAllReviewedCalls()).toBe(0);
  });

  it("summary line disappears after mark all reviewed", async () => {
    installProfileFetch({ incompatibleProfileId: 2 });

    render(ProfileSettings);
    await findLibraryRow("alpha");

    await screen.findByText(/1 profile\(s\) need review/i);
    await fireEvent.click(screen.getByRole("button", { name: /mark all as reviewed/i }));
    await screen.findByText("Mark all profiles as reviewed?");
    await fireEvent.click(screen.getByRole("button", { name: /mark all as reviewed/i }));

    await waitFor(() => {
      expect(screen.queryByText(/profile\(s\) need review/i)).not.toBeInTheDocument();
    });
  });

  it("shows error notice when mark all reviewed fails", async () => {
    installProfileFetch({ incompatibleProfileId: 2, markAllError: "db unavailable" });

    render(ProfileSettings);
    await findLibraryRow("alpha");

    await screen.findByText(/1 profile\(s\) need review/i);
    await fireEvent.click(screen.getByRole("button", { name: /mark all as reviewed/i }));
    await screen.findByText("Mark all profiles as reviewed?");
    await fireEvent.click(screen.getByRole("button", { name: /mark all as reviewed/i }));

    expect(await screen.findByText(/Failed to apply fix: db unavailable/i)).toBeInTheDocument();
  });

  it("shows a waived badge on library rows that carry one", async () => {
    installProfileFetch({ waivedProfileId: 1 });

    render(ProfileSettings);

    const alphaRow = await findLibraryRow("alpha");
    expect(await within(alphaRow).findByText("3 waived")).toBeInTheDocument();

    const betaRow = await findLibraryRow("beta");
    expect(within(betaRow).queryByText(/waived/)).not.toBeInTheDocument();
  });

  // ── review state ───────────────────────────────────────────────────────────

  it("shows the reviewed line with the waived issues for a reviewed profile", async () => {
    installProfileFetch({ reviewedProfileId: 2 });

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    expect(await screen.findByText("Reviewed against v1.0.0")).toBeInTheDocument();
    expect(screen.getByText("1 issue(s) waived by this review")).toBeInTheDocument();
    // The waived issues stay collapsed until asked for.
    expect(screen.queryAllByText(/zone15/)).toHaveLength(0);

    await fireEvent.click(screen.getByRole("button", { name: "Show waived issues" }));
    expect(screen.getAllByText(/zone15/).length).toBeGreaterThan(0);
  });

  it("re-checking a reviewed profile clears the stamp and brings the banner back", async () => {
    const state = installProfileFetch({ reviewedProfileId: 2 });

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));
    await screen.findByText("Reviewed against v1.0.0");

    await fireEvent.click(screen.getByRole("button", { name: "Check again" }));

    await waitFor(() => {
      expect(state.patchedBodies).toHaveLength(1);
    });
    expect(state.patchedBodies[0]).toEqual({ op: "clear_reviewed" });

    // The normal incompatible banner and its fix buttons take over.
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add missing test cases" })).toBeInTheDocument();
    expect(screen.queryByText("Reviewed against v1.0.0")).not.toBeInTheDocument();
  });

  it("shows no reviewed line for an unreviewed profile", async () => {
    installProfileFetch({ incompatibleProfileId: 2 });

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    await screen.findByRole("alert");
    expect(screen.queryByRole("button", { name: "Check again" })).not.toBeInTheDocument();
  });

  // ── diff panel ─────────────────────────────────────────────────────────────

  it("shows the diff summary for the selected profile", async () => {
    installProfileFetch();

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    expect(await screen.findByText("Differences from engine defaults")).toBeInTheDocument();
    expect(screen.getByText(
      "2 deviation(s), 2 redundant override(s), 1 missing key(s), 0 unknown key(s)"
    )).toBeInTheDocument();
  });

  it("sends the unsaved draft config to the diff endpoint", async () => {
    const state = installProfileFetch();

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));
    await screen.findByText("Differences from engine defaults");

    await fireEvent.input(screen.getByLabelText("Config JSON"), {
      target: { value: '{"net":{"ipv6":false}}' }
    });

    await waitFor(() => {
      expect(state.diffBodies.at(-1)).toEqual({ config: { net: { ipv6: false } } });
    });
  });

  it("expands the diff into a table of per-property rows", async () => {
    installProfileFetch();

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));
    await screen.findByText("Differences from engine defaults");

    await fireEvent.click(screen.getByRole("button", { name: "Show details" }));

    expect(screen.getByText("resolver.defaults.nameserver_max_total_ms")).toBeInTheDocument();
    expect(screen.getByText("60000")).toBeInTheDocument();
    // Two properties restate the default: the scalar and the tunables map.
    expect(screen.getAllByText("Restates the engine default")).toHaveLength(2);
    expect(screen.getByText("EXTRA_ADDRESS_CHILD: NOTICE -> WARNING")).toBeInTheDocument();
  });

  it("marks missing keys of a wholesale property as the dangerous class", async () => {
    installProfileFetch();

    const { container } = render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));
    await screen.findByText("Differences from engine defaults");
    await fireEvent.click(screen.getByRole("button", { name: "Show details" }));

    const danger = container.querySelector(".diff-danger");
    expect(danger).toBeTruthy();
    expect(danger.textContent).toContain("MISSING_ADDRESS_CHILD");
    expect(screen.getByText(/omitted tags resolve to DEBUG/)).toBeInTheDocument();
  });

  it("strips only the wholly redundant properties from the draft", async () => {
    installProfileFetch();

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));
    await screen.findByText("Differences from engine defaults");

    await fireEvent.input(screen.getByLabelText("Config JSON"), {
      target: {
        value: JSON.stringify({
          badkeys: { path: "" },
          test_levels: { BASIC: { B01_CHILD_FOUND: "INFO" } },
          test_cases_vars: { zone13: { SPF_LOOKUP_LIMIT: 10 } },
          resolver: { defaults: { nameserver_max_total_ms: 60000 } }
        })
      }
    });

    const strip = await screen.findByRole("button", { name: "Strip 2 redundant override(s)" });
    await fireEvent.click(strip);

    const config = JSON.parse(screen.getByLabelText("Config JSON").value);
    // Redundant: removed, and the emptied parent object pruned with it.
    expect(config.badkeys).toBeUndefined();
    expect(config.test_cases_vars).toBeUndefined();
    // Deviating: kept, including the partially redundant wholesale-replace map.
    expect(config.test_levels).toEqual({ BASIC: { B01_CHILD_FOUND: "INFO" } });
    expect(config.resolver).toEqual({ defaults: { nameserver_max_total_ms: 60000 } });
    // The draft is dirty and can be saved.
    expect(screen.getByRole("button", { name: "Save" }).disabled).toBe(false);
  });

  it("disables the strip button when nothing is redundant", async () => {
    installProfileFetch({
      diff: {
        summary: { deviations: 1, redundant: 0, missing: 0, unknown: 0 },
        properties: [
          { path: "net.ipv6", kind: "changed", redundant: false, default: true, value: false }
        ]
      }
    });

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));

    const strip = await screen.findByRole("button", { name: "No redundant overrides" });
    expect(strip.disabled).toBe(true);
  });

  it("shows no diff panel on the default profile view", async () => {
    installProfileFetch();

    render(ProfileSettings);
    await findLibraryRow("default");

    // The default profile view opens on load and is read-only.
    expect(screen.getByRole("heading", { name: "default" })).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByText("Differences from engine defaults")).not.toBeInTheDocument();
    });
  });

  it("hides the diff panel while the draft config is not valid JSON", async () => {
    installProfileFetch();

    render(ProfileSettings);

    const betaRow = await findLibraryRow("beta");
    await fireEvent.click(within(betaRow).getByRole("button", { name: /beta/i }));
    await screen.findByText("Differences from engine defaults");

    await fireEvent.input(screen.getByLabelText("Config JSON"), {
      target: { value: "not valid json" }
    });

    await waitFor(() => {
      expect(screen.queryByText("Differences from engine defaults")).not.toBeInTheDocument();
    });
  });
});
