import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ExpiredResult from "./ExpiredResult.svelte";

describe("ExpiredResult", () => {
  beforeEach(() => vi.restoreAllMocks());
  afterEach(() => cleanup());

  it("shows expired heading", () => {
    render(ExpiredResult);
    expect(screen.getByText(/no longer available/i)).toBeTruthy();
  });

  it("shows the expired body text", () => {
    render(ExpiredResult);
    expect(screen.getByText(/expired or does not exist/i)).toBeTruthy();
  });

  it("shows Run new test button", () => {
    render(ExpiredResult);
    expect(screen.getByTestId("new-test-button")).toBeTruthy();
  });

  it("calls onnewtest callback when button is clicked", async () => {
    const handler = vi.fn();
    render(ExpiredResult, { props: { onnewtest: handler } });
    await fireEvent.click(screen.getByTestId("new-test-button"));
    expect(handler).toHaveBeenCalledOnce();
  });
});
