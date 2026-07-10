import { render, screen, waitFor, fireEvent, cleanup } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import DnssecChain from "./DnssecChain.svelte";

// secureChain is a complete "secure" summary: one DS matching a KSK, a ZSK, and
// a valid DNSKEY signature.
function secureChain() {
  return {
    version: 1,
    zone: "example.com",
    parent_zone: "com",
    delegation: "normal",
    status: "secure",
    parent: {
      ds_source: "parent",
      ds: [{ key_tag: 1000, algorithm: 13, digest_type: 2, digest: "ab", servers: ["192.0.2.1"] }],
      servers_disagreeing: [],
    },
    child: {
      dnskeys: [
        { key_tag: 1000, algorithm: 13, flags: 257, sep: true, servers: ["203.0.113.1"] },
        { key_tag: 2000, algorithm: 13, flags: 256, sep: false, servers: ["203.0.113.1"] },
      ],
      dnskey_rrsig: [{ key_tag: 1000, algorithm: 13, state: "valid", inception: 1700000000, expiration: 1800000000, servers: ["203.0.113.1"] }],
      signed: [],
      servers_disagreeing: [],
    },
    links: [{ ds_key_tag: 1000, dnskey_key_tag: 1000, status: "match", servers: ["203.0.113.1"] }],
  };
}

function jsonResponse(body, status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => body };
}

// openChain flips the <details> open and dispatches toggle, which jsdom does
// not fire on its own.
function openChain(container) {
  const details = container.querySelector("details");
  details.open = true;
  details.dispatchEvent(new Event("toggle"));
  return details;
}

describe("DnssecChain", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("does not fetch before the section is opened", () => {
    render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("fetches exactly once on open and not again on re-toggle", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const { container } = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });

    const details = openChain(container);
    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(fetch).toHaveBeenCalledTimes(1);

    // Close then reopen: must not refetch.
    details.open = false;
    details.dispatchEvent(new Event("toggle"));
    details.open = true;
    details.dispatchEvent(new Event("toggle"));
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("shows a loading note while the request is pending", async () => {
    fetch.mockReturnValue(new Promise(() => {})); // never resolves
    const { container } = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
    openChain(container);
    await waitFor(() => expect(screen.getByTestId("chain-loading")).toBeTruthy());
  });

  it("renders an SVG with the expected nodes on success", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const { container } = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
    openChain(container);

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const svg = screen.getByTestId("chain-svg");
    expect(svg.getAttribute("role")).toBe("img");
    // DS + KSK + ZSK = 3 node groups (no abstract DNSKEY-RRset box).
    expect(container.querySelectorAll("g.chain-node").length).toBe(3);
    expect(container.querySelector("g.node-ksk")).toBeTruthy();
    // The KSK self-signs the DNSKEY RRset: a loop path is drawn.
    expect(container.querySelector("path.chain-edge")).toBeTruthy();
    expect(screen.getByTestId("chain-legend")).toBeTruthy();
    expect(screen.getByTestId("chain-facts")).toBeTruthy();
  });

  it("draws grey reference edges from CDS/CDNSKEY to the named key", async () => {
    const chain = secureChain();
    chain.child.signed = [
      { type: "CDS", rrsig: [{ key_tag: 1000, state: "valid" }], refs: [1000] },
      { type: "CDNSKEY", rrsig: [{ key_tag: 1000, state: "valid" }], refs: [1000] },
    ];
    fetch.mockResolvedValue(jsonResponse(chain));
    const { container } = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
    openChain(container);

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(container.querySelectorAll("path.edge-ref").length).toBe(2);
  });

  it("shows a custom tooltip immediately on hover", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const { container } = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
    openChain(container);

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const ksk = container.querySelector("g.node-ksk");
    await fireEvent.mouseMove(ksk, { clientX: 120, clientY: 120 });

    const tip = container.querySelector(".chain-tip");
    expect(tip.classList.contains("chain-tip-shown")).toBe(true);
    expect(tip.textContent).toContain("KSK");
    expect(tip.textContent).toContain("Algorithm:");
  });

  it("shows the unavailable note on a 404 and never an SVG", async () => {
    fetch.mockResolvedValue(jsonResponse({}, 404));
    const { container } = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
    openChain(container);

    await waitFor(() => expect(screen.getByTestId("chain-empty")).toBeTruthy());
    expect(screen.queryByTestId("chain-svg")).toBeNull();
  });

  it("shows an error and refetches when retry is clicked", async () => {
    fetch.mockRejectedValueOnce(new Error("network"));
    const { container } = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
    openChain(container);

    await waitFor(() => expect(screen.getByTestId("chain-error")).toBeTruthy());
    expect(fetch).toHaveBeenCalledTimes(1);

    fetch.mockResolvedValue(jsonResponse(secureChain()));
    await fireEvent.click(screen.getByTestId("chain-retry"));
    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("renders the unsigned callout without an SVG", async () => {
    fetch.mockResolvedValue(jsonResponse({ version: 1, zone: "example.com", status: "unsigned", parent: {}, child: {}, links: [] }));
    const { container } = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
    openChain(container);

    await waitFor(() => expect(screen.getByTestId("chain-unsigned")).toBeTruthy());
    expect(screen.queryByTestId("chain-svg")).toBeNull();
  });

  it("shows the disagreement note when servers disagree", async () => {
    const chain = secureChain();
    chain.parent.servers_disagreeing = ["192.0.2.2"];
    fetch.mockResolvedValue(jsonResponse(chain));
    const { container } = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
    openChain(container);

    await waitFor(() => expect(screen.getByTestId("chain-disagree")).toBeTruthy());
  });

  it("shows the provided-DS note for undelegated input", async () => {
    const chain = secureChain();
    chain.delegation = "undelegated";
    chain.parent.ds_source = "input";
    chain.parent.ds[0].servers = ["-"];
    fetch.mockResolvedValue(jsonResponse(chain));
    const { container } = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
    openChain(container);

    await waitFor(() => expect(screen.getByTestId("chain-provided-ds")).toBeTruthy());
  });
});
