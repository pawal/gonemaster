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

// GlueNameservers returns nameserver objects (name + address) from the parent's glue.
func GlueNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	return z.Glue(ctx)
}

// ApexNameservers returns nameserver objects resolved from the child zone's apex NS RRset.
func ApexNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	return z.NS(ctx)
}

// AllNSNames returns the deduplicated union of glue names and apex NS names.
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

// AllNameservers returns the deduplicated union of GlueNameservers and ApexNameservers.
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
