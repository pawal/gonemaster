package nsdiscovery

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
)

// Shared test helpers for the nsdiscovery package.

// nsAnswerPacket builds an NS answer packet listing nsNames as authoritative
// nameservers for zoneName.
func nsAnswerPacket(zoneName string, nsNames ...string) packet.Packet {
	return dnstest.Response(dnstest.NotAuthoritative(),
		dnstest.Answers(dnstest.NSRRs(zoneName, nsNames...)...))
}

// authoritativeNSPacket builds an authoritative NS answer packet.
func authoritativeNSPacket(zoneName string, nsNames ...string) packet.Packet {
	return dnstest.Response(dnstest.Answers(dnstest.NSRRs(zoneName, nsNames...)...))
}

// newRootRecursor builds a recursor pre-seeded with fake addresses at the root.
func newRootRecursor(t *testing.T, data map[string][]string) *recursor.Recursor {
	t.Helper()
	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", data); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	return r
}

// setNSHook registers a nameserver that returns an NS answer for the given
// zone-NS query and an empty packet for other queries.
func setNSHook(ctx context.Context, t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, nsNames ...string) {
	t.Helper()
	ns, err := nameserver.NewWithContext(ctx, name, addr, r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if qname != zoneName || qtype != "NS" {
			return packet.Packet{}, nil
		}
		return nsAnswerPacket(zoneName, nsNames...), nil
	})
}

// setHookWithPacket installs a query hook returning a caller-supplied packet
// for any NS query at the named zone apex; other queries return an empty
// packet (nil Msg).
func setHookWithPacket(ctx context.Context, t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, p packet.Packet) {
	t.Helper()
	ns, err := nameserver.NewWithContext(ctx, name, addr, r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if qname != zoneName || qtype != "NS" {
			return packet.Packet{}, nil
		}
		return p, nil
	})
}

// newAuthoritativeNameserver builds a nameserver hooked to return an
// authoritative NS answer for zoneName.
func newAuthoritativeNameserver(ctx context.Context, t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, nsNames ...string) nameserver.Nameserver {
	t.Helper()
	ns, err := nameserver.NewWithContext(ctx, name, addr, r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if qname != zoneName || qtype != "NS" {
			return packet.Packet{}, nil
		}
		return authoritativeNSPacket(zoneName, nsNames...), nil
	})
	return ns
}

// seedParentCache stores a sentinel entry in cache under the given zone key.
// Returns a freshly built nameserver matching the entry so tests can compare
// materialized output.
func seedParentCache(ctx context.Context, t *testing.T, r *recursor.Recursor, cache *Cache, zoneKey string, nsName string, nsAddr string) nameserver.Nameserver {
	t.Helper()
	ns, err := nameserver.NewWithContext(ctx, nsName, nsAddr, r.Client())
	if err != nil {
		t.Fatalf("seed nameserver: %v", err)
	}
	cache.store(zoneKey, []nameserver.Nameserver{ns}, true)
	return ns
}
