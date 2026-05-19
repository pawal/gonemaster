// Package nsdiscovery resolves the nameservers that serve a DNS zone.
//
// DNS gives several possible answers to the question "which nameservers
// serve zone X" and they can disagree. This package distinguishes the
// views that matter for testing:
//
//   - Glue: NS names and addresses from the parent's delegation (what the
//     parent says serves the zone). Exposed as [GlueNameservers] and via
//     the Zone method z.GlueNames(ctx).
//   - Apex: NS records returned by the zone's own authoritative servers
//     (what the zone claims about itself). Exposed as [ApexNameservers]
//     and via z.ApexNSNames(ctx).
//   - Union: both sets combined and deduplicated. Exposed as [AllNSNames]
//     for names only and [AllNameservers] for name+address tuples.
//   - Delegation: parent-queried view with bailiwick-aware address
//     resolution. Exposed as [DelegationNameservers], returning [NSItem].
//   - Zone (apex): authoritative AA NS at the apex, merging in-bailiwick
//     and out-of-bailiwick address resolution. Exposed as [ZoneNameservers].
//   - Parent chain: parent zone's own nameservers, found by walking the
//     delegation chain from the root. Exposed as [ParentNameservers],
//     backed by a package-global cache that [ClearParentNSCache] resets.
//
// Glue/Apex/Union are appropriate for testcases that should trust the
// zone's own view; Delegation/Zone/Parent are for testcases that need
// the authoritative parent-queried view (e.g. for delegation consistency
// checks).
package nsdiscovery
