import { render, screen, fireEvent, waitFor, within, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ProfileSettings from "./ProfileSettings.svelte";

const jsonResponse = (data, ok = true) => ({
  ok,
  statusText: ok ? "OK" : "Bad Request",
  headers: {
    get: () => "application/json"
  },
  json: async () => data,
  text: async () => JSON.stringify(data)
});

const emptyResponse = () => ({
  ok: true,
  statusText: "No Content",
  headers: {
    get: () => ""
  },
  json: async () => ({}),
  text: async () => ""
});

describe("ProfileSettings", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    global.fetch = vi.fn();
    global.confirm = vi.fn(() => true);
  });

  afterEach(() => {
    cleanup();
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

  const sampleProfiles = () => ([
    {
      id: 1,
      name: "alpha",
      description: "Public baseline",
      config: { net: { ipv4: true } },
      public: true,
      created_at: "2026-04-01T10:00:00Z",
      updated_at: "2026-04-02T10:00:00Z"
    },
    {
      id: 2,
      name: "beta",
      description: "Strict resolver profile",
      config: { resolver: { defaults: { timeout: 5 } } },
      public: false,
      created_at: "2026-04-01T11:00:00Z",
      updated_at: "2026-04-03T09:30:00Z"
    }
  ]);

  const sampleTags = () => ([
    { name: "ops", default_profile_id: 2 },
    { name: "prod", default_profile_id: 2 }
  ]);

  const installProfileFetch = () => {
    let profiles = sampleProfiles();
    const tags = sampleTags();
    const createdBodies = [];
    const updatedBodies = [];

    global.fetch.mockImplementation((url, options = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      const method = options.method || "GET";

      if (value === "/api/v1/profiles/default") return jsonResponse(sampleDefaultProfile());
      if (value === "/api/v1/profiles" && method === "GET") return jsonResponse(profiles);
      if (value === "/api/v1/profiles" && method === "POST") {
        const body = JSON.parse(options.body);
        createdBodies.push(body);
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
        const body = JSON.parse(options.body);
        updatedBodies.push(body);
        const updated = {
          id: 2,
          ...body,
          created_at: "2026-04-01T11:00:00Z",
          updated_at: "2026-04-05T09:30:00Z"
        };
        profiles = profiles.map((profile) => profile.id === 2 ? updated : profile);
        return jsonResponse(updated);
      }
      if (value === "/api/v1/profiles/3" && method === "DELETE") {
        profiles = profiles.filter((profile) => profile.id !== 3);
        return emptyResponse();
      }
      if (value === "/api/v1/tags?limit=500") return jsonResponse(tags);
      return jsonResponse({});
    });

    return {
      createdBodies,
      updatedBodies,
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

    await waitFor(() => {
      expect(state.getProfiles().some((profile) => profile.name === "gamma")).toBe(false);
    });
    await waitFor(() => {
      expect(screen.queryByText("gamma")).not.toBeInTheDocument();
    });
    expect(global.confirm).toHaveBeenCalledWith('Delete profile "gamma"?');
    expect(screen.getByText("Profile deleted.")).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "default" })).toBeInTheDocument();
  });
});
