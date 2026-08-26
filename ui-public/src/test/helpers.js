import { fireEvent } from "@testing-library/svelte";
import { vi } from "vitest";

// Only replaceState can set a path in jsdom, and it persists between tests.
export const goTo = (url) => window.history.replaceState(null, "", url);

// What the browser fires on Back or Forward once the URL has already moved.
export const goBackTo = async (url) => {
  goTo(url);
  await fireEvent(window, new PopStateEvent("popstate"));
};

// Clicks a link, reports whether the component prevented the navigation, and
// stops jsdom from trying to follow the href.
export const clickLink = async (link, init = {}) => {
  let prevented = null;
  const swallow = (e) => {
    prevented = e.defaultPrevented;
    e.preventDefault();
  };
  document.addEventListener("click", swallow);
  await fireEvent(link, new MouseEvent("click", {
    bubbles: true, cancelable: true, button: 0, ...init,
  }));
  document.removeEventListener("click", swallow);
  return prevented;
};

// The modified clicks a link must leave to the browser, so it can open a tab.
export const MODIFIED_CLICKS = [
  ["middle click", { button: 1 }],
  ["ctrl-click", { ctrlKey: true }],
  ["meta-click", { metaKey: true }],
  ["shift-click", { shiftKey: true }],
];

// The public API client reads ok, status, headers.get and json.
export const jsonResponse = (body, status = 200) => ({
  ok: status >= 200 && status < 300,
  status,
  headers: { get: () => null },
  json: async () => body
});

export const errorResponse = (status, body = {}, headers = {}) => ({
  ok: false,
  status,
  headers: { get: (name) => headers[name] ?? null },
  json: async () => body
});

// Installs a global.fetch dispatching by URL substring in declaration order.
export const fetchRouter = (routes, fallback = jsonResponse([])) => {
  global.fetch = vi.fn().mockImplementation(async (url) => {
    for (const [match, response] of routes) {
      if (String(url).includes(match)) return response;
    }
    return fallback;
  });
  return global.fetch;
};

// A complete "secure" chain summary: one DS matching a KSK, a ZSK, and a valid
// DNSKEY signature.
export const secureChain = (overrides = {}) => ({
  version: 1,
  zone: "example.com",
  parent_zone: "com",
  delegation: "normal",
  status: "secure",
  parent: {
    ds_source: "parent",
    ds: [{ key_tag: 1000, algorithm: 13, digest_type: 2, digest: "ab", servers: ["192.0.2.1"] }],
    servers_disagreeing: []
  },
  child: {
    dnskeys: [
      { key_tag: 1000, algorithm: 13, flags: 257, sep: true, servers: ["203.0.113.1"] },
      { key_tag: 2000, algorithm: 13, flags: 256, sep: false, servers: ["203.0.113.1"] }
    ],
    dnskey_rrsig: [
      {
        key_tag: 1000,
        algorithm: 13,
        state: "valid",
        inception: 1700000000,
        expiration: 1800000000,
        servers: ["203.0.113.1"]
      }
    ],
    signed: [],
    servers_disagreeing: []
  },
  links: [{ ds_key_tag: 1000, dnskey_key_tag: 1000, status: "match", servers: ["203.0.113.1"] }],
  ...overrides
});
