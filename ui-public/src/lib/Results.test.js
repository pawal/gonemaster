import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Results from "./Results.svelte";
import { errorResponse, jsonResponse } from "../test/helpers.js";

const resultResp = (entries = [], testcase_descriptions = {}, nameserver_timings = []) =>
  jsonResponse({
    job_id: "test-id",
    status: "succeeded",
    raw: { locale: "en", entries },
    nameserver_timings,
    testcase_descriptions,
  });

const errResp = (status) => errorResponse(status);

const entry = (module, level, message = "msg", testcase = "tc") => ({
  timestamp: 0,
  module,
  testcase,
  tag: "TAG",
  level,
  message,
});

const taggedEntry = (tag, args, level = "INFO") => ({
  timestamp: 0,
  module: "BASIC",
  testcase: "Basic01",
  tag,
  level,
  args,
  message: "msg",
});

describe("Results", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

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

  it("renders the explanation header for a blocked non-global query", async () => {
    global.fetch.mockResolvedValue(resultResp([
      {
        timestamp: 0,
        module: "System",
        testcase: "",
        tag: "NON_GLOBAL_QUERY_BLOCKED",
        level: "NOTICE",
        message: "Query to ns1.example/192.168.0.1 skipped",
      },
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() =>
      expect(screen.getByText("Query skipped for a non-public address")).toBeInTheDocument()
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

  it("renders unreachable and unresolved nameserver rows with status badges", async () => {
    global.fetch.mockResolvedValue(resultResp([], {}, [
      // ok row: has samples.
      {
        nameserver: "ns1.example.com",
        address: "192.0.2.10",
        avg_ms: 24,
        min_ms: 20,
        max_ms: 30,
        count: 3,
        status: "ok",
      },
      // unreachable: has address, no samples.
      {
        nameserver: "dead.example.com",
        address: "192.0.2.99",
        avg_ms: 0,
        min_ms: 0,
        max_ms: 0,
        count: 0,
        status: "unreachable",
      },
      // unresolved: no address at all.
      {
        nameserver: "ghost.example.com",
        address: "",
        avg_ms: 0,
        min_ms: 0,
        max_ms: 0,
        count: 0,
        status: "unresolved",
      },
    ]));
    render(Results, { props: { publicID: "abc12345", domain: "example.com" } });
    await waitFor(() => expect(screen.getByTestId("nameserver-timings")).toBeTruthy());
    const timings = screen.getByTestId("nameserver-timings");
    timings.open = true;
    await fireEvent(timings, new Event("toggle"));

    const rows = screen.getAllByTestId("nameserver-timing-row");
    expect(rows).toHaveLength(3);
    // Problem rows expose their status via data-status and have a visible
    // badge so operators can see which nameservers are broken.
    const statuses = rows.map((r) => r.getAttribute("data-status"));
    expect(statuses).toEqual(expect.arrayContaining(["ok", "unreachable", "unresolved"]));
    expect(screen.getByText("No response")).toBeTruthy();
    expect(screen.getByText("Does not resolve")).toBeTruthy();
    // Unreachable row shows ∞ in timing cells; unresolved shows - and no address.
    expect(screen.getAllByText("∞").length).toBeGreaterThanOrEqual(3);
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
    const mkResp = (msg) =>
      jsonResponse({ job_id: "x", status: "succeeded", raw: { locale: "en", entries: [entry("Module::Alpha", "INFO", msg)] } });
    global.fetch.mockResolvedValueOnce(mkResp("first")).mockResolvedValueOnce(mkResp("second"));
    const { rerender } = render(Results, { props: { publicID: "abc12345", locale: "en" } });
    await waitFor(() => screen.getByTestId("module-group"));

    // Open the module group by setting its open property and firing toggle
    const details = screen.getByTestId("module-group");
    details.open = true;
    await fireEvent(details, new Event("toggle"));

    // Change locale - results re-fetch
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

  it("links a glossary term inside the finding explanation", async () => {
    global.fetch.mockResolvedValue(resultResp([
      { timestamp: 0, module: "DELEGATION", testcase: "delegation01", tag: "NOT_ENOUGH_NS_DEL", level: "WARNING", message: "x" },
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("result-explanation-tag")).toBeTruthy());
    // The tag description mentions "delegation", a glossary term.
    const term = screen.getAllByTestId("glossary-term").find((el) => el.textContent === "delegation");
    expect(term).toBeTruthy();
  });

  it("shows a glossary definition tooltip on hover", async () => {
    global.fetch.mockResolvedValue(resultResp([
      { timestamp: 0, module: "DELEGATION", testcase: "delegation01", tag: "NOT_ENOUGH_NS_DEL", level: "WARNING", message: "x" },
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("result-explanation-tag")).toBeTruthy());
    const term = screen.getAllByTestId("glossary-term").find((el) => el.textContent === "delegation");
    // Tooltip is absent until the term is hovered.
    expect(screen.queryByTestId("glossary-tip")).toBeNull();
    await fireEvent.mouseEnter(term);
    expect(screen.getByTestId("glossary-tip").textContent).toMatch(/pointer in the parent zone/i);
  });

  it("keeps the full explanation text intact when terms are linked", async () => {
    global.fetch.mockResolvedValue(resultResp([
      { timestamp: 0, module: "DELEGATION", testcase: "delegation01", tag: "NOT_ENOUGH_NS_DEL", level: "WARNING", message: "x" },
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("result-explanation-tag")).toBeTruthy());
    // The linkified paragraph reads exactly as authored (no injected tooltip text).
    expect(screen.getByTestId("result-explanation-tag").textContent)
      .toMatch(/parent zone's delegation lists fewer nameservers/i);
  });

  it("re-fetches with new locale when locale prop changes", async () => {
    const enResp = jsonResponse({ job_id: "x", status: "succeeded", raw: { locale: "en", entries: [entry("M", "INFO", "english msg")] } });
    const svResp = jsonResponse({ job_id: "x", status: "succeeded", raw: { locale: "sv", entries: [entry("M", "INFO", "swedish msg")] } });
    global.fetch.mockResolvedValueOnce(enResp).mockResolvedValueOnce(svResp);
    const { rerender } = render(Results, { props: { publicID: "abc12345", locale: "en" } });
    await waitFor(() => expect(screen.getByText("english msg")).toBeTruthy());

    await rerender({ locale: "sv" });
    await waitFor(() => expect(screen.getByText("swedish msg")).toBeTruthy());
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls[1][0]).toContain("locale=sv");
  });

  // Not-a-DNS-zone callout

  it("shows the callout and calls ontestparent with the found parent zone", async () => {
    const ontestparent = vi.fn();
    global.fetch.mockResolvedValue(resultResp([
      taggedEntry("B01_NO_CHILD", { domain_child: "resources.eosc.ch", domain_super: "eosc.ch" }, "ERROR"),
      taggedEntry("B01_PARENT_FOUND", { domain: "eosc.ch" }),
    ]));
    render(Results, { props: { publicID: "abc12345", domain: "resources.eosc.ch", ontestparent } });
    await waitFor(() => expect(screen.getByTestId("no-zone-callout")).toBeTruthy());
    // Body names the tested domain.
    expect(screen.getByTestId("no-zone-callout").textContent).toContain("resources.eosc.ch");
    const btn = screen.getByTestId("no-zone-test-parent");
    expect(btn.textContent).toContain("eosc.ch");
    await fireEvent.click(btn);
    expect(ontestparent).toHaveBeenCalledWith("eosc.ch");
  });

  // A bare unknown label or a bad TLD both report the root as the found
  // parent (verified against the live engine), so the root must be filtered.
  it("shows the callout without a parent button when the found parent is the root", async () => {
    global.fetch.mockResolvedValue(resultResp([
      taggedEntry("B01_NO_CHILD", { domain_child: "skjsfsdkf", domain_super: "." }, "ERROR"),
      taggedEntry("B01_PARENT_FOUND", { domain: "." }),
    ]));
    render(Results, { props: { publicID: "abc12345", domain: "skjsfsdkf" } });
    await waitFor(() => expect(screen.getByTestId("no-zone-callout")).toBeTruthy());
    expect(screen.queryByTestId("no-zone-test-parent")).toBeNull();
  });

  it("shows the callout without a parent button when no parent zone was found at all", async () => {
    global.fetch.mockResolvedValue(resultResp([
      taggedEntry("B01_NO_CHILD", { domain_child: "example.skjsfsdkf", domain_super: "skjsfsdkf" }, "ERROR"),
      taggedEntry("B01_PARENT_NOT_FOUND", {}),
    ]));
    render(Results, { props: { publicID: "abc12345", domain: "example.skjsfsdkf" } });
    await waitFor(() => expect(screen.getByTestId("no-zone-callout")).toBeTruthy());
    expect(screen.queryByTestId("no-zone-test-parent")).toBeNull();
  });

  it("shows the callout without a parent button when the parent is ambiguous", async () => {
    global.fetch.mockResolvedValue(resultResp([
      taggedEntry("B01_NO_CHILD", { domain_child: "x.example", domain_super: "example" }, "ERROR"),
      taggedEntry("B01_PARENT_FOUND", { domain: "a.example" }),
      taggedEntry("B01_PARENT_FOUND", { domain: "b.example" }),
    ]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("no-zone-callout")).toBeTruthy());
    expect(screen.queryByTestId("no-zone-test-parent")).toBeNull();
  });

  it("does not show the callout when there is no B01_NO_CHILD finding", async () => {
    global.fetch.mockResolvedValue(resultResp([entry("Module::A", "INFO")]));
    render(Results, { props: { publicID: "abc12345" } });
    await waitFor(() => expect(screen.getByTestId("result-banner")).toBeTruthy());
    expect(screen.queryByTestId("no-zone-callout")).toBeNull();
  });

  // markerResp adds the has_dnssec_chain field to a plain result payload.
  const markerResp = (hasChain) =>
    jsonResponse({
      job_id: "test-id",
      status: "succeeded",
      raw: { locale: "en", entries: [] },
      nameserver_timings: [],
      testcase_descriptions: {},
      has_dnssec_chain: hasChain,
    });

  it("renders the DNSSEC chain section when enabled and the marker is set", async () => {
    global.fetch.mockResolvedValue(markerResp(true));
    render(Results, { props: { publicID: "abc12345", domain: "example.com", dnssecChainEnabled: true } });
    await waitFor(() => expect(screen.getByTestId("dnssec-chain")).toBeTruthy());
  });

  it("hides the DNSSEC chain section when the marker is absent", async () => {
    global.fetch.mockResolvedValue(markerResp(false));
    render(Results, { props: { publicID: "abc12345", domain: "example.com", dnssecChainEnabled: true } });
    await waitFor(() => expect(screen.getByTestId("result-banner")).toBeTruthy());
    expect(screen.queryByTestId("dnssec-chain")).toBeNull();
  });

  it("hides the DNSSEC chain section when the feature flag is off", async () => {
    global.fetch.mockResolvedValue(markerResp(true));
    render(Results, { props: { publicID: "abc12345", domain: "example.com", dnssecChainEnabled: false } });
    await waitFor(() => expect(screen.getByTestId("result-banner")).toBeTruthy());
    expect(screen.queryByTestId("dnssec-chain")).toBeNull();
  });
});
