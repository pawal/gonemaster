import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { status, setStatus, clearStatus } from "./status.svelte.js";

describe("status store", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    clearStatus();
  });

  afterEach(() => {
    clearStatus();
    vi.useRealTimers();
  });

  it("starts empty", () => {
    expect(status.message).toBe("");
    expect(status.tone).toBe("");
  });

  it("setStatus stores message and tone", () => {
    setStatus("hi", "ok");
    expect(status.message).toBe("hi");
    expect(status.tone).toBe("ok");
  });

  it("auto-dismisses an ok message after 5000ms", () => {
    setStatus("hi", "ok");
    vi.advanceTimersByTime(4999);
    expect(status.message).toBe("hi");
    vi.advanceTimersByTime(1);
    expect(status.message).toBe("");
    expect(status.tone).toBe("");
  });

  it("auto-dismisses a warn message after 8000ms", () => {
    setStatus("oops", "warn");
    vi.advanceTimersByTime(7999);
    expect(status.message).toBe("oops");
    vi.advanceTimersByTime(1);
    expect(status.message).toBe("");
  });

  it("does not arm a timer when called with an empty message", () => {
    setStatus("hi", "ok");
    setStatus("", "ok");
    expect(status.message).toBe("");
    vi.advanceTimersByTime(10000);
    expect(status.message).toBe("");
  });

  it("replacing a message resets the dismiss timer", () => {
    setStatus("first", "ok");
    vi.advanceTimersByTime(4000);
    setStatus("second", "ok");
    vi.advanceTimersByTime(4000);
    expect(status.message).toBe("second");
    vi.advanceTimersByTime(1000);
    expect(status.message).toBe("");
  });

  it("clearStatus wipes message and cancels the timer", () => {
    setStatus("hi", "ok");
    clearStatus();
    expect(status.message).toBe("");
    vi.advanceTimersByTime(10000);
    expect(status.message).toBe("");
  });
});
