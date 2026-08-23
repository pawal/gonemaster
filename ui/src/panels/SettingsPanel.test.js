import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SettingsPanel from "./SettingsPanel.svelte";
import { jsonResponse } from "../test/helpers.js";

const settingsSubTabs = [
  { id: "system", labelKey: "settings_subtab_system" },
  { id: "profiles", labelKey: "settings_subtab_profiles" },
  { id: "scoring", labelKey: "settings_subtab_scoring" },
];

describe("SettingsPanel", () => {
  beforeEach(() => {
    global.fetch = vi.fn().mockResolvedValue(jsonResponse({}));
  });

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

  it("gives only the active tab a tabindex of 0 (roving tabindex)", () => {
    render(SettingsPanel, { props: { settingsSubTab: "profiles", settingsSubTabs } });
    expect(screen.getByRole("tab", { name: /Profiles/i })).toHaveAttribute("tabindex", "0");
    expect(screen.getByRole("tab", { name: /System/i })).toHaveAttribute("tabindex", "-1");
  });

  it("moves to the next sub-tab on ArrowRight and wraps at the end", async () => {
    const onSetSubTab = vi.fn();
    render(SettingsPanel, { props: { settingsSubTab: "system", settingsSubTabs, onSetSubTab } });
    await fireEvent.keyDown(screen.getByRole("tab", { name: /System/i }), { key: "ArrowRight" });
    expect(onSetSubTab).toHaveBeenCalledWith("profiles");

    cleanup();
    onSetSubTab.mockClear();
    render(SettingsPanel, { props: { settingsSubTab: "scoring", settingsSubTabs, onSetSubTab } });
    await fireEvent.keyDown(screen.getByRole("tab", { name: /Scoring/i }), { key: "ArrowRight" });
    expect(onSetSubTab).toHaveBeenCalledWith("system");
  });

  it("moves to the previous sub-tab on ArrowLeft and to the ends on Home/End", async () => {
    const onSetSubTab = vi.fn();
    render(SettingsPanel, { props: { settingsSubTab: "profiles", settingsSubTabs, onSetSubTab } });
    const profiles = screen.getByRole("tab", { name: /Profiles/i });
    await fireEvent.keyDown(profiles, { key: "ArrowLeft" });
    expect(onSetSubTab).toHaveBeenCalledWith("system");
    await fireEvent.keyDown(profiles, { key: "End" });
    expect(onSetSubTab).toHaveBeenCalledWith("scoring");
    await fireEvent.keyDown(profiles, { key: "Home" });
    expect(onSetSubTab).toHaveBeenCalledWith("system");
  });
});
