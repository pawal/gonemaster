/** Base URL for all public API calls. */
export const API_BASE = "/pub/api/v1";

/**
 * Submit a new test job.
 * @param {string} domain
 * @param {object} opts  Extra fields merged into the request body (e.g.
 *                       ipv4_disabled, ipv6_disabled, nameservers, ds_info).
 * @returns {Promise<Response>}
 */
export async function createJob(domain, opts = {}) {
  return fetch(`${API_BASE}/jobs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ domain, ...opts }),
  });
}

/**
 * Poll the status of a job by its public ID.
 * @param {string} publicID
 * @returns {Promise<Response>}
 */
export async function getJob(publicID) {
  return fetch(`${API_BASE}/jobs/${encodeURIComponent(publicID)}`);
}

/**
 * Fetch the result of a completed job.
 * @param {string} publicID
 * @param {string} locale  BCP-47 locale code, default "en".
 * @returns {Promise<Response>}
 */
export async function getResult(publicID, locale = "en") {
  return fetch(
    `${API_BASE}/jobs/${encodeURIComponent(publicID)}/result?locale=${encodeURIComponent(locale)}`
  );
}

/**
 * Fetch the DNSSEC chain summary for a completed job. Returns the raw Response
 * so the caller can distinguish 404 (no chain data) from a successful payload.
 * The payload is locale-free (structural), so no locale parameter is sent.
 * @param {string} publicID
 * @returns {Promise<Response>}
 */
export async function getDnssecChain(publicID) {
  return fetch(`${API_BASE}/jobs/${encodeURIComponent(publicID)}/dnssec-chain`);
}

/**
 * Fetch the list of available locale codes from the server.
 * @returns {Promise<Response>}
 */
export async function getLocales() {
  return fetch(`${API_BASE}/locales`);
}

/**
 * Look up NS and DS records for a domain from the parent zone.
 * @param {string} domain
 * @returns {Promise<Response>}
 */
export async function lookupDomain(domain) {
  return fetch(`${API_BASE}/lookup/${encodeURIComponent(domain)}`);
}

/**
 * Fetch the server version string.
 * @returns {Promise<Response>}
 */
export async function getVersion() {
  return fetch(`${API_BASE}/version`);
}

/**
 * Fetch public server feature flags (e.g. show_score_public).
 * @returns {Promise<Response>}
 */
export async function getInfo() {
  return fetch(`${API_BASE}/info`);
}
