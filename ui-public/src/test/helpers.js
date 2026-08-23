import { vi } from "vitest";

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
