// Outbound links to the authoritative reference for an entity. Pure
// builders so the link set is a data table rather than scattered markup.

import { idnToUnicode } from "$lib/idn";

export type ExternalLink = {
  label: string;
  href: string;
  title: string;
};

// Context the server can supply to sharpen a domain's link set.
export type DomainLinkContext = {
  // Direct RDAP endpoint for this domain, when the server resolved one.
  rdapUrl?: string | null;
};

const RIPESTAT = "https://stat.ripe.net/";

// Lowercase and drop one trailing root dot.
function normalizeName(name: string): string {
  const lowered = (name ?? "").trim().toLowerCase();
  return lowered.endsWith(".") ? lowered.slice(0, -1) : lowered;
}

// True for a single-label name. That is all "is a TLD" means here: a
// single-label name that is not delegated in the root yields links that
// 404 at IANA, which is harmless.
export function isTLD(name: string): boolean {
  const normalized = normalizeName(name);
  if (!normalized) return false;
  return !normalized.includes(".");
}

export function domainLinks(domain: string, ctx: DomainLinkContext = {}): ExternalLink[] {
  const name = normalizeName(domain);
  if (!name) return [];
  const ascii = encodeURIComponent(name);
  const out: ExternalLink[] = [];
  const tld = isTLD(name);

  if (tld) {
    out.push({
      label: "IANA root zone",
      href: `https://www.iana.org/domains/root/db/${ascii}.html`,
      title: "Root zone database entry at IANA"
    });
    // ICANNWiki keys its pages on the U-label.
    const unicode = encodeURIComponent(idnToUnicode(name));
    out.push({
      label: "ICANNWiki",
      href: `https://icannwiki.org/.${unicode}`,
      title: "Registry background at ICANNWiki"
    });
  }

  const direct = (ctx.rdapUrl ?? "").trim();
  if (direct) {
    out.push({ label: "RDAP", href: direct, title: "Registration data from the registry (RDAP)" });
  } else if (tld) {
    out.push({
      label: "RDAP",
      href: `https://rdap.iana.org/domain/${ascii}`,
      title: "Registration data from IANA (RDAP)"
    });
  } else {
    out.push({
      label: "RDAP",
      href: `https://client.rdap.org/?type=domain&object=${ascii}`,
      title: "Registration data via the RDAP web client"
    });
  }
  return out;
}

export function asnLinks(asn: number | string): ExternalLink[] {
  const digits = String(asn ?? "")
    .trim()
    .replace(/^as/i, "");
  if (!/^\d+$/.test(digits)) return [];
  return [
    {
      label: "RIPEstat",
      href: `${RIPESTAT}AS${digits}`,
      title: "Routing and registry data at RIPEstat"
    }
  ];
}

export function prefixLinks(prefix: string): ExternalLink[] {
  return ripestatLinks(prefix);
}

export function addressLinks(address: string): ExternalLink[] {
  return ripestatLinks(address);
}

// Nameservers have no institutional reference page today.
export function nameserverLinks(_name: string): ExternalLink[] {
  return [];
}

function ripestatLinks(resource: string): ExternalLink[] {
  const value = (resource ?? "").trim();
  if (!value) return [];
  return [
    {
      label: "RIPEstat",
      href: `${RIPESTAT}${encodeURIComponent(value)}`,
      title: "Routing and registry data at RIPEstat"
    }
  ];
}
