import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ServerSettings from "./ServerSettings.svelte";

const jsonResponse = (data, ok = true) => ({
  ok,
  statusText: ok ? "OK" : "Bad Request",
  headers: { get: () => "application/json" },
  json: async () => data,
  text: async () => JSON.stringify(data),
});

const sampleSettings = () => ({
  listen_addr: { value: "127.0.0.1:8080", source: "default", readonly: true },
  db_driver: { value: "", source: "default", readonly: true },
  db_dsn: { value: "", source: "default", readonly: true },
  profile_path: { value: "", source: "default", readonly: true },
  worker_count: { value: 4, source: "default" },
  max_concurrent_jobs: { value: 0, source: "default" },
  min_level: { value: "INFO", source: "config_file" },
  retention_days: { value: 0, source: "default" },
  public_url: { value: "", source: "default" },
  rate_limit_enabled: { value: false, source: "default" },
  rate_limit_max: { value: 10, source: "default" },
  rate_limit_window: { value: "10m0s", source: "default" },
  show_score_admin: { value: true, source: "default" },
  show_score_public: { value: true, source: "default" },
  show_nameserver_timings_admin: { value: true, source: "default" },
  show_nameserver_timings_public: { value: true, source: "default" },
});

const settingsMock = (url, options = {}) => {
  const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
  if (value.includes("/api/v1/settings") && (!options.method || options.method === "GET")) {
    return jsonResponse(sampleSettings());
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
    render(ServerSettings);

    await waitFor(() => {
      expect(screen.getByLabelText(/Worker count/)).toBeInTheDocument();
    });

    expect(screen.getAllByText("(default)", { exact: false }).length).toBeGreaterThan(0);
    expect(screen.getAllByText("(config file)", { exact: false }).length).toBeGreaterThan(0);
  });

  it("disables save button when no changes are made", async () => {
    global.fetch.mockImplementation(settingsMock);
    render(ServerSettings);

    await waitFor(() => {
      expect(screen.getByLabelText(/Worker count/)).toBeInTheDocument();
    });

    const saveButton = screen.getByRole("button", { name: "Save changes" });
    expect(saveButton.disabled).toBe(true);
  });

  it("enables save button after editing a mutable setting", async () => {
    global.fetch.mockImplementation(settingsMock);
    render(ServerSettings);

    await waitFor(() => {
      expect(screen.getByLabelText(/Worker count/)).toBeInTheDocument();
    });

    const workerInput = screen.getByLabelText(/Worker count/);
    await fireEvent.input(workerInput, { target: { value: "8" } });

    const saveButton = screen.getByRole("button", { name: "Save changes" });
    expect(saveButton.disabled).toBe(false);
  });

  it("sends PUT request with changed values on save", async () => {
    const calls = [];
    global.fetch.mockImplementation((url, options = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
      if (options.method === "PUT") calls.push({ url: value, body: JSON.parse(options.body) });
      return settingsMock(url, options);
    });

    render(ServerSettings);

    await waitFor(() => {
      expect(screen.getByLabelText(/Worker count/)).toBeInTheDocument();
    });

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
    render(ServerSettings);

    await waitFor(() => {
      expect(screen.getByLabelText(/Worker count/)).toBeInTheDocument();
    });

    await fireEvent.input(screen.getByLabelText(/Worker count/), { target: { value: "8" } });
    await fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(screen.getByText("Settings saved.")).toBeInTheDocument();
    });
  });

  it("shows error when settings fail to load", async () => {
    global.fetch.mockImplementation((url, options = {}) => {
      const value = typeof url === "string" ? url : String(url?.url || url?.href || url || "");
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
    render(ServerSettings);

    await waitFor(() => {
      expect(screen.getByLabelText(/Database driver/)).toBeInTheDocument();
    });

    expect(screen.getByLabelText(/Database driver/).disabled).toBe(true);
    expect(screen.getByLabelText(/Database DSN/).disabled).toBe(true);
    expect(screen.getByLabelText(/Profile path/).disabled).toBe(true);
  });

  it("renders toggle inputs for boolean settings", async () => {
    global.fetch.mockImplementation(settingsMock);
    render(ServerSettings);

    await waitFor(() => {
      expect(screen.getByLabelText(/Rate limiting/)).toBeInTheDocument();
    });

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
