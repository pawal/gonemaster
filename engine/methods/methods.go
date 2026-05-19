package methods

import (
	"context"
	"fmt"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

var errNilZone = fmt.Errorf("zone is nil")

// This package is a transitional forwarder to engine/nsdiscovery.
// New code should import nsdiscovery directly (and use z.GlueNames /
// z.ApexNSNames for name-only lookups).

// Method1 returns the parent zone.
//
// Deprecated: call z.Parent(ctx) directly.
func Method1(ctx context.Context, z *zone.Zone) (*zone.Zone, error) {
	return ParentZone(ctx, z)
}

// Method2 returns glue names for the zone.
//
// Deprecated: call z.GlueNames(ctx) directly.
func Method2(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	return GlueNames(ctx, z)
}

// Method3 returns nameserver names found in the zone apex.
//
// Deprecated: call z.ApexNSNames(ctx) directly.
func Method3(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	return ApexNSNames(ctx, z)
}

// Method4 returns glue nameserver objects for the zone.
//
// Deprecated: use [nsdiscovery.GlueNameservers] instead.
func Method4(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	return GlueNameservers(ctx, z)
}

// Method5 returns nameserver objects for the zone.
//
// Deprecated: use [nsdiscovery.ApexNameservers] instead.
func Method5(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	return ApexNameservers(ctx, z)
}

// Method2and3 returns the union of Method2 and Method3 results.
//
// Deprecated: use [nsdiscovery.AllNSNames] instead.
func Method2and3(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	return AllNSNames(ctx, z)
}

// Method4and5 returns the union of Method4 and Method5 results.
//
// Deprecated: use [nsdiscovery.AllNameservers] instead.
func Method4and5(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	return AllNameservers(ctx, z)
}

// ParentZone returns the parent zone of z.
//
// Deprecated: call z.Parent(ctx) directly.
func ParentZone(ctx context.Context, z *zone.Zone) (*zone.Zone, error) {
	if z == nil {
		return nil, errNilZone
	}
	return z.Parent(ctx)
}

// GlueNames returns the nameserver names from the parent's delegation glue.
//
// Deprecated: call z.GlueNames(ctx) directly.
func GlueNames(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	if z == nil {
		return nil, errNilZone
	}
	return z.GlueNames(ctx)
}

// ApexNSNames returns nameserver names found in the child zone's apex NS RRset.
//
// Deprecated: call z.ApexNSNames(ctx) directly.
func ApexNSNames(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	if z == nil {
		return nil, errNilZone
	}
	return z.ApexNSNames(ctx)
}

// GlueNameservers returns nameserver objects (name + address) from the parent's glue.
//
// Deprecated: use [nsdiscovery.GlueNameservers] instead.
func GlueNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	return nsdiscovery.GlueNameservers(ctx, z)
}

// ApexNameservers returns nameserver objects resolved from the child zone's apex NS RRset.
//
// Deprecated: use [nsdiscovery.ApexNameservers] instead.
func ApexNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	return nsdiscovery.ApexNameservers(ctx, z)
}

// AllNSNames returns the deduplicated union of GlueNames and ApexNSNames.
//
// Deprecated: use [nsdiscovery.AllNSNames] instead.
func AllNSNames(ctx context.Context, z *zone.Zone) ([]dnsname.Name, error) {
	return nsdiscovery.AllNSNames(ctx, z)
}

// AllNameservers returns the deduplicated union of GlueNameservers and ApexNameservers.
//
// Deprecated: use [nsdiscovery.AllNameservers] instead.
func AllNameservers(ctx context.Context, z *zone.Zone) ([]nameserver.Nameserver, error) {
	return nsdiscovery.AllNameservers(ctx, z)
}
