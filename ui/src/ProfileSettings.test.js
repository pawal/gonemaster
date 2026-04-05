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

  it("loads profile rows and filters by public and in-use state", async () => {
    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/profiles")) return jsonResponse(sampleProfiles());
      if (value.includes("/api/v1/tags?limit=500")) return jsonResponse(sampleTags());
      return jsonResponse({});
    });

    render(ProfileSettings);

    const alphaRow = await findLibraryRow("alpha");
    const betaRow = await findLibraryRow("beta");
    expect(alphaRow).toBeTruthy();
    expect(betaRow).toBeTruthy();
    expect(screen.getByText("In use · 2")).toBeInTheDocument();
    expect(within(alphaRow).getByText("Public")).toBeInTheDocument();

    await fireEvent.change(screen.getByLabelText("Filter"), { target: { value: "in_use" } });
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /alpha/i })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: /beta/i })).toBeInTheDocument();

    await fireEvent.change(screen.getByLabelText("Filter"), { target: { value: "public" } });
    expect(screen.getByRole("button", { name: /alpha/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /beta/i })).not.toBeInTheDocument();
  });

  it("opens duplicate and new draft previews from the library entry points", async () => {
    global.fetch.mockImplementation((url) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/profiles")) return jsonResponse(sampleProfiles());
      if (value.includes("/api/v1/tags?limit=500")) return jsonResponse(sampleTags());
      return jsonResponse({});
    });

    render(ProfileSettings);

    const alphaListItem = await findLibraryRow("alpha");
    await fireEvent.click(within(alphaListItem).getByRole("button", { name: "Duplicate" }));

    expect(screen.getByRole("heading", { name: "Duplicate draft" })).toBeInTheDocument();
    expect(screen.getByText("alpha copy")).toBeInTheDocument();
    expect(screen.getByText("Duplicated from alpha")).toBeInTheDocument();
    expect(screen.getByText(/"ipv4": true/)).toBeInTheDocument();

    await fireEvent.click(screen.getByRole("button", { name: "New profile" }));
    expect(screen.getByRole("heading", { name: "New profile draft" })).toBeInTheDocument();
    expect(screen.getByText("New unsaved profile")).toBeInTheDocument();
    expect(screen.getByText("{}")).toBeInTheDocument();
  });

  it("deletes a profile and refreshes the library", async () => {
    let profiles = sampleProfiles();
    let tags = sampleTags();

    global.fetch.mockImplementation((url, options = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (value.includes("/api/v1/profiles/2") && options.method === "DELETE") {
        profiles = profiles.filter((profile) => profile.id !== 2);
        tags = tags.map((tag) => ({ ...tag, default_profile_id: null }));
        return jsonResponse("");
      }
      if (value.includes("/api/v1/profiles")) return jsonResponse(profiles);
      if (value.includes("/api/v1/tags?limit=500")) return jsonResponse(tags);
      return jsonResponse({});
    });

    render(ProfileSettings);

    const betaListItem = await findLibraryRow("beta");
    await fireEvent.click(within(betaListItem).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(screen.queryByText("beta")).not.toBeInTheDocument();
    });
    expect(global.confirm).toHaveBeenCalledWith('Delete profile "beta"?');
    expect(screen.getByText("Profile deleted.")).toBeInTheDocument();
  });
});
