import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SettingsPanel from "./SettingsPanel.svelte";

const settingsSubTabs = [
  { id: "system", labelKey: "settings_subtab_system" },
  { id: "profiles", labelKey: "settings_subtab_profiles" },
  { id: "scoring", labelKey: "settings_subtab_scoring" },
];

describe("SettingsPanel", () => {
  beforeEach(() => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      headers: { get: () => "application/json" },
      json: async () => ({}),
      text: async () => "{}",
    });
  });

  afterEach(() => cleanup());

  it("renders the three sub-tab buttons", () => {
    render(SettingsPanel, { props: { settingsSubTab: "system", settingsSubTabs } });
    expect(screen.getByRole("tab", { name: /System/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Profiles/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Scoring/i })).toBeInTheDocument();
  });

  it("marks the active sub-tab as aria-selected=true and the others false", () => {
    render(SettingsPanel, { props: { settingsSubTab: "profiles", settingsSubTabs } });
    expect(screen.getByRole("tab", { name: /Profiles/i })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: /System/i })).toHaveAttribute("aria-selected", "false");
  });

  it("invokes onSetSubTab with the clicked sub-tab id", async () => {
    const onSetSubTab = vi.fn();
    render(SettingsPanel, { props: { settingsSubTab: "system", settingsSubTabs, onSetSubTab } });
    await fireEvent.click(screen.getByRole("tab", { name: /Scoring/i }));
    expect(onSetSubTab).toHaveBeenCalledWith("scoring");
  });

  it("only mounts the subpanel matching settingsSubTab", () => {
    render(SettingsPanel, { props: { settingsSubTab: "scoring", settingsSubTabs } });
    expect(document.getElementById("settings-subpanel-scoring")).not.toBeNull();
    expect(document.getElementById("settings-subpanel-system")).toBeNull();
    expect(document.getElementById("settings-subpanel-profiles")).toBeNull();
  });
});
