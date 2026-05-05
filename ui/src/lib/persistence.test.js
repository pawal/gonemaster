import { describe, it, expect } from "vitest";
import {
  persistedStateKey,
  persistedQueryKeys,
  normalizePageSize,
  normalizeCursor,
  hasPersistedURLState,
  decodeStateFromURL,
  decodeStateFromStorage,
  encodeStateToURLParams,
  serializeStateForStorage,
} from "./persistence.js";

const makeValidators = () => ({
  isKnownJobSort: (v) =>
    ["started_at_desc", "started_at_asc", "domain_asc", "domain_desc"].includes(v),
  isKnownSeverityFilter: (v) => ["all", "warnings_plus", "errors_only"].includes(v),
  isKnownBatchStatus: (v) =>
    ["", "queued", "running", "succeeded", "failed", "canceled", "expired", "paused"].includes(v),
  isKnownBatchSort: (v) =>
    ["started_at_desc", "started_at_asc", "created_at_desc"].includes(v),
  normalizeRecentPageSize: (v) => normalizePageSize(v, [10, 20, 50, 100], 20),
  normalizeBatchPageSize: (v) => normalizePageSize(v, [10, 20, 50, 100], 20),
});

describe("constants", () => {
  it("exposes a versioned localStorage key", () => {
    expect(persistedStateKey).toBe("gonemaster.ui.state.v1");
  });

  it("lists all persisted URL query keys", () => {
    expect(persistedQueryKeys).toEqual(
      ["r_sort", "r_sev", "r_batch", "r_domain", "r_limit", "r_cursor",
       "b_id", "b_sort", "b_limit", "b_cursor", "b_status", "b_domain"],
    );
  });
});

describe("normalizePageSize", () => {
  it("returns the value when it is one of the allowed sizes", () => {
    expect(normalizePageSize(50, [10, 20, 50, 100])).toBe(50);
  });

  it("returns the default when value is missing or not allowed", () => {
    expect(normalizePageSize(NaN, [10, 20, 50, 100])).toBe(20);
    expect(normalizePageSize(7, [10, 20, 50, 100])).toBe(20);
    expect(normalizePageSize(undefined, [10, 20, 50, 100])).toBe(20);
  });

  it("respects an explicit default", () => {
    expect(normalizePageSize(7, [10, 20, 50, 100], 50)).toBe(50);
  });

  it("coerces numeric strings", () => {
    expect(normalizePageSize("50", [10, 20, 50, 100])).toBe(50);
  });
});

describe("normalizeCursor", () => {
  it("returns 0 for missing or negative values", () => {
    expect(normalizeCursor(undefined)).toBe(0);
    expect(normalizeCursor(-1)).toBe(0);
    expect(normalizeCursor("not-a-number")).toBe(0);
  });

  it("floors finite non-negative values", () => {
    expect(normalizeCursor(5)).toBe(5);
    expect(normalizeCursor(7.9)).toBe(7);
    expect(normalizeCursor("12")).toBe(12);
  });
});

describe("hasPersistedURLState", () => {
  it("returns true if any persisted key is present", () => {
    expect(hasPersistedURLState(new URLSearchParams("r_sort=domain_asc"))).toBe(true);
    expect(hasPersistedURLState(new URLSearchParams("b_id=abc"))).toBe(true);
  });

  it("returns false otherwise", () => {
    expect(hasPersistedURLState(new URLSearchParams(""))).toBe(false);
    expect(hasPersistedURLState(new URLSearchParams("foo=bar"))).toBe(false);
  });
});

describe("decodeStateFromURL", () => {
  const validators = makeValidators();

  it("returns null when no persisted keys are present", () => {
    expect(decodeStateFromURL(new URLSearchParams("foo=bar"), validators)).toBeNull();
  });

  it("decodes a typical query string", () => {
    const params = new URLSearchParams(
      "r_sort=domain_asc&r_sev=errors_only&r_batch=batch1&r_domain=example.com&r_limit=50&r_cursor=10&b_id=b2&b_sort=started_at_asc&b_limit=10&b_cursor=5&b_status=running&b_domain=foo",
    );
    expect(decodeStateFromURL(params, validators)).toEqual({
      jobSort: "domain_asc",
      severityFilter: "errors_only",
      jobBatchFilter: "batch1",
      recentDomainFilter: "example.com",
      recentPageSize: 50,
      recentCursor: 10,
      selectedBatchId: "b2",
      batchSort: "started_at_asc",
      batchPageSize: 10,
      batchCursor: 5,
      batchStatusFilter: "running",
      batchDomainFilter: "foo",
    });
  });

  it("ignores unknown sort values", () => {
    const params = new URLSearchParams("r_sort=nonsense&r_sev=all");
    const out = decodeStateFromURL(params, validators);
    expect(out.jobSort).toBeUndefined();
    expect(out.severityFilter).toBe("all");
  });

  it("trims whitespace on text filters", () => {
    const params = new URLSearchParams();
    params.set("r_domain", "  example.com  ");
    params.set("b_id", "  bid  ");
    const out = decodeStateFromURL(params, validators);
    expect(out.recentDomainFilter).toBe("example.com");
    expect(out.selectedBatchId).toBe("bid");
  });

  it("falls back to defaults for unparseable numeric values", () => {
    const params = new URLSearchParams("r_limit=junk&r_cursor=junk");
    const out = decodeStateFromURL(params, validators);
    expect(out.recentPageSize).toBe(20);
    expect(out.recentCursor).toBe(0);
  });
});

