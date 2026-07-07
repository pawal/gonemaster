import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";
import ConfirmDialog from "./ConfirmDialog.svelte";
import { setCatalog } from "../i18n.js";

setCatalog("en", { cancel: "Cancel", submitting: "Working..." });

describe("ConfirmDialog", () => {
  afterEach(() => cleanup());

  it("renders nothing when closed", () => {
    const { container } = render(ConfirmDialog, { props: { open: false, title: "Delete?" } });
    expect(container.querySelector(".modal-card")).toBeNull();
  });

  it("shows title, message and confirm/cancel, and fires the callbacks", async () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    render(ConfirmDialog, {
      props: { open: true, title: "Delete profile?", message: "This cannot be undone.", confirmLabel: "Delete", onConfirm, onCancel },
    });
    expect(screen.getByRole("dialog", { name: "Delete profile?" })).toBeInTheDocument();
    expect(screen.getByText("This cannot be undone.")).toBeInTheDocument();

    await fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(onConfirm).toHaveBeenCalled();
    await fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalled();
  });

  it("keeps confirm disabled until the required phrase is typed", async () => {
    const onConfirm = vi.fn();
    render(ConfirmDialog, {
      props: { open: true, title: "Purge?", confirmLabel: "Purge", confirmPhrase: "my-tag", phrasePrompt: "Type my-tag", onConfirm },
    });
    const confirm = screen.getByRole("button", { name: "Purge" });
    expect(confirm).toBeDisabled();

    await fireEvent.input(screen.getByLabelText("Type my-tag"), { target: { value: "my-tag" } });
    expect(confirm).not.toBeDisabled();
    await fireEvent.click(confirm);
    expect(onConfirm).toHaveBeenCalled();
  });

  it("cancels on Escape and on backdrop click", async () => {
    const onCancel = vi.fn();
    const { container } = render(ConfirmDialog, { props: { open: true, title: "X", onCancel } });
    await fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onCancel).toHaveBeenCalledTimes(1);
    await fireEvent.click(container.querySelector(".modal-backdrop"));
    expect(onCancel).toHaveBeenCalledTimes(2);
  });
});
