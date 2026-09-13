import { render, screen, waitFor, fireEvent, cleanup } from "@testing-library/svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import DnssecChain from "./DnssecChain.svelte";
import { jsonResponse, secureChain } from "../test/helpers.js";
import { locale, setCatalog } from "../i18n.js";
import de from "../i18n/de.json";

// openChain flips the <details> open and dispatches toggle, which jsdom does
// not fire on its own.
function openChain(container) {
  const details = container.querySelector("details");
  details.open = true;
  details.dispatchEvent(new Event("toggle"));
  return details;
}

// Renders the section already opened, which is when it fetches.
function renderOpened() {
  const { container } = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
  openChain(container);
  return container;
}

describe("DnssecChain", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
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
    const container = renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-loading")).toBeTruthy());
  });

  it("renders an SVG with the expected nodes on success", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const svg = screen.getByTestId("chain-svg");
    expect(svg.getAttribute("role")).toBe("img");
    // DS + KSK + ZSK = 3 node groups (no abstract DNSKEY-RRset box).
    expect(container.querySelectorAll("g.chain-node").length).toBe(3);
    expect(container.querySelector("g.node-ksk")).toBeTruthy();
    // The KSK self-signs the DNSKEY RRset: a loop path is drawn.
    expect(container.querySelector("path.chain-edge")).toBeTruthy();
    expect(screen.getByTestId("chain-legend")).toBeTruthy();
    // Facts use IANA mnemonics, not raw algorithm/digest numbers.
    const facts = screen.getByTestId("chain-facts").textContent;
    expect(facts).toContain("ECDSAP256SHA256");
    expect(facts).toContain("SHA-256");
    expect(facts).not.toContain("(13/2)");
  });

  it("wraps a status too long for its box onto a second line", async () => {
    setCatalog("de", de);
    locale.set("de");
    try {
      fetch.mockResolvedValue(
        jsonResponse(
          secureChain({
            version: 3,
            ns_names: [{ name: "ns1.example.com", status: "chain_broken", signer: "example.com", servers: ["192.0.2.1"] }],
          })
        )
      );
      const container = renderOpened();
      await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
      const face = [...container.querySelectorAll("g.node-nsname text")].map((el) => el.textContent);
      expect(face).toEqual(["ns1", "Vertrauenskette", "unterbrochen"]);
    } finally {
      locale.set("en");
    }
  });

  it("draws the row labels after every edge so none cuts the text", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const kids = [...screen.getByTestId("chain-svg").children];
    const lastEdge = kids.findLastIndex((el) => el.classList.contains("chain-edge"));
    const firstLabel = kids.findIndex((el) => el.classList.contains("chain-cluster-label"));
    expect(lastEdge).toBeGreaterThan(-1);
    expect(firstLabel).toBeGreaterThan(lastEdge);
  });

  it("draws grey reference edges from CDS/CDNSKEY to the named key", async () => {
    const chain = secureChain();
    chain.child.signed = [
      { type: "CDS", rrsig: [{ key_tag: 1000, state: "valid" }], refs: [1000] },
      { type: "CDNSKEY", rrsig: [{ key_tag: 1000, state: "valid" }], refs: [1000] },
    ];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(container.querySelectorAll("path.edge-ref").length).toBe(2);
  });

  it("shows a custom tooltip immediately on hover, localized from tip data", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const ksk = container.querySelector("g.node-ksk");
    await fireEvent.mouseMove(ksk, { clientX: 120, clientY: 120 });

    const tip = container.querySelector(".chain-tip");
    expect(tip.classList.contains("chain-tip-shown")).toBe(true);
    // "Algorithm:" and the mnemonic exist only via i18n + params now.
    expect(tip.textContent).toContain("KSK");
    expect(tip.textContent).toContain("Algorithm: ECDSAP256SHA256");
  });

  // The tip box is measured once per text change, not per pointer move: the
  // per-move read forced a layout and returned the previous text's box,
  // because the DOM updates only after the handler returns.
  it("measures the tooltip once per text change and flips it at the viewport edge", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const container = renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());

    const tip = container.querySelector(".chain-tip");
    const measure = vi.fn(() => ({ width: 200, height: 80, x: 0, y: 0, top: 0, left: 0, right: 0, bottom: 0 }));
    tip.getBoundingClientRect = measure;

    const ksk = container.querySelector("g.node-ksk");
    await fireEvent.mouseMove(ksk, { clientX: 100, clientY: 100 });
    const afterFirst = measure.mock.calls.length;
    expect(afterFirst).toBeGreaterThan(0);

    // Three more moves over the same node: same text, so no new measurement.
    for (const x of [110, 120, 130]) {
      await fireEvent.mouseMove(ksk, { clientX: x, clientY: 100 });
    }
    expect(measure.mock.calls.length).toBe(afterFirst);
    expect(tip.style.left).toBe("144px");

    // Near the right edge the box flips to the left of the pointer, using the
    // width measured for the text now on screen.
    await fireEvent.mouseMove(ksk, { clientX: window.innerWidth - 10, clientY: 100 });
    expect(parseInt(tip.style.left, 10)).toBe(window.innerWidth - 10 - 200 - 14);

    // A different node means different text, and one fresh measurement.
    const zsk = container.querySelector("g.node-zsk");
    await fireEvent.mouseMove(zsk, { clientX: 100, clientY: 100 });
    expect(measure.mock.calls.length).toBe(afterFirst + 1);
  });

  it("renders the parent-zone signing key above the DS", async () => {
    const chain = secureChain();
    chain.parent.dnskeys = [{ key_tag: 5000, algorithm: 13, flags: 256, key_size: 2048, servers: ["192.0.2.1"] }];
    chain.parent.ds_rrsig = [{ key_tag: 5000, algorithm: 13, state: "valid", inception: 100, expiration: 200, servers: ["192.0.2.1"] }];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const pk = container.querySelector("g.node-parent-key");
    expect(pk).toBeTruthy();
    expect(pk.getAttribute("data-tip")).toContain("Parent zone DNSKEY");
  });

  it("shows the record TTL in the DS and key box tooltips", async () => {
    const chain = secureChain();
    chain.parent.ds[0].ttl = 86400;
    chain.child.dnskeys[0].ttl = 3600;
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(container.querySelector("g.node-ds").getAttribute("data-tip")).toContain("TTL: 86400");
    expect(container.querySelector("g.node-ksk").getAttribute("data-tip")).toContain("TTL: 3600");
  });

  it("shows the DNSKEY RRset signature with validity on every key node", async () => {
    const chain = secureChain();
    chain.child.dnskey_rrsig = [
      { key_tag: 1000, algorithm: 13, state: "valid", inception: 1700000000, expiration: 1800000000, servers: ["203.0.113.1"] },
    ];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    // The ZSK does not sign the DNSKEY RRset, but its box tip still shows the
    // covering signature and window (like the DS and SOA boxes do).
    const zsk = container.querySelector("g.node-zsk");
    const tip = zsk.getAttribute("data-tip");
    expect(tip).toContain("DNSKEY RRset signature (key 1000)");
    expect(tip).toContain("2023-11-14 to 2027-01-15");
  });

  it("localizes signature state and window in edge tooltips", async () => {
    const chain = secureChain();
    chain.child.dnskey_rrsig = [
      { key_tag: 1000, algorithm: 13, state: "expired", inception: 1700000000, expiration: 1750000000, servers: ["203.0.113.1"] },
    ];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const self = container.querySelector("path.chain-edge");
    // The data-tip attribute holds the rendered, localized multi-line string.
    const tip = self.getAttribute("data-tip");
    expect(tip).toContain("Status: expired");
    expect(tip).toContain("2023-11-14 to 2025-06-15");
  });

  it("renders an unverifiable large-RSA-exponent chain as a warn (partial), not a failure", async () => {
    // The .lv shape: DS matches the KSK, but the local verifier cannot check
    // the KSK's RSA exponent, so the DNSKEY signature is unsupported_key. The
    // chain rolls up to partial and the signature edge is amber, not red.
    const chain = secureChain();
    chain.status = "partial";
    chain.child.dnskey_rrsig = [
      { key_tag: 1000, algorithm: 8, state: "unsupported_key", inception: 1700000000, expiration: 1800000000, servers: ["203.0.113.1"] },
    ];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());

    // The heading badge is the amber "partial" tone, not the red "broken" one.
    const badge = screen.getByTestId("chain-status-badge");
    expect(badge.classList.contains("badge-warn")).toBe(true);
    expect(badge.textContent).toBe("Partial");

    // The unverifiable-key callout names the affected key tag.
    const callout = screen.getByTestId("chain-unsupported-key");
    expect(callout.textContent).toContain("1000");

    // The self-signature edge is amber (edge-warn), never edge-bad.
    const self = container.querySelector("path.chain-edge");
    const cls = self.getAttribute("class");
    expect(cls).toContain("edge-warn");
    expect(cls).not.toContain("edge-bad");
    expect(self.getAttribute("data-tip")).toContain("Status: unsupported key exponent");
  });

  it("renders when the parent serves multiple DS RRSIGs with the same key tag and day", async () => {
    // The real .lv shape: root servers re-sign the DS RRset on staggered
    // schedules, so the same signer (key 57780) appears twice, differing only
    // sub-day. The facts list must not derive a duplicate {#each} key from
    // (key_tag, day) or Svelte throws each_key_duplicate and the section hangs.
    const chain = secureChain();
    chain.status = "partial";
    chain.parent.ds_rrsig = [
      { key_tag: 57780, algorithm: 8, state: "valid", inception: 1784044800, expiration: 1785171600, servers: ["192.203.230.10"] },
      { key_tag: 57780, algorithm: 8, state: "valid", inception: 1784052000, expiration: 1785178800, servers: ["198.41.0.4"] },
    ];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    // Both DS RRSIG facts lines render rather than collapsing or crashing.
    expect(screen.getAllByTestId("chain-ds-sig-fact").length).toBe(2);
  });

  it("shows the unavailable note on a 404 and never an SVG", async () => {
    fetch.mockResolvedValue(jsonResponse({}, 404));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-empty")).toBeTruthy());
    expect(screen.queryByTestId("chain-svg")).toBeNull();
  });

  it("shows an error and refetches when retry is clicked", async () => {
    fetch.mockRejectedValueOnce(new Error("network"));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-error")).toBeTruthy());
    expect(fetch).toHaveBeenCalledTimes(1);

    fetch.mockResolvedValue(jsonResponse(secureChain()));
    await fireEvent.click(screen.getByTestId("chain-retry"));
    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("renders the unsigned callout without an SVG", async () => {
    fetch.mockResolvedValue(jsonResponse({ version: 1, zone: "example.com", status: "unsigned", parent: {}, child: {}, links: [] }));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-unsigned")).toBeTruthy());
    expect(screen.queryByTestId("chain-svg")).toBeNull();
  });

  it("shows the disagreement note when servers disagree", async () => {
    const chain = secureChain();
    chain.parent.servers_disagreeing = ["192.0.2.2"];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-disagree")).toBeTruthy());
  });

  it("lists disagreeing and record-less server addresses in the facts", async () => {
    const chain = secureChain();
    chain.parent.servers_disagreeing = ["192.0.2.2"];
    chain.parent.servers_without_ds = ["192.0.2.3"];
    chain.child.servers_without_dnskey = ["203.0.113.9"];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-disagree-servers")).toBeTruthy());
    expect(screen.getByTestId("chain-disagree-servers").textContent).toContain("192.0.2.2");
    expect(screen.getByTestId("chain-servers-without-ds").textContent).toContain("192.0.2.3");
    expect(screen.getByTestId("chain-servers-without-dnskey").textContent).toContain("203.0.113.9");
  });

  it("omits the per-server facts lines when every server agrees", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-facts")).toBeTruthy());
    expect(screen.queryByTestId("chain-disagree-servers")).toBeNull();
    expect(screen.queryByTestId("chain-servers-without-ds")).toBeNull();
    expect(screen.queryByTestId("chain-servers-without-dnskey")).toBeNull();
  });

  it("shows the provided-DS note for undelegated input", async () => {
    const chain = secureChain();
    chain.delegation = "undelegated";
    chain.parent.ds_source = "input";
    chain.parent.ds[0].servers = ["-"];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-provided-ds")).toBeTruthy());
  });

  it("shows the no-DNSKEY callout only with positive server evidence", async () => {
    // DS present and zero keys, but no server answered without keys: the
    // child cache was simply cold, so "the zone serves no DNSKEY" is unproven.
    const cold = secureChain();
    cold.status = "indeterminate";
    cold.child.dnskeys = [];
    cold.child.dnskey_rrsig = [];
    cold.child.servers_without_dnskey = [];
    fetch.mockResolvedValue(jsonResponse(cold));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-indeterminate")).toBeTruthy());
    expect(screen.queryByTestId("chain-no-dnskey")).toBeNull();

    cleanup();
    const proven = secureChain();
    proven.status = "broken";
    proven.child.dnskeys = [];
    proven.child.dnskey_rrsig = [];
    proven.child.servers_without_dnskey = ["203.0.113.1"];
    fetch.mockResolvedValue(jsonResponse(proven));
    const second = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
    openChain(second.container);

    await waitFor(() => expect(screen.getByTestId("chain-no-dnskey")).toBeTruthy());
    expect(screen.queryByTestId("chain-indeterminate")).toBeNull();
  });

  it("includes key sizes in the key facts summary when known", async () => {
    const chain = secureChain();
    chain.child.dnskeys[0].key_size = 2048;
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-facts")).toBeTruthy());
    expect(screen.getByTestId("chain-facts").textContent).toContain("2048 bit");
  });

  it("lists the DNSKEY signature validity window in the facts", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-facts")).toBeTruthy());
    const facts = screen.getByTestId("chain-facts").textContent;
    // inception 1700000000 -> 2023-11-14, expiration 1800000000 -> 2027-01-15.
    expect(facts).toContain("2023-11-14");
    expect(facts).toContain("2027-01-15");
    expect(facts).toContain("RRSIG DNSKEY (1000)");
  });

  it("shows a status badge toned by the roll-up and repeats it in the facts", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const { container } = render(DnssecChain, { props: { publicID: "abc", domain: "example.com" } });
    // No badge before the data loads.
    expect(screen.queryByTestId("chain-status-badge")).toBeNull();
    openChain(container);

    await waitFor(() => expect(screen.getByTestId("chain-status-badge")).toBeTruthy());
    const badge = screen.getByTestId("chain-status-badge");
    expect(badge.textContent).toBe("Secure");
    expect(badge.classList.contains("badge-ok")).toBe(true);
    // The status is repeated as the first facts line.
    expect(screen.getByTestId("chain-status-fact").textContent).toContain("Secure");
  });

  it("tones the badge red for a broken chain", async () => {
    const broken = secureChain();
    broken.status = "broken";
    fetch.mockResolvedValue(jsonResponse(broken));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-status-badge")).toBeTruthy());
    const badge = screen.getByTestId("chain-status-badge");
    expect(badge.textContent).toBe("Broken");
    expect(badge.classList.contains("badge-bad")).toBe(true);
  });

  it("tones the badge neutral for an unsigned zone", async () => {
    fetch.mockResolvedValue(jsonResponse({ version: 1, zone: "example.com", status: "unsigned", parent: {}, child: {}, links: [] }));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-status-badge")).toBeTruthy());
    const badge = screen.getByTestId("chain-status-badge");
    expect(badge.textContent).toBe("Unsigned");
    expect(badge.classList.contains("badge-neutral")).toBe(true);
  });

  it("tints the DS node and lists DS signatures when the DS RRSIG is expired", async () => {
    const chain = secureChain();
    chain.parent.ds_rrsig = [
      { key_tag: 5000, state: "expired", inception: 1700000000, expiration: 1750000000 },
    ];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(container.querySelector("g.node-ds.node-sig-bad")).toBeTruthy();
    const facts = screen.getByTestId("chain-facts").textContent;
    expect(facts).toContain("RRSIG DS (5000)");
    expect(facts).toContain("expired");
  });

  it("renders a phantom key node for a DS naming an absent key", async () => {
    // A DS points to keytag 5000, which is not in the DNSKEY RRset. The graph
    // draws a grey phantom key node so the broken DS edge has a target.
    const chain = secureChain();
    chain.parent.ds = [
      { key_tag: 1000, algorithm: 13, digest_type: 2, digest: "ab", servers: ["192.0.2.1"] },
      { key_tag: 5000, algorithm: 13, digest_type: 2, digest: "cd", servers: ["192.0.2.1"] },
    ];
    chain.links = [
      { ds_key_tag: 1000, ds_digest_type: 2, dnskey_key_tag: 1000, status: "match", servers: ["203.0.113.1"] },
      { ds_key_tag: 5000, ds_digest_type: 2, status: "no_dnskey", servers: ["192.0.2.1"] },
    ];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const phantom = container.querySelector("g.node-key-phantom");
    expect(phantom).toBeTruthy();
    expect(phantom.textContent).toContain("5000");
  });

  it("flags an unanchored KSK as a rollover, even without CDS/CDNSKEY", async () => {
    // A double-signature KSK rollover managed with manual DS updates: KSK 1000
    // is anchored, KSK 3000 signs but has no DS, and there are no CDS records.
    const chain = secureChain();
    chain.child.dnskeys = [
      { key_tag: 1000, algorithm: 8, flags: 257, sep: true, anchored: true, key_size: 2048, servers: ["203.0.113.1"] },
      { key_tag: 3000, algorithm: 8, flags: 257, sep: true, key_size: 2048, servers: ["203.0.113.1"] },
      { key_tag: 2000, algorithm: 8, flags: 256, sep: false, servers: ["203.0.113.1"] },
    ];
    chain.child.dnskey_rrsig = [
      { key_tag: 1000, algorithm: 8, state: "valid" },
      { key_tag: 3000, algorithm: 8, state: "valid" },
    ];
    chain.child.signed = [];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-rollover")).toBeTruthy());
    expect(screen.getByTestId("chain-rollover").textContent).toContain("3000");
    // The incoming KSK node is marked and its edges de-emphasized.
    expect(container.querySelector("g.node-incoming")).toBeTruthy();
    expect(container.querySelector("path.edge-incoming")).toBeTruthy();
  });

  it("does not flag a rollover for a plain single-KSK secure zone", async () => {
    const chain = secureChain();
    chain.child.dnskeys[0].anchored = true;
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(screen.queryByTestId("chain-rollover")).toBeNull();
    expect(container.querySelector("g.node-incoming")).toBeNull();
  });

  it("shows a rollover callout and marks the CDS node when CDS/CDNSKEY signal a key change", async () => {
    const chain = secureChain();
    chain.child.dnskeys.push({ key_tag: 3000, algorithm: 13, flags: 257, sep: true, servers: ["203.0.113.1"] });
    chain.child.signed = [
      { type: "CDS", rrsig: [{ key_tag: 1000, state: "valid" }], refs: [1000, 3000], ds_match: "rollover", new_keys: [3000] },
      { type: "CDNSKEY", rrsig: [{ key_tag: 1000, state: "valid" }], refs: [1000, 3000], ds_match: "rollover", new_keys: [3000] },
    ];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-rollover")).toBeTruthy());
    // The incoming key tag is named in the callout.
    expect(screen.getByTestId("chain-rollover").textContent).toContain("3000");
    // The CDS node is toned as a rollover, and a pending ref edge is drawn.
    expect(container.querySelector("g.node-rollover")).toBeTruthy();
    expect(container.querySelector("path.edge-ref-pending")).toBeTruthy();
  });

  it("shows no rollover callout when CDS matches the parent DS", async () => {
    const chain = secureChain();
    chain.child.signed = [
      { type: "CDS", rrsig: [{ key_tag: 1000, state: "valid" }], refs: [1000], ds_match: "match" },
    ];
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(screen.queryByTestId("chain-rollover")).toBeNull();
    expect(container.querySelector("g.node-rollover")).toBeNull();
  });

  it("shows a truncation callout when the summary was capped", async () => {
    const chain = secureChain();
    chain.truncated = true;
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-truncated")).toBeTruthy());
  });

  it("omits the truncation callout when nothing was capped", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(screen.queryByTestId("chain-truncated")).toBeNull();
  });

  it("marks a revoked key on the node, legend, and facts", async () => {
    const chain = secureChain();
    chain.child.dnskeys[0].revoked = true;
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(container.querySelector("g.node-revoked")).toBeTruthy();
    expect(screen.getByTestId("chain-legend-revoked")).toBeTruthy();
    expect(screen.getByTestId("chain-facts").textContent).toContain("revoked");
  });

  it("omits the revoked legend item when no key is revoked", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(container.querySelector("g.node-revoked")).toBeNull();
    expect(screen.queryByTestId("chain-legend-revoked")).toBeNull();
  });

  // A lagging secondary: the validated path holds through the fresh servers,
  // so the roll-up is partial and the stale servers are named.
  it("warns about stale secondaries and tones the partial badge amber", async () => {
    const chain = secureChain();
    chain.status = "partial";
    chain.child.servers_stale = ["203.0.113.9", "203.0.113.10"];
    chain.child.dnskey_rrsig.push({
      key_tag: 1000, algorithm: 13, state: "expired",
      inception: 1600000000, expiration: 1650000000, servers: ["203.0.113.9"],
    });
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const badge = screen.getByTestId("chain-status-badge");
    expect(badge.textContent).toBe("Partial");
    expect(badge.classList.contains("badge-warn")).toBe(true);
    expect(screen.getByTestId("chain-stale")).toBeTruthy();
    const staleFact = screen.getByTestId("chain-stale-servers").textContent;
    expect(staleFact).toContain("203.0.113.9");
    expect(staleFact).toContain("203.0.113.10");
    // The expired signature colors the self-loop even though a fresh
    // signature by the same key covers the same RRset.
    expect(container.querySelector("path.chain-edge.edge-bad")).toBeTruthy();
  });

  it("merges parent and child stale servers into one callout", async () => {
    const chain = secureChain();
    chain.status = "partial";
    chain.parent.servers_stale = ["192.0.2.1"];
    chain.child.servers_stale = ["203.0.113.9"];
    fetch.mockResolvedValue(jsonResponse(chain));
    renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-stale")).toBeTruthy());
    const staleFact = screen.getByTestId("chain-stale-servers").textContent;
    expect(staleFact).toContain("192.0.2.1");
    expect(staleFact).toContain("203.0.113.9");
  });

  it("omits the stale callout when no server serves an expired signature", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(screen.queryByTestId("chain-stale")).toBeNull();
    expect(screen.queryByTestId("chain-stale-servers")).toBeNull();
  });

  // A newer blob may carry a status this build has no name for; the graph must
  // still render and the badge must not leak the raw token.
  it("renders neutrally for an unknown future status", async () => {
    const chain = secureChain();
    chain.version = 3;
    chain.status = "quantum_broken";
    fetch.mockResolvedValue(jsonResponse(chain));
    renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(screen.queryByTestId("chain-status-badge")).toBeNull();
    expect(screen.queryByTestId("chain-status-fact")).toBeNull();
    expect(screen.getByTestId("chain-facts").textContent).not.toContain("quantum_broken");
  });
});

