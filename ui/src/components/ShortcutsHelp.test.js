import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { describe, expect, it, vi } from "vitest";
import ShortcutsHelp from "./ShortcutsHelp.svelte";
import { setCatalog } from "../i18n.js";

setCatalog("en", {
  shortcuts_help_title: "Keyboard shortcuts",
  shortcuts_help_aria: "Keyboard shortcuts help",
  shortcuts_section_global: "Global",
  shortcuts_section_navigation: "Go to",
  shortcuts_section_results: "Results",
  shortcut_help_toggle: "Show or hide this help",
  shortcut_new_scan: "New scan",
  shortcut_focus_filter: "Focus filter",
  shortcut_go_hint: "Press g, then a letter",
  shortcut_row_next: "Next result",
  shortcut_row_prev: "Previous result",
  shortcut_row_open: "Open result",
  shortcut_close: "Close",
  tab_single: "Single Job",
  tab_recent: "Recent Tests",
  tab_domains: "Domains",
  tab_tags: "Tags",
  tab_cohorts: "Cohorts",
  tab_batches: "Batch Jobs",
  tab_metrics: "Metrics",
  tab_settings: "Settings",
});

describe("ShortcutsHelp", () => {
  it("renders nothing when closed", () => {
    const { container } = render(ShortcutsHelp, { props: { open: false } });
    expect(container.querySelector(".modal-card")).toBeNull();
  });

  it("shows the title and all three sections when open", () => {
    render(ShortcutsHelp, { props: { open: true } });
    expect(screen.getByRole("dialog", { name: "Keyboard shortcuts help" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Keyboard shortcuts" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Global" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Go to" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Results" })).toBeInTheDocument();
    expect(screen.getByText("New scan")).toBeInTheDocument();
  });

  it("closes on Escape, backdrop click and the Close button", async () => {
    const onClose = vi.fn();
    const { container } = render(ShortcutsHelp, { props: { open: true, onClose } });
    await fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
    await fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(2);
    await fireEvent.click(container.querySelector(".modal-backdrop"));
    expect(onClose).toHaveBeenCalledTimes(3);
  });

  it("moves focus to the Close button when opened", async () => {
    render(ShortcutsHelp, { props: { open: true } });
    await waitFor(() => expect(screen.getByRole("button", { name: "Close" })).toHaveFocus());
  });
});
