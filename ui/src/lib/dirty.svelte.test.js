import { describe, it, expect, vi, afterEach } from "vitest";
import { dirtyGuard } from "./dirty.svelte.js";

afterEach(() => {
  dirtyGuard.clear();
  vi.restoreAllMocks();
});

describe("dirtyGuard", () => {
  it("allows leaving when not dirty without prompting", () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    dirtyGuard.register(false, "Discard?");
    expect(dirtyGuard.confirmLeave()).toBe(true);
    expect(confirm).not.toHaveBeenCalled();
  });

  it("prompts when dirty and blocks leaving if the user cancels", () => {
    vi.spyOn(window, "confirm").mockReturnValue(false);
    dirtyGuard.register(true, "Discard?");
    expect(dirtyGuard.confirmLeave()).toBe(false);
    expect(dirtyGuard.dirty).toBe(true);
  });

  it("clears the guard once the user confirms discarding", () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    dirtyGuard.register(true, "Discard?");
    expect(dirtyGuard.confirmLeave()).toBe(true);
    expect(confirm).toHaveBeenCalledWith("Discard?");
    expect(dirtyGuard.dirty).toBe(false);
  });

  it("clear() resets the dirty flag", () => {
    dirtyGuard.register(true, "Discard?");
    expect(dirtyGuard.dirty).toBe(true);
    dirtyGuard.clear();
    expect(dirtyGuard.dirty).toBe(false);
  });
});