describe("DnssecChain node faces", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  it("shows the algorithm and key size on the key and DS nodes", async () => {
    const chain = secureChain();
    chain.child.dnskeys[0].key_size = 256;
    chain.child.dnskeys[1].algorithm = 8;
    chain.child.dnskeys[1].key_size = 2048;
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const ksk = container.querySelector("g.node-ksk").textContent;
    expect(ksk).toContain("KSK");
    expect(ksk).toContain("tag 1000");
    expect(ksk).toContain("ECDSAP256SHA256");
    expect(ksk).toContain("256 bit");

    expect(container.querySelector("g.node-zsk").textContent).toContain("RSASHA256");
    // The DS face carries the algorithm it must share with the key it anchors.
    expect(container.querySelector("g.node-ds").textContent).toContain("ECDSAP256SHA256");
  });

  it("omits the size line when the key size is unknown", async () => {
    // The fixture carries no key_size, so no node may claim one.
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const ksk = container.querySelector("g.node-ksk");
    expect(ksk.textContent).toContain("ECDSAP256SHA256");
    expect(ksk.textContent).not.toContain("bit");
    expect(ksk.querySelectorAll("text").length).toBe(3);
  });

  // The label block is centred on its ink, not on its baselines, so the space
  // above the heading matches the space below the last line however many lines
  // a node has. Centring on baselines leaves the bottom visibly tight.
  it("centres the label block in the box whatever the line count", async () => {
    // Cap height of the 13px bold heading: where the block's ink starts. The
    // face has no descenders, so its ink ends on the last baseline.
    const capHeight = 9.5;
    const gaps = (g) => {
      const box = g.querySelector("rect");
      const top = Number(box.getAttribute("y"));
      const bottom = top + Number(box.getAttribute("height"));
      const ys = [...g.querySelectorAll("text")].map((t) => Number(t.getAttribute("y")));
      return { above: ys[0] - capHeight - top, below: bottom - ys[ys.length - 1] };
    };

    const chain = secureChain();
    chain.child.dnskeys[0].key_size = 256; // four lines: heading, tag, algorithm, size
    fetch.mockResolvedValue(jsonResponse(chain));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const four = gaps(container.querySelector("g.node-ksk"));
    expect(four.above).toBeCloseTo(four.below, 1);

    // The ZSK carries no key size, so it renders one line shorter.
    const three = gaps(container.querySelector("g.node-zsk"));
    expect(three.above).toBeCloseTo(three.below, 1);
    expect(three.above).toBeGreaterThan(four.above);
  });

  // The face is an addition, not a move: the hover text has to keep every
  // detail it had, including the ones now duplicated on the node.
  it("keeps the full algorithm label in the node tooltip", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const container = renderOpened();

    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    const tip = container.querySelector("g.node-ksk").dataset.tip;
    expect(tip).toContain("ECDSAP256SHA256 (alg 13)");
    expect(tip).toContain("KSK");
  });
});

