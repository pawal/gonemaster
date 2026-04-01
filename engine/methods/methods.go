package methods

import (
	"context"
	"fmt"
	"sort"
	"strings"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// Method1 returns the parent zone.
func Method1(ctx context.Context, z *zone.Zone) (*zone.Zone, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	return z.Parent(ctx)
}

// Method2 returns glue names for the zone.
func Method2(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	return z.GlueNames(ctx)
}

// Method3 returns nameserver names found in the zone apex.
func Method3(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}

	responses, err := z.QueryAll(ctx, z.Name.String(), "NS", nil)
	if err != nil {
		return nil, err
	}

	seen := map[string]dnsname.Name{}
	for _, resp := range responses {
		if resp.Msg == nil {
			continue
		}
		for _, rr := range resp.GetRecordsForName("NS", z.Name) {
			nsRR, ok := rr.(*dns.NS)
			if !ok {
				continue
			}
			name := dnsname.New(strings.ToLower(nsRR.Ns))
			seen[name.String()] = name
		}
	}

	return sortedNames(seen), nil
}

// Method4 returns glue nameserver objects for the zone.
func Method4(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	return z.Glue(ctx)
}

// Method5 returns nameserver objects for the zone.
func Method5(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	if z == nil {
		return nil, fmt.Errorf("zone is nil")
	}
	return z.NS(ctx)
}

// Method2and3 returns the union of Method2 and Method3 results.
func Method2and3(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	glue, err := Method2(ctx, z)
	if err != nil {
		return nil, err
	}
	child, err := Method3(ctx, z)
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

// Method4and5 returns the union of Method4 and Method5 results.
func Method4and5(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	glue, err := Method4(ctx, z)
	if err != nil {
		return nil, err
	}
	child, err := Method5(ctx, z)
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
