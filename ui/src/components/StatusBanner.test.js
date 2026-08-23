import { render, screen, fireEvent } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";
import StatusBanner from "./StatusBanner.svelte";
import { setStatus, clearStatus, status } from "../lib/status.svelte.js";

describe("StatusBanner", () => {
  afterEach(() => {
    clearStatus();
  });

  it("renders nothing when there is no message", () => {
    const { container } = render(StatusBanner);
    expect(container.querySelector(".status-toast")).toBeNull();
  });

  it("shows the message and an 'ok' tone class", async () => {
    render(StatusBanner);
    setStatus("Job created.", "ok");
    expect(await screen.findByText(/Job created\./)).toBeInTheDocument();
    const toast = document.querySelector(".status-toast");
    expect(toast).not.toBeNull();
    expect(toast.classList.contains("status-ok")).toBe(true);
  });

  it("uses a 'warn' tone class for warning messages", async () => {
    render(StatusBanner);
    setStatus("Something broke.", "warn");
    expect(await screen.findByText(/Something broke\./)).toBeInTheDocument();
    const toast = document.querySelector(".status-toast");
    expect(toast.classList.contains("status-warn")).toBe(true);
  });

  it("dismiss button clears the status", async () => {
    render(StatusBanner);
    setStatus("Hello.", "ok");
    expect(await screen.findByText(/Hello\./)).toBeInTheDocument();
    const dismiss = screen.getByRole("button", { name: /Dismiss notification/i });
    await fireEvent.click(dismiss);
    expect(status.message).toBe("");
    expect(document.querySelector(".status-toast")).toBeNull();
  });
});
