// Package constants contains shared constants and embedded reference datasets.
//
// NotQueryableReason is the one address policy for every caller that sends a
// query. After unmapping an IPv4-mapped address it declines:
//
//   - an entry of the IANA IPv4 or IPv6 special-purpose registry not marked
//     globally reachable, the most specific entry deciding; this covers 6to4
//     (2002::/16) and Teredo (2001::/32) whatever IPv4 they embed;
//   - multicast;
//   - the deprecated site-local (fec0::/10) and IPv4-compatible (::/96) ranges;
//   - a NAT64 well-known prefix address (64:ff9b::/96) whose IPv4 it declines.
package constants