describe("decodeStateFromStorage", () => {
  const validators = makeValidators();

  it("returns null for empty or invalid input", () => {
    expect(decodeStateFromStorage(null, validators)).toBeNull();
    expect(decodeStateFromStorage("", validators)).toBeNull();
    expect(decodeStateFromStorage("not json", validators)).toBeNull();
    expect(decodeStateFromStorage("123", validators)).toBeNull();
  });

  it("decodes a stored JSON state", () => {
    const raw = JSON.stringify({
      jobSort: "domain_asc",
      severityFilter: "warnings_plus",
      jobBatchFilter: "  b1  ",
      recentDomainFilter: "  example.com  ",
      recentPageSize: 50,
      recentCursor: 3,
      selectedBatchId: "  bid  ",
      batchSort: "created_at_desc",
      batchPageSize: 10,
      batchCursor: 1,
      batchStatusFilter: "running",
      batchDomainFilter: "  foo  ",
    });
    expect(decodeStateFromStorage(raw, validators)).toEqual({
      jobSort: "domain_asc",
      severityFilter: "warnings_plus",
      jobBatchFilter: "b1",
      recentDomainFilter: "example.com",
      recentPageSize: 50,
      recentCursor: 3,
      selectedBatchId: "bid",
      batchSort: "created_at_desc",
      batchPageSize: 10,
      batchCursor: 1,
      batchStatusFilter: "running",
      batchDomainFilter: "foo",
    });
  });

  it("normalizes missing page sizes and cursor to defaults", () => {
    const out = decodeStateFromStorage(JSON.stringify({}), validators);
    expect(out.recentPageSize).toBe(20);
    expect(out.recentCursor).toBe(0);
    expect(out.batchPageSize).toBe(20);
    expect(out.batchCursor).toBe(0);
  });
});

describe("encodeStateToURLParams", () => {
  it("clears persisted keys and only sets non-default values", () => {
    const params = new URLSearchParams("foo=bar&r_sort=stale");
    const state = {
      jobSort: "started_at_desc",
      severityFilter: "all",
      jobBatchFilter: "",
      recentDomainFilter: "",
      recentPageSize: 20,
      recentCursor: 0,
      selectedBatchId: "",
      batchSort: "started_at_desc",
      batchPageSize: 20,
      batchCursor: 0,
      batchStatusFilter: "",
      batchDomainFilter: "",
    };
    const out = encodeStateToURLParams(params, state);
    expect(out.toString()).toBe("foo=bar");
  });

  it("emits keys whose values differ from defaults", () => {
    const state = {
      jobSort: "domain_asc",
      severityFilter: "errors_only",
      jobBatchFilter: "b1",
      recentDomainFilter: "example.com",
      recentPageSize: 50,
      recentCursor: 10,
      selectedBatchId: "bid",
      batchSort: "started_at_asc",
      batchPageSize: 10,
      batchCursor: 5,
      batchStatusFilter: "running",
      batchDomainFilter: "foo",
    };
    const out = encodeStateToURLParams(new URLSearchParams(), state);
    const parsed = new URLSearchParams(out.toString());
    expect(parsed.get("r_sort")).toBe("domain_asc");
    expect(parsed.get("r_sev")).toBe("errors_only");
    expect(parsed.get("r_batch")).toBe("b1");
    expect(parsed.get("r_domain")).toBe("example.com");
    expect(parsed.get("r_limit")).toBe("50");
    expect(parsed.get("r_cursor")).toBe("10");
    expect(parsed.get("b_id")).toBe("bid");
    expect(parsed.get("b_sort")).toBe("started_at_asc");
    expect(parsed.get("b_limit")).toBe("10");
    expect(parsed.get("b_cursor")).toBe("5");
    expect(parsed.get("b_status")).toBe("running");
    expect(parsed.get("b_domain")).toBe("foo");
  });

  it("preserves unrelated query parameters", () => {
    const params = new URLSearchParams("foo=bar&baz=qux");
    const state = {
      jobSort: "domain_asc",
      severityFilter: "all",
      jobBatchFilter: "",
      recentDomainFilter: "",
      recentPageSize: 20,
      recentCursor: 0,
      selectedBatchId: "",
      batchSort: "started_at_desc",
      batchPageSize: 20,
      batchCursor: 0,
      batchStatusFilter: "",
      batchDomainFilter: "",
    };
    const out = encodeStateToURLParams(params, state);
    expect(out.get("foo")).toBe("bar");
    expect(out.get("baz")).toBe("qux");
    expect(out.get("r_sort")).toBe("domain_asc");
  });
});

describe("serializeStateForStorage", () => {
  it("JSON-stringifies the state", () => {
    expect(serializeStateForStorage({ a: 1, b: "x" })).toBe('{"a":1,"b":"x"}');
  });
});

describe("URL <-> state round-trip", () => {
  const validators = makeValidators();
  const baseState = {
    jobSort: "started_at_desc",
    severityFilter: "all",
    jobBatchFilter: "",
    recentDomainFilter: "",
    recentPageSize: 20,
    recentCursor: 0,
    selectedBatchId: "",
    batchSort: "started_at_desc",
    batchPageSize: 20,
    batchCursor: 0,
    batchStatusFilter: "",
    batchDomainFilter: "",
  };

  it("decodes what it encodes", () => {
    const original = {
      ...baseState,
      jobSort: "domain_asc",
      severityFilter: "errors_only",
      recentDomainFilter: "example.com",
      recentPageSize: 50,
      recentCursor: 7,
      selectedBatchId: "bid",
    };
    const params = encodeStateToURLParams(new URLSearchParams(), original);
    const decoded = decodeStateFromURL(params, validators);
    expect(decoded.jobSort).toBe("domain_asc");
    expect(decoded.severityFilter).toBe("errors_only");
    expect(decoded.recentDomainFilter).toBe("example.com");
    expect(decoded.recentPageSize).toBe(50);
    expect(decoded.recentCursor).toBe(7);
    expect(decoded.selectedBatchId).toBe("bid");
  });
});
