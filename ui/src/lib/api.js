export async function apiCall(apiBase, path, options = {}) {
  const url = path?.startsWith("/") ? `${apiBase}${path}` : `${apiBase}/${path}`;
  const headers = { ...(options.headers || {}) };
  if (options.body && !headers["Content-Type"]) {
    headers["Content-Type"] = "application/json";
  }
  const response = await fetch(url, { ...options, headers });
  const contentType = response.headers.get("content-type") || "";
  const payload = contentType.includes("application/json")
    ? await response.json()
    : await response.text();
  if (!response.ok) {
    const message = payload?.error?.message || payload?.message || response.statusText;
    throw new Error(message);
  }
  return payload;
}
