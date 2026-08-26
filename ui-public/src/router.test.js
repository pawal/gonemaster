import { describe, it, expect, afterEach, beforeEach } from "vitest";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  DEFAULT_BASE,
  base,
  parsePath,
  pathFor,
  navigate,
  parseHash,
  hashFor,
  upgradeLegacyHash,
  isPlainClick,
  readLang,
  writeLang,
} from "./router.js";
import { goTo } from "./test/helpers.js";

describe("base", () => {
  const setStatedBase = (value) => {
    document.head.querySelectorAll('meta[name="gonemaster:base"]').forEach((m) => m.remove());
    if (value === null) return;
    const meta = document.createElement("meta");
    meta.setAttribute("name", "gonemaster:base");
    meta.setAttribute("content", value);
    document.head.appendChild(meta);
  };

  afterEach(() => setStatedBase(null));

  // The fallback is hardcoded, so pin it: a drift here 404s every link.
  it("falls back to the base in vite.config.js when the page states none", () => {
    const here = dirname(fileURLToPath(import.meta.url));
    const config = readFileSync(resolve(here, "..", "vite.config.js"), "utf8");
    const match = config.match(/base:\s*"([^"]+)"/);
    expect(match).not.toBeNull();
    expect(DEFAULT_BASE).toBe(match[1]);
    expect(base()).toBe(match[1]);
  });

  // A proxy can serve the UI at the root, and then every produced link must
  // drop the prefix or it 404s.
  it("honours the base the server states", () => {
    setStatedBase("/");
    expect(base()).toBe("/");
    expect(pathFor("result", "abc12345")).toBe("/result/abc12345");
    expect(parsePath("/result/abc12345")).toEqual({ view: "result", publicID: "abc12345" });
  });

  it("accepts any stated prefix", () => {
    setStatedBase("/dns/check/");
    expect(pathFor("home")).toBe("/dns/check/");
    expect(parsePath("/dns/check/result/xyz")).toEqual({ view: "result", publicID: "xyz" });
  });

  it.each(["", "public/", "/public", "not-a-path"])(
    "ignores the unusable stated base %j",
    (value) => {
      setStatedBase(value);
      expect(base()).toBe(DEFAULT_BASE);
    },
  );

  it("ignores the unsubstituted placeholder", () => {
    setStatedBase("__UI_BASE__");
    expect(base()).toBe(DEFAULT_BASE);
  });
});

describe("parsePath", () => {
  it.each([
    ["/public/", { view: "home", publicID: null }],
    ["/public/anything-else", { view: "home", publicID: null }],
    ["/public/result", { view: "home", publicID: null }],
    ["/public/result/", { view: "home", publicID: null }],
  ])("routes %j to home", (pathname, expected) => {
    expect(parsePath(pathname)).toEqual(expected);
  });

  it("routes the result path to the result view", () => {
    expect(parsePath("/public/result/abc12345")).toEqual({ view: "result", publicID: "abc12345" });
  });

  it("accepts alphanumeric public IDs of various lengths", () => {
    expect(parsePath("/public/result/Ab3Cd8Ef")).toEqual({ view: "result", publicID: "Ab3Cd8Ef" });
    expect(parsePath("/public/result/x")).toEqual({ view: "result", publicID: "x" });
  });

  it("tolerates a trailing slash on a result path", () => {
    expect(parsePath("/public/result/abc12345/")).toEqual({ view: "result", publicID: "abc12345" });
  });

  it("rejects IDs containing non-alphanumeric characters (routes to home)", () => {
    expect(parsePath("/public/result/abc-123")).toEqual({ view: "home", publicID: null });
    expect(parsePath("/public/result/abc/extra")).toEqual({ view: "home", publicID: null });
  });

  // The app is only ever served under BASE, so anything else is not ours.
  it("routes a path outside BASE to home rather than guessing", () => {
    expect(parsePath("/result/abc12345")).toEqual({ view: "home", publicID: null });
    expect(parsePath("/other/result/abc12345")).toEqual({ view: "home", publicID: null });
    expect(parsePath("/")).toEqual({ view: "home", publicID: null });
  });

  it("survives a missing pathname", () => {
    expect(parsePath(undefined)).toEqual({ view: "home", publicID: null });
  });
});

describe("pathFor", () => {
  it("returns the base for the home view", () => {
    expect(pathFor("home")).toBe("/public/");
    expect(pathFor("home", null)).toBe("/public/");
  });

  it("returns the result path for a result view with a publicID", () => {
    expect(pathFor("result", "abc12345")).toBe("/public/result/abc12345");
  });

  it("falls back to the base for a result view without a publicID", () => {
    expect(pathFor("result", null)).toBe("/public/");
    expect(pathFor("result")).toBe("/public/");
  });

  it("round-trips through parsePath", () => {
    expect(parsePath(pathFor("result", "abc12345"))).toEqual({
      view: "result",
      publicID: "abc12345",
    });
    expect(parsePath(pathFor("home"))).toEqual({ view: "home", publicID: null });
  });
});

describe("navigate", () => {
  beforeEach(() => {
    goTo("/public/");
  });

  it("pushes the result path onto the history stack", () => {
    const before = window.history.length;
    navigate("result", "abc12345");
    expect(window.location.pathname).toBe("/public/result/abc12345");
    expect(window.history.length).toBe(before + 1);
  });

  it("pushes the base path for the home view", () => {
    navigate("result", "abc12345");
    navigate("home");
    expect(window.location.pathname).toBe("/public/");
  });

  // Losing the query here dropped the chosen language on every navigation.
  it("carries the query along", () => {
    goTo("/public/?lang=sv");
    navigate("result", "abc12345");
    expect(window.location.pathname).toBe("/public/result/abc12345");
    expect(readLang()).toBe("sv");

    navigate("home");
    expect(window.location.pathname).toBe("/public/");
    expect(readLang()).toBe("sv");
  });

  it("adds no query when there was none", () => {
    navigate("result", "abc12345");
    expect(window.location.search).toBe("");
  });
});

