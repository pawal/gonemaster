import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ShareButton from "./ShareButton.svelte";

describe("ShareButton", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    Object.defineProperty(window, "location", {
      value: { origin: "https://example.com", pathname: "/public/" },
      writable: true,
      configurable: true,
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders Share button", () => {
    render(ShareButton, { props: { publicID: "abc12345" } });
    expect(screen.getByTestId("share-button").textContent.trim()).toBe("Share result");
  });

  it("calls clipboard.writeText with the result URL", async () => {
    const write = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText: write } });
    render(ShareButton, { props: { publicID: "abc12345" } });
    await fireEvent.click(screen.getByTestId("share-button"));
    await waitFor(() => expect(write).toHaveBeenCalledWith(
      "https://example.com/public/#/result/abc12345"
    ));
  });

  it("shows Copied! after click", async () => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } });
    render(ShareButton, { props: { publicID: "abc12345" } });
    await fireEvent.click(screen.getByTestId("share-button"));
    await waitFor(() =>
      expect(screen.getByTestId("share-button").textContent.trim()).toBe("Copied!")
    );
  });

  it("reverts to Share after 2s", async () => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } });
    render(ShareButton, { props: { publicID: "abc12345" } });
    await fireEvent.click(screen.getByTestId("share-button"));
    await waitFor(() =>
      expect(screen.getByTestId("share-button").textContent.trim()).toBe("Copied!")
    );
    await vi.advanceTimersByTimeAsync(2001);
    await waitFor(() =>
      expect(screen.getByTestId("share-button").textContent.trim()).toBe("Share result")
    );
  });
});
