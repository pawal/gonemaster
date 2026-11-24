// Top-level dashboard sections by route ID; "end" limits the active match to the exact route.

import type { RouteId } from "$app/types";

// A route ID without [params], which resolve() takes without a params argument.
export type SectionId = Exclude<RouteId, `${string}[${string}`>;

export type NavItem = {
  href: SectionId;
  label: string;
  end: boolean;
};

export const navItems: NavItem[] = [
  { href: "/", label: "Overview", end: true },
  { href: "/cohorts", label: "Cohorts", end: false },
  { href: "/domains", label: "Domains", end: false },
  { href: "/nameservers", label: "Nameservers", end: false },
  // One unified "Addresses" tab replaces the old Endpoints + Prefixes.
  // The /endpoints route stays so bookmarks keep working; the /prefixes
  // list is gone from the nav (detail pages are still reachable from
  // per-address prefix links).
  { href: "/endpoints", label: "Addresses", end: false },
  { href: "/asns", label: "ASNs", end: false },
  { href: "/tags", label: "Tags", end: false },
  // Trends and diff live alongside the list views - both are cohort-
  // scoped and snapshot-aware. The snapshot-intent batch model makes
  // the time-series dimension meaningful for the first time, so these
  // tabs are first-class navigation rather than buried settings.
  { href: "/trends", label: "Trends", end: false },
  { href: "/diff", label: "Diff", end: false }
];

export function isActive(pathname: string, item: NavItem): boolean {
  // Normalize trailing slash so "/domains/" and "/domains" match the same item.
  const normalized = pathname.replace(/\/+$/, "") || "/";
  const target = item.href.replace(/\/+$/, "") || "/";
  if (item.end) return normalized === target;
  return normalized === target || normalized.startsWith(target + "/");
}

// Hide the "Cohorts" tab when only a single cohort is selectable: the
// chooser dropdown already hides in that case, the cohort detail lives
// on the overview, and a one-item list view adds nothing. The threshold
// matches FilterBar's `cohorts.length > 1` rule so the two collapse in
// lockstep.
export function visibleNavItems(items: NavItem[], cohortCount: number): NavItem[] {
  if (cohortCount > 1) return items;
  return items.filter((i) => i.href !== "/cohorts");
}
