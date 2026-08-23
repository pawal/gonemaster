import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ServerSettings from "./ServerSettings.svelte";
import { jsonResponse, requestUrl, serverSettingsFixture } from "../test/helpers.js";

const settingsMock = (url, options = {}) => {
  const value = requestUrl(url);
  if (value.includes("/api/v1/settings") && (!options.method || options.method === "GET")) {
    return jsonResponse(serverSettingsFixture());
  }
  if (value.includes("/api/v1/settings") && options.method === "PUT") {
    return jsonResponse({ status: "ok" });
  }
  return jsonResponse({});
};

describe("ServerSettings", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  // Renders and waits for the settings fetch to paint the named field.
  const renderLoaded = async (label = /Worker count/) => {
    render(ServerSettings);
    await waitFor(() => {
      expect(screen.getByLabelText(label)).toBeInTheDocument();
    });
  };

  it("renders server settings with labels and values after loading", async () => {
    global.fetch.mockImplementation(settingsMock);
    render(ServerSettings);

    await waitFor(() => {
      expect(screen.getByText("Server Settings")).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(screen.getByLabelText(/Worker count/)).toBeInTheDocument();
    });

    const workerInput = screen.getByLabelText(/Worker count/);
    expect(workerInput.value).toBe("4");
    expect(workerInput.disabled).toBe(false);

    const listenInput = screen.getByLabelText(/Listen address/);
    expect(listenInput.value).toBe("127.0.0.1:8080");
    expect(listenInput.disabled).toBe(true);

    expect(screen.getByLabelText(/Show nameserver timings in admin UI/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Show nameserver timings in public UI/)).toBeInTheDocument();
  });

  it("shows source labels for settings", async () => {
    global.fetch.mockImplementation(settingsMock);
    await renderLoaded();

    expect(screen.getAllByText("(default)", { exact: false }).length).toBeGreaterThan(0);
    expect(screen.getAllByText("(config file)", { exact: false }).length).toBeGreaterThan(0);
  });

  it("disables save button when no changes are made", async () => {
    global.fetch.mockImplementation(settingsMock);
    await renderLoaded();

    const saveButton = screen.getByRole("button", { name: "Save changes" });
    expect(saveButton.disabled).toBe(true);
  });

  it("enables save button after editing a mutable setting", async () => {
    global.fetch.mockImplementation(settingsMock);
    await renderLoaded();

    const workerInput = screen.getByLabelText(/Worker count/);
    await fireEvent.input(workerInput, { target: { value: "8" } });

    const saveButton = screen.getByRole("button", { name: "Save changes" });
    expect(saveButton.disabled).toBe(false);
  });

  it("sends PUT request with changed values on save", async () => {
    const calls = [];
    global.fetch.mockImplementation((url, options = {}) => {
      const value = requestUrl(url);
      if (options.method === "PUT") calls.push({ url: value, body: JSON.parse(options.body) });
      return settingsMock(url, options);
    });

    await renderLoaded();

    const workerInput = screen.getByLabelText(/Worker count/);
    await fireEvent.input(workerInput, { target: { value: "8" } });
    await fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(calls.length).toBe(1);
    });
    expect(calls[0].body.worker_count).toBe(8);
  });

  it("shows success toast after saving settings", async () => {
    global.fetch.mockImplementation(settingsMock);
    await renderLoaded();

    await fireEvent.input(screen.getByLabelText(/Worker count/), { target: { value: "8" } });
    await fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(screen.getByText("Settings saved.")).toBeInTheDocument();
    });
  });

  it("shows error when settings fail to load", async () => {
    global.fetch.mockImplementation((url, options = {}) => {
      const value = requestUrl(url);
      if (value.includes("/api/v1/settings")) {
        return jsonResponse({ error: { message: "db connection lost" } }, false);
      }
      return settingsMock(url, options);
    });

    render(ServerSettings);

    await waitFor(() => {
      expect(screen.getByText(/Failed to load settings/)).toBeInTheDocument();
    });
  });

  it("renders readonly settings as disabled inputs", async () => {
    global.fetch.mockImplementation(settingsMock);
    await renderLoaded(/Database driver/);

    expect(screen.getByLabelText(/Database driver/).disabled).toBe(true);
    expect(screen.getByLabelText(/Database DSN/).disabled).toBe(true);
    expect(screen.getByLabelText(/Profile path/).disabled).toBe(true);
  });

  it("renders toggle inputs for boolean settings", async () => {
    global.fetch.mockImplementation(settingsMock);
    await renderLoaded(/Rate limiting/);

    const toggle = screen.getByLabelText(/Rate limiting/);
    expect(toggle.type).toBe("checkbox");
    expect(toggle.checked).toBe(false);
  });

  it("renders the non-global query targets toggle in the Public API group", async () => {
    global.fetch.mockImplementation(settingsMock);
    render(ServerSettings);

    await waitFor(() => {
      expect(screen.getByLabelText(/non-global query targets/i)).toBeInTheDocument();
    });

    const toggle = screen.getByLabelText(/non-global query targets/i);
    expect(toggle.type).toBe("checkbox");
  });
});