// Links shared before the move to path routing are hash-form and must resolve.
describe("parseHash", () => {
  it.each([
    ["", { view: "home", publicID: null }],
    ["#", { view: "home", publicID: null }],
    ["#/", { view: "home", publicID: null }],
    ["#/anything-else", { view: "home", publicID: null }],
  ])("routes %j to home", (hash, expected) => {
    expect(parseHash(hash)).toEqual(expected);
  });

  it("routes #/result/:id to result view", () => {
    expect(parseHash("#/result/abc12345")).toEqual({ view: "result", publicID: "abc12345" });
  });

  it("accepts alphanumeric public IDs of various lengths", () => {
    expect(parseHash("#/result/Ab3Cd8Ef")).toEqual({ view: "result", publicID: "Ab3Cd8Ef" });
    expect(parseHash("#/result/x")).toEqual({ view: "result", publicID: "x" });
  });

  it("rejects #/result/ with no ID (routes to home)", () => {
    expect(parseHash("#/result/")).toEqual({ view: "home", publicID: null });
  });

  it("rejects IDs containing non-alphanumeric characters (routes to home)", () => {
    expect(parseHash("#/result/abc-123")).toEqual({ view: "home", publicID: null });
    expect(parseHash("#/result/abc/extra")).toEqual({ view: "home", publicID: null });
  });

  it("works without leading #", () => {
    expect(parseHash("/result/abc12345")).toEqual({ view: "result", publicID: "abc12345" });
  });
});

describe("hashFor", () => {
  it("returns #/ for home view", () => {
    expect(hashFor("home")).toBe("#/");
    expect(hashFor("home", null)).toBe("#/");
  });

  it("returns #/result/:id for result view with a publicID", () => {
    expect(hashFor("result", "abc12345")).toBe("#/result/abc12345");
  });

  it("falls back to #/ for result view without a publicID", () => {
    expect(hashFor("result", null)).toBe("#/");
    expect(hashFor("result")).toBe("#/");
  });
});

describe("upgradeLegacyHash", () => {
  beforeEach(() => {
    goTo("/public/");
  });

  it("rewrites a legacy result link to the path form", () => {
    goTo("/public/#/result/abc12345");
    expect(upgradeLegacyHash()).toBe(true);
    expect(window.location.pathname).toBe("/public/result/abc12345");
    expect(window.location.hash).toBe("");
  });

  it("replaces rather than pushes, so Back does not return to the old link", () => {
    goTo("/public/#/result/abc12345");
    const before = window.history.length;
    upgradeLegacyHash();
    expect(window.history.length).toBe(before);
  });

  it("keeps the query string", () => {
    goTo("/public/?lang=sv#/result/abc12345");
    expect(upgradeLegacyHash()).toBe(true);
    expect(window.location.pathname).toBe("/public/result/abc12345");
    expect(window.location.search).toBe("?lang=sv");
  });

  it("leaves a plain home URL alone", () => {
    goTo("/public/");
    expect(upgradeLegacyHash()).toBe(false);
    expect(window.location.pathname).toBe("/public/");
  });

  it("leaves an unrecognised hash alone", () => {
    goTo("/public/#/something");
    expect(upgradeLegacyHash()).toBe(false);
    expect(window.location.pathname).toBe("/public/");
  });

  it("lets an existing path route win over a stale hash", () => {
    goTo("/public/result/pathwins#/result/hashloses");
    expect(upgradeLegacyHash()).toBe(false);
    expect(window.location.pathname).toBe("/public/result/pathwins");
  });
});

describe("readLang and writeLang", () => {
  beforeEach(() => {
    goTo("/public/");
  });

  it("reads the lang query parameter", () => {
    goTo("/public/?lang=sv");
    expect(readLang()).toBe("sv");
  });

  it("returns an empty string when there is no lang parameter", () => {
    expect(readLang()).toBe("");
    goTo("/public/?other=1");
    expect(readLang()).toBe("");
  });

  it("writes the lang parameter without touching the path", () => {
    goTo("/public/result/abc12345");
    writeLang("da");
    expect(window.location.pathname).toBe("/public/result/abc12345");
    expect(readLang()).toBe("da");
  });

  it("replaces an existing lang rather than appending a second one", () => {
    goTo("/public/?lang=sv");
    writeLang("de");
    expect(window.location.search).toBe("?lang=de");
  });

  it("keeps other query parameters", () => {
    goTo("/public/?keep=1");
    writeLang("fi");
    expect(new URLSearchParams(window.location.search).get("keep")).toBe("1");
    expect(readLang()).toBe("fi");
  });

  it("replaces rather than pushes, so Back does not walk language changes", () => {
    const before = window.history.length;
    writeLang("nb");
    expect(window.history.length).toBe(before);
  });
});

describe("isPlainClick", () => {
  const click = (over = {}) => ({
    button: 0,
    metaKey: false,
    ctrlKey: false,
    shiftKey: false,
    altKey: false,
    ...over,
  });

  it("accepts an unmodified left click", () => {
    expect(isPlainClick(click())).toBe(true);
  });

  it.each([
    ["middle click", { button: 1 }],
    ["right click", { button: 2 }],
    ["ctrl-click", { ctrlKey: true }],
    ["meta-click", { metaKey: true }],
    ["shift-click", { shiftKey: true }],
    ["alt-click", { altKey: true }],
  ])("rejects %s, which the browser must handle itself", (_name, over) => {
    expect(isPlainClick(click(over))).toBe(false);
  });
});
