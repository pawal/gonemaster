import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";
import ShortcutsHelp from "./ShortcutsHelp.svelte";

describe("ShortcutsHelp", () => {
  afterEach(() => cleanup());

  it("renders nothing when closed", () => {
    const { container } = render(ShortcutsHelp, { props: { open: false } });
    expect(container.querySelector(".modal-card")).toBeNull();
  });

  it("shows the title and both sections when open", () => {
    render(ShortcutsHelp, { props: { open: true } });
    expect(screen.getByRole("dialog", { name: "Keyboard shortcuts help" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Keyboard shortcuts" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Global" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Go to section" })).toBeInTheDocument();
  });

  it("lists a g-sequence row for every nav section", () => {
    render(ShortcutsHelp, { props: { open: true } });
    // Labels come from the nav list via shortcutNavRows.
    for (const label of ["Overview", "Domains", "Nameservers", "Addresses", "ASNs", "Trends", "Diff"]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
    // The g prefix is shown for the section chords.
    expect(screen.getAllByText("g").length).toBeGreaterThan(0);
  });

  it("closes on Escape, backdrop click and the Close button", async () => {
    const onClose = vi.fn();
    const { container } = render(ShortcutsHelp, { props: { open: true, onClose } });
    await fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
    await fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(2);
    await fireEvent.click(container.querySelector(".modal-backdrop")!);
    expect(onClose).toHaveBeenCalledTimes(3);
  });

  it("does not close when the card itself is clicked", async () => {
    const onClose = vi.fn();
    render(ShortcutsHelp, { props: { open: true, onClose } });
    await fireEvent.click(screen.getByRole("dialog"));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("moves focus to the Close button when opened", async () => {
    render(ShortcutsHelp, { props: { open: true } });
    await waitFor(() => expect(screen.getByRole("button", { name: "Close" })).toHaveFocus());
  });
});
