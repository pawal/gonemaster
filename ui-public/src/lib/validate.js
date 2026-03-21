/**
 * Validate a domain name entered by the user.
 * Returns a i18n key if invalid, or null if the value looks acceptable.
 * Full normalization and authoritative validation is done server-side.
 */
export function validateDomain(domain) {
  if (!domain || !domain.trim()) return "pub.error_domain_required";
  const trimmed = domain.trim();
  if (/\s/.test(trimmed)) return "pub.error_domain_invalid";
  if (trimmed.length > 253) return "pub.error_domain_invalid";
  // Must contain at least one character that isn't just dots/hyphens
  if (/^[\.\-]+$/.test(trimmed)) return "pub.error_domain_invalid";
  return null;
}

/** Return the initial empty NS row object. */
export function emptyNsRow() {
  return { ns: "", ip: "" };
}

/** Return the initial empty DS row object. */
export function emptyDsRow() {
  return { keytag: "", algorithm: "", digtype: "", digest: "" };
}

/**
 * Build the extra opts object for createJob from the form state.
 * Returns only the fields that are actually set.
 */
export function buildJobOpts(ipMode, nsRows, dsRows) {
  const opts = {};
  if (ipMode === "disable_ipv4") opts.ipv4_disabled = true;
  if (ipMode === "disable_ipv6") opts.ipv6_disabled = true;

  const nameservers = nsRows
    .filter((r) => r.ns.trim())
    .map((r) => (r.ip.trim() ? { ns: r.ns.trim(), ip: r.ip.trim() } : { ns: r.ns.trim() }));
  if (nameservers.length) opts.nameservers = nameservers;

  const ds_info = dsRows
    .filter((r) => r.keytag && r.algorithm && r.digtype && r.digest)
    .map((r) => ({
      keytag: Number(r.keytag),
      algorithm: Number(r.algorithm),
      digtype: Number(r.digtype),
      digest: r.digest.trim(),
    }));
  if (ds_info.length) opts.ds_info = ds_info;

  return opts;
}