describe("in-domain nameserver names", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  // The reference case: the zone's own chain is intact, and one of its
  // nameserver names is signed by a zone nothing delegates.
  const orphanChain = () =>
    secureChain({
      version: 3,
      ns_names: [
        { name: "a.ns.example.com", status: "orphan", signer: "a.ns.example.com", servers: ["192.0.2.1"] },
        { name: "b.ns.example.com", status: "validates", signer: "example.com", servers: ["192.0.2.1"] }
      ]
    });

  it("draws the branch with the orphan marked bad and the healthy name ok", async () => {
    fetch.mockResolvedValue(jsonResponse(orphanChain()));
    const container = renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());

    const nodes = [...container.querySelectorAll("g.node-nsname")];
    expect(nodes.length).toBe(2);
    expect(nodes[0].classList.contains("node-ns-bad")).toBe(true);
    expect(nodes[1].classList.contains("node-ns-ok")).toBe(true);
    expect(container.querySelector("g.node-orphan")).toBeTruthy();
  });

  it("names the status and the signer on the node face and in its tip", async () => {
    fetch.mockResolvedValue(jsonResponse(orphanChain()));
    const container = renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());

    const node = container.querySelector("g.node-nsname");
    expect(node.textContent).toContain("a.ns");
    expect(node.textContent).toContain("orphan zone");
    expect(node.dataset.tip).toContain("a.ns.example.com");
    expect(node.dataset.tip).toContain("Address records signed by a.ns.example.com");
  });

  // The zone's own chain is secure and the badge says so; the branch carries
  // the fault. The badge must not be demoted by a fault below the apex.
  it("keeps the secure badge while the branch is bad", async () => {
    fetch.mockResolvedValue(jsonResponse(orphanChain()));
    renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-status-badge")).toBeTruthy());

    const badge = screen.getByTestId("chain-status-badge");
    expect(badge.textContent).toBe("Secure");
    expect(badge.classList.contains("badge-ok")).toBe(true);
  });

  it("adds a legend entry only when the branch is drawn", async () => {
    fetch.mockResolvedValue(jsonResponse(orphanChain()));
    renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-legend-nsname")).toBeTruthy());

    cleanup();
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(screen.queryByTestId("chain-legend-nsname")).toBeNull();
  });

  it("draws nothing new for a document stored without the section", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain()));
    const container = renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());

    expect(container.querySelector("g.node-nsname")).toBeNull();
    expect(container.querySelector("g.node-orphan")).toBeNull();
    expect(container.querySelector("path.chain-stub")).toBeNull();
  });
});

