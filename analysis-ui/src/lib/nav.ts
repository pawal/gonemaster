// Top-level dashboard sections. Paths are relative to the SvelteKit base
// ("/analysis"), so the browser URLs are "/analysis", "/analysis/domains",
// etc. The "end" flag tells the active-link matcher whether to match only
// the exact path (for the root overview) or any descendant.

export type NavItem = {
  href: string;
  label: string;
  end: boolean;
};

export const navItems: NavItem[] = [
  { href: "/", label: "Overview", end: true },
  { href: "/cohorts", label: "Cohorts", end: false },
  { href: "/domains", label: "Domains", end: false },
  { href: "/nameservers", label: "Nameservers", end: false },
  { href: "/endpoints", label: "Endpoints", end: false },
  { href: "/asns", label: "ASNs", end: false },
  { href: "/prefixes", label: "Prefixes", end: false },
  { href: "/tags", label: "Tags", end: false },
  { href: "/testcases", label: "Testcases", end: false }
];

export function isActive(pathname: string, item: NavItem): boolean {
  // Normalize trailing slash so "/domains/" and "/domains" match the same item.
  const normalized = pathname.replace(/\/+$/, "") || "/";
  const target = item.href.replace(/\/+$/, "") || "/";
  if (item.end) return normalized === target;
  return normalized === target || normalized.startsWith(target + "/");
}
