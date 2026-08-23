import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ScoringSettings from "./ScoringSettings.svelte";
import { jsonResponse, requestUrl } from "../test/helpers.js";

const defaultConfig = () => ({
  severity_penalties: { NOTICE: 1, WARNING: 5, ERROR: 20, CRITICAL: 0 },
  category_weights: { dnssec: 1.5, nameserver_health: 1.2, connectivity: 1.0, zone_consistency: 0.8 },
  module_categories: { DNSSEC: "dnssec", NAMESERVER: "nameserver_health" },
  tag_penalties: { DS07_NOT_SIGNED: 20 },
  grade_bands: [
    { grade: "A", min_score: 90 },
    { grade: "B", min_score: 75 },
    { grade: "F", min_score: 0 },
  ],
  bonus_criteria: {
    no_warnings_or_errors: true,
    dnssec_enabled: true,
    strong_algorithm: true,
    nsec3_non_optout: true,
    cds_cdnskey_published: true,
    ipv6_all_nameservers: true,
    as_diversity: true,
  },
});

const configResponse = (overrides = {}) =>
  Object.assign({ config: defaultConfig(), source: "default", readonly: false }, overrides);

describe("ScoringSettings", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  // Renders and waits for the config fetch to paint the first section.
  const renderLoaded = async (heading = "Severity Penalties") => {
    render(ScoringSettings);
    await waitFor(() => {
      expect(screen.getByText(heading)).toBeInTheDocument();
    });
  };

  // load

  it("loads and displays the scoring config", async () => {
    global.fetch.mockResolvedValue(jsonResponse(configResponse()));
    await renderLoaded();

    // Severity table rows
    expect(screen.getByText("NOTICE")).toBeInTheDocument();
    expect(screen.getByText("WARNING")).toBeInTheDocument();

    // Category weights
    expect(screen.getByText("Category Weights")).toBeInTheDocument();
    expect(screen.getByText("dnssec")).toBeInTheDocument();

    // Tag override row
    expect(screen.getByText("Tag Penalty Overrides")).toBeInTheDocument();
    expect(screen.getByDisplayValue("DS07_NOT_SIGNED")).toBeInTheDocument();

    // Grade bands
    expect(screen.getByText("Grade Bands")).toBeInTheDocument();

    // Bonus criteria
    expect(screen.getByText("DNSSEC enabled")).toBeInTheDocument();
  });

  it("shows source indicator", async () => {
    global.fetch.mockResolvedValue(jsonResponse(configResponse({ source: "database" })));
    render(ScoringSettings);

    await waitFor(() => {
      expect(screen.getByText(/database/)).toBeInTheDocument();
    });
  });

  it("shows readonly notice when source is cli_flag", async () => {
    global.fetch.mockResolvedValue(jsonResponse(configResponse({ source: "cli_flag", readonly: true })));
    render(ScoringSettings);

    await waitFor(() => {
      expect(screen.getByRole("alert")).toBeInTheDocument();
    });
    expect(screen.getByRole("alert").textContent).toMatch(/CLI flag/i);

    // Save button absent when readonly
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
  });

  it("disables inputs when readonly", async () => {
    global.fetch.mockResolvedValue(jsonResponse(configResponse({ source: "cli_flag", readonly: true })));
    await renderLoaded();

    const numberInputs = screen.getAllByRole("spinbutton");
    for (const input of numberInputs) {
      expect(input).toBeDisabled();
    }
  });

  // edit

  it("save button appears after editing a severity penalty", async () => {
    global.fetch.mockResolvedValue(jsonResponse(configResponse()));
    await renderLoaded();

    // Save should start disabled (no changes).
    const saveBtn = screen.getByRole("button", { name: "Save" });
    expect(saveBtn).toBeDisabled();

    // Edit WARNING penalty.
    const warningInput = screen.getByLabelText(/Penalty WARNING/i);
    await fireEvent.input(warningInput, { target: { value: "99" } });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Save" })).not.toBeDisabled();
    });
  });

  it("save calls PUT /api/v1/scoring-config and reloads", async () => {
    let putCalled = false;
    global.fetch.mockImplementation((url, opts) => {
      const path = requestUrl(url);
      if (opts?.method === "PUT" && path.includes("/scoring-config")) {
        putCalled = true;
        return Promise.resolve(jsonResponse({ status: "ok" }));
      }
      // GET returns default config.
      return Promise.resolve(jsonResponse(configResponse()));
    });

    await renderLoaded();

    // Make a change.
    const warningInput = screen.getByLabelText(/Penalty WARNING/i);
    await fireEvent.input(warningInput, { target: { value: "77" } });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Save" })).not.toBeDisabled();
    });

    await fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(putCalled).toBe(true);
    });

    // After save, notice shown.
    await waitFor(() => {
      expect(screen.getByText("Scoring configuration saved.")).toBeInTheDocument();
    });
  });

  it("discard resets to loaded config", async () => {
    global.fetch.mockResolvedValue(jsonResponse(configResponse()));
    await renderLoaded();

    const warningInput = screen.getByLabelText(/Penalty WARNING/i);
    await fireEvent.input(warningInput, { target: { value: "77" } });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Save" })).not.toBeDisabled();
    });

    await fireEvent.click(screen.getByRole("button", { name: "Discard changes" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    });
  });

  // tag overrides

  it("can add and remove tag penalty override rows", async () => {
    global.fetch.mockResolvedValue(jsonResponse(configResponse()));
    await renderLoaded("Tag Penalty Overrides");

    // Add a new row.
    await fireEvent.click(screen.getByRole("button", { name: "Add override" }));

    await waitFor(() => {
      // The new empty tag input appears.
      const tagInputs = screen.getAllByLabelText(/Tag \d+/);
      expect(tagInputs.length).toBeGreaterThan(1);
    });

    // Remove the first row.
    const removeButtons = screen.getAllByRole("button", { name: "Remove" });
    await fireEvent.click(removeButtons[0]);

    await waitFor(() => {
      // Back to the count before adding.
      const tagInputs = screen.getAllByLabelText(/Tag \d+/);
      expect(tagInputs.length).toBe(1);
    });
  });

  // reset to defaults

  it("reset to defaults fetches defaults endpoint and populates form", async () => {
    let defaultsFetched = false;
    const modifiedConfig = defaultConfig();
    modifiedConfig.severity_penalties.WARNING = 99;

    global.fetch.mockImplementation((url) => {
      const path = requestUrl(url);
      if (path.includes("/scoring-config/defaults")) {
        defaultsFetched = true;
        return Promise.resolve(jsonResponse(defaultConfig()));
      }
      return Promise.resolve(jsonResponse(configResponse({ config: modifiedConfig })));
    });

    await renderLoaded();

    await fireEvent.click(screen.getByRole("button", { name: "Reset to defaults" }));

    await waitFor(() => {
      expect(defaultsFetched).toBe(true);
    });

    // After reset, save button should be enabled (form is now different from loaded).
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Save" })).not.toBeDisabled();
    });
  });

  // module mapping toggle

  it("module mapping is collapsed by default and expands on click", async () => {
    global.fetch.mockResolvedValue(jsonResponse(configResponse()));
    await renderLoaded();

    // Module mapping initially hidden.
    expect(screen.queryByText("DNSSEC")).toBeNull();

    await fireEvent.click(screen.getByRole("button", { name: "Show module mapping" }));

    await waitFor(() => {
      expect(screen.getByDisplayValue("DNSSEC")).toBeInTheDocument();
    });

    // Hide again.
    await fireEvent.click(screen.getByRole("button", { name: "Hide module mapping" }));
    await waitFor(() => {
      expect(screen.queryByDisplayValue("DNSSEC")).toBeNull();
    });
  });

  // import JSON

  it("import JSON panel applies valid JSON to the form", async () => {
    global.fetch.mockResolvedValue(jsonResponse(configResponse()));
    await renderLoaded();

    await fireEvent.click(screen.getByRole("button", { name: "Import JSON" }));

    const textarea = screen.getByRole("textbox", { name: "Import JSON" });
    const importConfig = defaultConfig();
    importConfig.severity_penalties.ERROR = 77;
    await fireEvent.input(textarea, { target: { value: JSON.stringify(importConfig) } });

    await fireEvent.click(screen.getByRole("button", { name: "Apply" }));

    // Panel should close and save should be enabled.
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Apply" })).toBeNull();
    });
    expect(screen.getByRole("button", { name: "Save" })).not.toBeDisabled();
  });

  it("import JSON shows error on invalid JSON", async () => {
    global.fetch.mockResolvedValue(jsonResponse(configResponse()));
    await renderLoaded();

    await fireEvent.click(screen.getByRole("button", { name: "Import JSON" }));

    const textarea = screen.getByRole("textbox", { name: "Import JSON" });
    await fireEvent.input(textarea, { target: { value: "not valid json {{" } });
    await fireEvent.click(screen.getByRole("button", { name: "Apply" }));

    await waitFor(() => {
      expect(screen.getByText(/Invalid JSON/i)).toBeInTheDocument();
    });
  });

  // default reconciliation

  const reconcileMock = (stored, defaults, configOverrides = {}) =>
    (url) => {
      const path = requestUrl(url);
      if (path.includes("/scoring-config/defaults")) {
        return Promise.resolve(jsonResponse(defaults));
      }
      return Promise.resolve(jsonResponse(configResponse(Object.assign({ config: stored }, configOverrides))));
    };

  it("surfaces default tag and module entries missing from a saved config", async () => {
    const stored = defaultConfig(); // DS07_NOT_SIGNED, DNSSEC, NAMESERVER
    const defaults = defaultConfig();
    defaults.tag_penalties = { ...defaults.tag_penalties, N18_NO_RESPONSE: 0, N18_FILTERED_RESPONSE: 0 };
    defaults.module_categories = { ...defaults.module_categories, BASIC: "nameserver_health" };

    global.fetch.mockImplementation(reconcileMock(stored, defaults, { source: "database" }));
    render(ScoringSettings);

    await waitFor(() => {
      expect(screen.getByText(/3 new default scoring entries/)).toBeInTheDocument();
    });
    expect(screen.getByText("N18_NO_RESPONSE = 0")).toBeInTheDocument();
    expect(screen.getByText("N18_FILTERED_RESPONSE = 0")).toBeInTheDocument();
    expect(screen.getByText("BASIC → nameserver_health")).toBeInTheDocument();
  });

  it("add missing defaults appends rows, enables save, and clears the banner", async () => {
    const stored = defaultConfig();
    const defaults = defaultConfig();
    defaults.tag_penalties = { ...defaults.tag_penalties, N18_NO_RESPONSE: 0 };

    global.fetch.mockImplementation(reconcileMock(stored, defaults, { source: "database" }));
    render(ScoringSettings);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Add missing defaults" })).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();

    await fireEvent.click(screen.getByRole("button", { name: "Add missing defaults" }));

    await waitFor(() => {
      expect(screen.getByDisplayValue("N18_NO_RESPONSE")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Save" })).not.toBeDisabled();
    expect(screen.queryByText(/new default scoring entries/)).toBeNull();
  });

  it("shows no banner when the saved config already has every default entry", async () => {
    const cfg = defaultConfig();
    global.fetch.mockImplementation(reconcileMock(cfg, defaultConfig(), { source: "database" }));
    await renderLoaded();
    expect(screen.queryByText(/new default scoring entries/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Add missing defaults" })).toBeNull();
  });

  it("does not surface missing defaults when readonly", async () => {
    const stored = defaultConfig();
    const defaults = defaultConfig();
    defaults.tag_penalties = { ...defaults.tag_penalties, N18_NO_RESPONSE: 0 };

    global.fetch.mockImplementation(reconcileMock(stored, defaults, { source: "cli_flag", readonly: true }));
    await renderLoaded();
    expect(screen.queryByRole("button", { name: "Add missing defaults" })).toBeNull();
  });
});