describe("a bogus nameserver name is hard to miss", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  const orphanChain = () =>
    secureChain({
      version: 3,
      ns_names: [
        { name: "a.ns.example.com", status: "orphan", signer: "a.ns.example.com", servers: ["192.0.2.1"] },
        { name: "b.ns.example.com", status: "validates", signer: "ns.example.com", servers: ["192.0.2.1"] }
      ]
    });

  // The graph sits at the bottom of a tall diagram, so the fault also needs the
  // callout channel every other fault on this card uses.
  it("raises a red callout naming the name that does not validate", async () => {
    fetch.mockResolvedValue(jsonResponse(orphanChain()));
    renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-ns-bogus")).toBeTruthy());

    const callout = screen.getByTestId("chain-ns-bogus");
    expect(callout.textContent).toContain("a.ns.example.com");
    expect(callout.textContent).not.toContain("b.ns.example.com");
    expect(callout.classList.contains("callout-bad")).toBe(true);
  });

  it("raises no callout when every name validates", async () => {
    fetch.mockResolvedValue(
      jsonResponse(
        secureChain({
          version: 3,
          ns_names: [{ name: "ns1.example.com", status: "validates", signer: "example.com", servers: ["192.0.2.1"] }]
        })
      )
    );
    renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(screen.queryByTestId("chain-ns-bogus")).toBeNull();
  });

  // An insecure name is accepted by validators, so it is not a callout.
  it("raises no callout for an insecure name", async () => {
    fetch.mockResolvedValue(
      jsonResponse(
        secureChain({
          version: 3,
          ns_names: [{ name: "ns1.example.com", status: "insecure", servers: ["192.0.2.1"] }]
        })
      )
    );
    renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());
    expect(screen.queryByTestId("chain-ns-bogus")).toBeNull();
  });

  // Two boxes reading only "a.ns" and "ns" differ by stroke colour alone; the
  // word is what tells them apart.
  it("says in words what each signer node is", async () => {
    fetch.mockResolvedValue(jsonResponse(orphanChain()));
    const container = renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());

    expect(container.querySelector("g.node-orphan").textContent).toContain("no delegation");
    expect(container.querySelector("g.node-cut").textContent).toContain("delegation");
  });

  it("writes a bad status in the alerting style, not the muted one", async () => {
    fetch.mockResolvedValue(jsonResponse(orphanChain()));
    const container = renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-svg")).toBeTruthy());

    const bad = container.querySelector("g.node-ns-bad");
    const status = [...bad.querySelectorAll("text")].find((t) => t.textContent === "orphan zone");
    expect(status.classList.contains("chain-node-ns-bad")).toBe(true);
    const ok = container.querySelector("g.node-ns-ok");
    const okStatus = [...ok.querySelectorAll("text")].find((t) => t.textContent === "validates");
    expect(okStatus.classList.contains("chain-node-ns-ok")).toBe(true);
  });
});

