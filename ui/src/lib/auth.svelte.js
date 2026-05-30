let mode = $state("open");
let authenticated = $state(true);

export const auth = {
  get mode() {
    return mode;
  },
  get authenticated() {
    return authenticated;
  },
};

// markUnauthenticated flips to the token-mode login gate (used on a 401).
export function markUnauthenticated() {
  mode = "token";
  authenticated = false;
}

// refreshWhoami probes /whoami and updates the auth state.
export async function refreshWhoami(apiFetch) {
  try {
    const r = await apiFetch("/whoami");
    mode = r?.mode === "token" ? "token" : "open";
    authenticated = r?.authenticated !== false;
  } catch (_) {
    // whoami is exempt; a failure means the server is unreachable. Leave the
    // default (open) so the app renders and surfaces its own errors.
  }
}

// login posts the token to /session; on success the session cookie is set.
export async function login(apiFetch, token) {
  await apiFetch("/session", { method: "POST", body: JSON.stringify({ token }) });
  mode = "token";
  authenticated = true;
}

// logout clears the session cookie.
export async function logout(apiFetch) {
  try {
    await apiFetch("/session", { method: "DELETE" });
  } catch (_) {
    // Best effort; clearing local state below is what matters for the UI.
  }
  authenticated = false;
}
