import { render, screen, fireEvent, cleanup } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";
import RunResultBody from "./RunResultBody.svelte";

const sampleResult = {
  job_id: "j1",
  summary: { levels: { NOTICE: 0, WARNING: 1, ERROR: 0, CRITICAL: 0 } },
  raw: {
    entries: [
      { module: "DNSSEC", level: "WARNING", testcase: "dnssec01", message: "warn-msg" },
    ],
  },
};

describe("RunResultBody", () => {
  afterEach(() => cleanup());

  it("renders nothing when result is null", () => {
    const { container } = render(RunResultBody, { props: { result: null } });
    expect(container.querySelector(".status-banner")).toBeNull();
    expect(container.querySelector(".module-list")).toBeNull();
  });

  it("renders the summary, banner, and module card when given a result", () => {
    render(RunResultBody, { props: { result: sampleResult } });
    expect(screen.getByText("WARNING")).toBeInTheDocument();
    expect(screen.getByText("DNSSEC")).toBeInTheDocument();
  });

  it("expands a module to reveal its entries when its toggle is clicked", async () => {
    render(RunResultBody, { props: { result: sampleResult } });
    expect(screen.queryByText("warn-msg")).toBeNull();
    await fireEvent.click(screen.getByRole("button", { expanded: false, name: /DNSSEC/i }));
    expect(screen.getByText("warn-msg")).toBeInTheDocument();
  });

  it("uses idPrefix when constructing module-body ids", async () => {
    render(RunResultBody, { props: { result: sampleResult, idPrefix: "dm-" } });
    await fireEvent.click(screen.getByRole("button", { expanded: false, name: /DNSSEC/i }));
    const body = document.querySelector(".module-body");
    expect(body).not.toBeNull();
    expect(body.id.startsWith("dm-module-")).toBe(true);
  });

  it("renders the nameserver timings table only when enabled and timings are present", () => {
    const resultWithTimings = {
      ...sampleResult,
      nameserver_timings: [
        { nameserver: "ns1", address: "1.2.3.4", avg_ms: 10, min_ms: 5, max_ms: 20, count: 3 },
      ],
    };

    const { rerender, container } = render(RunResultBody, {
      props: { result: resultWithTimings, nameserverTimingsEnabled: false },
    });
    expect(container.querySelector('[data-testid="admin-nameserver-timings"]')).toBeNull();

    cleanup();
    render(RunResultBody, {
      props: { result: resultWithTimings, nameserverTimingsEnabled: true },
    });
    expect(document.querySelector('[data-testid="admin-nameserver-timings"]')).not.toBeNull();
  });

  it("marks a timed-out nameserver as unreachable instead of showing a response time", () => {
    const resultWithTimings = {
      ...sampleResult,
      nameserver_timings: [
        { nameserver: "ns1", address: "1.2.3.4", avg_ms: 10, min_ms: 5, max_ms: 20, count: 3, status: "ok" },
        { nameserver: "ns.cocca.fr", address: "2.3.4.5", status: "unreachable" },
      ],
    };

    render(RunResultBody, {
      props: { result: resultWithTimings, nameserverTimingsEnabled: true },
    });

    const rows = document.querySelectorAll('[data-testid="admin-nameserver-timing-row"]');
    const dead = [...rows].find((r) => r.textContent.includes("ns.cocca.fr"));
    expect(dead).toBeTruthy();
    expect(dead.getAttribute("data-status")).toBe("unreachable");
    // The infinity marker stands in for the timeout; no fabricated 0/large ms.
    expect(dead.textContent).toContain("∞");
    expect(dead.querySelector(".ns-timings-badge-unreachable")).not.toBeNull();
  });
});