describe("a zone its parent proves does not exist", () => {
  beforeEach(() => {
    global.fetch = vi.fn();
  });

  const undelegatedChain = () =>
    secureChain({
      version: 3,
      status: "undelegated",
      parent_zone: "ns.example.com",
      parent: { ds_source: "none", ds: [], servers_disagreeing: [] },
      links: []
    });

  // "No DS" reads as a parent that is merely silent. This parent denies the
  // delegation, so the badge must not be neutral.
  it("badges it bad, not the neutral island tone", async () => {
    fetch.mockResolvedValue(jsonResponse(undelegatedChain()));
    renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-status-badge")).toBeTruthy());

    const badge = screen.getByTestId("chain-status-badge");
    expect(badge.textContent).toBe("Not delegated");
    expect(badge.classList.contains("badge-bad")).toBe(true);
  });

  it("names the parent that carries the proof in a callout", async () => {
    fetch.mockResolvedValue(jsonResponse(undelegatedChain()));
    renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-undelegated")).toBeTruthy());

    const callout = screen.getByTestId("chain-undelegated");
    expect(callout.textContent).toContain("ns.example.com");
    expect(callout.classList.contains("callout-bad")).toBe(true);
  });

  it("leaves an ordinary island alone", async () => {
    fetch.mockResolvedValue(jsonResponse(secureChain({ status: "island" })));
    renderOpened();
    await waitFor(() => expect(screen.getByTestId("chain-status-badge")).toBeTruthy());

    expect(screen.getByTestId("chain-status-badge").textContent).toBe("No DS");
    expect(screen.queryByTestId("chain-undelegated")).toBeNull();
  });
});
