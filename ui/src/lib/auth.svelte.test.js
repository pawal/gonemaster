import { describe, it, expect } from "vitest";
import { auth, refreshWhoami, login, logout, markUnauthenticated } from "./auth.svelte.js";

describe("auth store", () => {
  it("starts in open mode, authenticated", () => {
    expect(auth.mode).toBe("open");
    expect(auth.authenticated).toBe(true);
  });

  it("refreshWhoami applies token mode and unauthenticated state", async () => {
    await refreshWhoami(async () => ({ mode: "token", authenticated: false }));
    expect(auth.mode).toBe("token");
    expect(auth.authenticated).toBe(false);
  });

  it("login posts the token to /session and authenticates", async () => {
    let seen = null;
    await login(async (path, opts) => {
      seen = { path, opts };
      return {};
    }, "gm_abc");
    expect(seen.path).toBe("/session");
    expect(seen.opts.method).toBe("POST");
    expect(JSON.parse(seen.opts.body).token).toBe("gm_abc");
    expect(auth.mode).toBe("token");
    expect(auth.authenticated).toBe(true);
  });

  it("markUnauthenticated flips to the login gate", () => {
    markUnauthenticated();
    expect(auth.mode).toBe("token");
    expect(auth.authenticated).toBe(false);
  });

  it("logout clears the authenticated flag", async () => {
    await login(async () => ({}), "gm_abc");
    await logout(async () => ({}));
    expect(auth.authenticated).toBe(false);
  });
});
