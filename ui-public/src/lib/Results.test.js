import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Results from "./Results.svelte";

const resultResp = (entries = [], testcase_descriptions = {}, nameserver_timings = []) => ({
  ok: true,
  status: 200,
  json: async () => ({
    job_id: "test-id",
    status: "succeeded",
    raw: { locale: "en", entries },
    nameserver_timings,
    testcase_descriptions,
  }),
});

const errResp = (status) => ({
  ok: false,
  status,
  json: async () => ({}),
});

const entry = (module, level, message = "msg", testcase = "tc") => ({
  timestamp: 0,
  module,
  testcase,
  tag: "TAG",
  level,
  message,
});

describe("Results", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    global.fetch = vi.fn();
  });

  afterEach(() => cleanup());

  it("shows loading indicator before fetch resolves", async () => {
    let resolve;
    global.fetch.mockReturnValue(new Promise((r) => { resolve = r; }));
    render(Results, { props: { publicID: "abc12345" } });
    expect(screen.getByTestId("results-loading")).toBeTruthy();
    resolve(resultResp([]));
  });

  it("shows result banner after successful fetch", async () => {
    global.fetch.mockResolvedValue(resultResp([]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("result-banner")).toBeTruthy());
  });

  it("banner has ok class when no entries", async () => {
    global.fetch.mockResolvedValue(resultResp([]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => {
      const banner = screen.getByTestId("result-banner");
      expect(banner.classList.contains("ok")).toBe(true);
    });
  });

  it("banner has error class when worst level is ERROR", async () => {
    global.fetch.mockResolvedValue(resultResp([
      entry("Module::A", "INFO"),
      entry("Module::A", "ERROR"),
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => {
      const banner = screen.getByTestId("result-banner");
      expect(banner.classList.contains("error")).toBe(true);
    });
  });

  it("groups entries by module", async () => {
    global.fetch.mockResolvedValue(resultResp([
      entry("Module::Alpha", "INFO"),
      entry("Module::Beta", "WARNING"),
      entry("Module::Alpha", "NOTICE"),
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() =>
      expect(screen.getAllByTestId("module-group")).toHaveLength(2)
    );
  });

  it("shows per-level count badges in module summary", async () => {
    global.fetch.mockResolvedValue(resultResp([
      entry("Module::Alpha", "WARNING", "w msg"),
      entry("Module::Alpha", "INFO", "i msg"),
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => {
      const badges = screen.getAllByTestId("module-badge");
      const texts = badges.map((b) => b.textContent.trim());
      expect(texts).toContain("INFO 1");
      expect(texts).toContain("WARNING 1");
    });
  });

  it("shows result rows inside module group", async () => {
    global.fetch.mockResolvedValue(resultResp([
      entry("Module::Alpha", "INFO", "first"),
      entry("Module::Alpha", "WARNING", "second"),
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() =>
      expect(screen.getAllByTestId("result-row")).toHaveLength(2)
    );
  });

  it("groups entries by testcase within module", async () => {
    global.fetch.mockResolvedValue(resultResp([
      entry("ADDRESS", "INFO", "msg1", "Address01"),
      entry("ADDRESS", "WARNING", "msg2", "Address02"),
      entry("ADDRESS", "INFO", "msg3", "Address01"),
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() =>
      expect(screen.getAllByTestId("testcase-group")).toHaveLength(2)
    );
    // Descriptions come from i18n (en.json pub.tc.address01 / pub.tc.address02).
    // The title appears both in the testcase <summary> and as a small
    // per-row caption above each finding, so use getAllByText.
    expect(screen.getAllByText(/globally routable/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/Reverse DNS entry/i).length).toBeGreaterThan(0);
  });

  it("testcase groups default closed for INFO/NOTICE, open for WARNING+", async () => {
    global.fetch.mockResolvedValue(resultResp([
      entry("ADDRESS", "INFO", "ok msg", "Address01"),
      entry("ADDRESS", "WARNING", "warn msg", "Address02"),
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() =>
      expect(screen.getAllByTestId("testcase-group")).toHaveLength(2)
    );
    const groups = screen.getAllByTestId("testcase-group");
    // Address01 has only INFO → closed; Address02 has WARNING → open
    const infoGroup = groups.find((g) => g.querySelector("summary").textContent.includes("globally routable"));
    const warnGroup = groups.find((g) => g.querySelector("summary").textContent.includes("Reverse DNS"));
    expect(infoGroup.open).toBe(false);
    expect(warnGroup.open).toBe(true);
  });

  it("shows share button when domain prop is set", async () => {
    global.fetch.mockResolvedValue(resultResp([]));
    render(Results, { props: { publicID: "abc12345", domain: "example.com" } });
    await waitFor(() =>
      expect(screen.getByTestId("share-button")).toBeTruthy()
    );
  });

  it("shows formatted finished date when finishedAt prop is set", async () => {
    global.fetch.mockResolvedValue(resultResp([]));
    render(Results, { props: { publicID: "abc12345", domain: "example.com", finishedAt: "2024-06-15T14:30:00Z" } });
    await waitFor(() => {
      // The formatted date should contain at least the year
      expect(screen.getByText(/2024/)).toBeTruthy();
    });
  });

  it("does not show date when finishedAt is null", async () => {
    global.fetch.mockResolvedValue(resultResp([]));
    render(Results, { props: { publicID: "abc12345", domain: "example.com", finishedAt: null } });
    await waitFor(() => screen.getByTestId("result-banner"));
    expect(document.querySelector(".result-date")).toBeNull();
  });

  it("renders nameserver timing table when data is present", async () => {
    global.fetch.mockResolvedValue(resultResp([], {}, [
      {
        nameserver: "ns1.example.com",
        address: "192.0.2.10",
        avg_ms: 24,
        min_ms: 20,
        max_ms: 30,
        count: 3,
      },
      {
        nameserver: "ns2.example.com",
        address: "192.0.2.20",
        avg_ms: 12,
        min_ms: 10,
        max_ms: 14,
        count: 2,
      },
    ]));
    render(Results, { props: { publicID: "abc12345", domain: "example.com" } });
    await waitFor(() => expect(screen.getByTestId("nameserver-timings")).toBeTruthy());
    const timings = screen.getByTestId("nameserver-timings");
    expect(timings.open).toBe(false);

    timings.open = true;
    await fireEvent(timings, new Event("toggle"));

    expect(screen.getAllByTestId("nameserver-timing-row")).toHaveLength(2);
    expect(screen.getByText("ns1.example.com")).toBeTruthy();
    expect(screen.getByText("192.0.2.10")).toBeTruthy();
    expect(screen.getByText("24")).toBeTruthy();
  });

  it("hides nameserver timing table when no data is present", async () => {
    global.fetch.mockResolvedValue(resultResp([]));
    render(Results, { props: { publicID: "abc12345", domain: "example.com" } });
    await waitFor(() => expect(screen.getByTestId("result-banner")).toBeTruthy());
    expect(screen.queryByTestId("nameserver-timings")).toBeNull();
  });

  it("hides nameserver timing table when display is disabled", async () => {
    global.fetch.mockResolvedValue(resultResp([], {}, [
      {
        nameserver: "ns1.example.com",
        address: "192.0.2.10",
        avg_ms: 24,
        min_ms: 20,
        max_ms: 30,
        count: 3,
      },
    ]));
    render(Results, { props: { publicID: "abc12345", domain: "example.com", nameserverTimingsEnabled: false } });
    await waitFor(() => expect(screen.getByTestId("result-banner")).toBeTruthy());
    expect(screen.queryByTestId("nameserver-timings")).toBeNull();
  });

  it("shows error on 404", async () => {
    global.fetch.mockResolvedValue(errResp(404));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  });

  it("shows error on network failure", async () => {
    global.fetch.mockRejectedValue(new Error("network down"));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
  });

  it("keeps module open after locale re-fetch", async () => {
    const mkResp = (msg) => ({
      ok: true, status: 200,
      json: async () => ({ job_id: "x", status: "succeeded", raw: { locale: "en", entries: [entry("Module::Alpha", "INFO", msg)] } }),
    });
    global.fetch.mockResolvedValueOnce(mkResp("first")).mockResolvedValueOnce(mkResp("second"));
    const { rerender } = render(Results, { props: { publicID: "abc12345", locale: "en" } });
    await waitFor(() => screen.getByTestId("module-group"));

    // Open the module group by setting its open property and firing toggle
    const details = screen.getByTestId("module-group");
    details.open = true;
    await fireEvent(details, new Event("toggle"));

    // Change locale — results re-fetch
    await rerender({ locale: "sv" });
    await waitFor(() => screen.getByText("second"));

    // Module group should still be open
    expect(screen.getByTestId("module-group").open).toBe(true);
  });

  it("renders tag header inline when pub.tag.<module>.<TAG>.header exists", async () => {
    global.fetch.mockResolvedValue(resultResp([
      { timestamp: 0, module: "DELEGATION", testcase: "delegation01", tag: "NOT_ENOUGH_NS_DEL", level: "WARNING", message: "Delegation does not list enough (1) nameservers." },
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("result-row")).toBeTruthy());
    const header = screen.getByTestId("result-tag-header");
    expect(header.textContent.trim()).toBe("Not enough nameservers");
    // Header is rendered inside the same message span as the templated
    // engine text, so both are on screen.
    expect(screen.getByText(/Delegation does not list enough/i)).toBeTruthy();
  });

  it("does not render a tag header when the i18n key is missing", async () => {
    global.fetch.mockResolvedValue(resultResp([
      { timestamp: 0, module: "DELEGATION", testcase: "delegation01", tag: "NO_SUCH_TAG_XYZ", level: "WARNING", message: "some message" },
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("result-row")).toBeTruthy());
    expect(screen.queryByTestId("result-tag-header")).toBeNull();
    // The engine message still renders.
    expect(screen.getByText(/some message/i)).toBeTruthy();
  });

  it("renders the About-this-finding disclosure with both the testcase description and the tag description when both keys exist", async () => {
    global.fetch.mockResolvedValue(resultResp([
      { timestamp: 0, module: "DELEGATION", testcase: "delegation01", tag: "NOT_ENOUGH_NS_DEL", level: "WARNING", message: "x" },
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("result-row")).toBeTruthy());
    const disclosure = screen.getByTestId("result-explanation");
    expect(disclosure).toBeTruthy();
    expect(disclosure.textContent).toMatch(/About this finding/i);
    // Testcase description renders first (general context).
    expect(screen.getByTestId("result-explanation-test").textContent)
      .toMatch(/at least two authoritative name servers/i);
    // Tag description follows (finding-specific).
    expect(screen.getByTestId("result-explanation-tag").textContent)
      .toMatch(/parent zone's delegation lists fewer/i);
  });

  it("shows only the testcase paragraph in the disclosure when the tag description is missing", async () => {
    global.fetch.mockResolvedValue(resultResp([
      { timestamp: 0, module: "DELEGATION", testcase: "delegation01", tag: "NO_SUCH_TAG_XYZ", level: "WARNING", message: "x" },
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("result-row")).toBeTruthy());
    expect(screen.getByTestId("result-explanation")).toBeTruthy();
    expect(screen.getByTestId("result-explanation-test")).toBeTruthy();
    expect(screen.queryByTestId("result-explanation-tag")).toBeNull();
  });

  it("shows only the tag paragraph in the disclosure when the testcase description is missing", async () => {
    global.fetch.mockResolvedValue(resultResp([
      { timestamp: 0, module: "DELEGATION", testcase: "UnknownTC", tag: "NOT_ENOUGH_NS_DEL", level: "WARNING", message: "x" },
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("result-row")).toBeTruthy());
    expect(screen.getByTestId("result-explanation")).toBeTruthy();
    expect(screen.queryByTestId("result-explanation-test")).toBeNull();
    expect(screen.getByTestId("result-explanation-tag")).toBeTruthy();
  });

  it("does not render the About disclosure when neither description key exists", async () => {
    global.fetch.mockResolvedValue(resultResp([
      { timestamp: 0, module: "DELEGATION", testcase: "UnknownTC", tag: "NO_SUCH_TAG_XYZ", level: "WARNING", message: "x" },
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("result-row")).toBeTruthy());
    expect(screen.queryByTestId("result-explanation")).toBeNull();
  });

  it("renders a per-row testcase-title caption above each finding", async () => {
    global.fetch.mockResolvedValue(resultResp([
      { timestamp: 0, module: "DELEGATION", testcase: "delegation01", tag: "NOT_ENOUGH_NS_DEL", level: "WARNING", message: "x" },
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("result-row-caption")).toBeTruthy());
    expect(screen.getByTestId("result-row-caption").textContent.trim())
      .toBe("Minimum number of name servers");
  });

  it("re-fetches with new locale when locale prop changes", async () => {
    const enResp = {
      ok: true, status: 200,
      json: async () => ({ job_id: "x", status: "succeeded", raw: { locale: "en", entries: [entry("M", "INFO", "english msg")] } }),
    };
    const svResp = {
      ok: true, status: 200,
      json: async () => ({ job_id: "x", status: "succeeded", raw: { locale: "sv", entries: [entry("M", "INFO", "swedish msg")] } }),
    };
    global.fetch.mockResolvedValueOnce(enResp).mockResolvedValueOnce(svResp);
    const { rerender } = render(Results, { props: { publicID: "abc12345", locale: "en" } });
    await waitFor(() => expect(screen.getByText("english msg")).toBeTruthy());

    await rerender({ locale: "sv" });
    await waitFor(() => expect(screen.getByText("swedish msg")).toBeTruthy());
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls[1][0]).toContain("locale=sv");
  });
});
