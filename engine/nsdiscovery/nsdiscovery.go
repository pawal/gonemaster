package nsdiscovery

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// GlueNameservers returns the nameservers from the parent's delegation glue
// (name and address). Equivalent to z.Glue(ctx) with a nil-zone guard.
// Returns an error if z is nil; otherwise propagates errors from the
// underlying zone lookup.
func GlueNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	return z.Glue(ctx)
}

// ApexNameservers returns the nameservers resolved from the child zone's
// apex NS RRset. Equivalent to z.NS(ctx) with a nil-zone guard. For
// in-bailiwick names this resolves via the zone's own glue; out-of-
// bailiwick names are resolved via the recursor.
func ApexNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	return z.NS(ctx)
}

// AllNSNames returns the deduplicated union of z.GlueNames and z.ApexNSNames.
// Names are lowercased and sorted lexicographically. If either underlying
// call returns an error, the error is propagated and no result is returned.
func AllNSNames(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	glue, err := z.GlueNames(ctx)
	if err != nil {
		return nil, err
	}
	child, err := z.ApexNSNames(ctx)
	if err != nil {
		return nil, err
	}

	seen := map[string]dnsname.Name{}
	for _, name := range glue {
		seen[strings.ToLower(name.String())] = name
	}
	for _, name := range child {
		seen[strings.ToLower(name.String())] = name
	}

	return sortedNames(seen), nil
}

// AllNameservers returns the deduplicated union of [GlueNameservers] and
// [ApexNameservers] (name+address pairs). The dedup key is the lower-cased
// "name/address" string, so the same name with two different addresses is
// kept as two entries. Output is sorted by that same key.
func AllNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	glue, err := GlueNameservers(ctx, z)
	if err != nil {
		return nil, err
	}
	child, err := ApexNameservers(ctx, z)
	if err != nil {
		return nil, err
	}

	seen := map[string]nameserver.Nameserver{}
	for _, ns := range glue {
		seen[strings.ToLower(ns.String())] = ns
	}
	for _, ns := range child {
		seen[strings.ToLower(ns.String())] = ns
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]nameserver.Nameserver, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out, nil
}

func sortedNames(seen map[string]dnsname.Name) []dnsname.Name {
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]dnsname.Name, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}
