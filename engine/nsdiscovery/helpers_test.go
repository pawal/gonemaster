package nsdiscovery

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/internal/nstest"
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

// setNSHook registers a nameserver answering NS queries at the zone apex with
// the given NS names.
func setNSHook(ctx context.Context, t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, nsNames ...string) {
	t.Helper()
	nstest.HookedNS(t, ctx, r, name, addr,
		nstest.AnswerHook(zoneName, "NS", nsAnswerPacket(zoneName, nsNames...)))
}

// setHookWithPacket registers a nameserver answering NS queries at the zone
// apex with a caller-supplied packet.
func setHookWithPacket(ctx context.Context, t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, p packet.Packet) {
	t.Helper()
	nstest.HookedNS(t, ctx, r, name, addr, nstest.AnswerHook(zoneName, "NS", p))
}

// newAuthoritativeNameserver builds a nameserver hooked to return an
// authoritative NS answer for zoneName.
func newAuthoritativeNameserver(ctx context.Context, t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, nsNames ...string) nameserver.Nameserver {
	t.Helper()
	return nstest.HookedNS(t, ctx, r, name, addr,
		nstest.AnswerHook(zoneName, "NS", authoritativeNSPacket(zoneName, nsNames...)))
}

// seedParentCache stores a sentinel entry in cache under the given zone key.
// Returns a freshly built nameserver matching the entry so tests can compare
// materialized output.
func seedParentCache(ctx context.Context, t *testing.T, r *recursor.Recursor, cache *Cache, zoneKey string, nsName string, nsAddr string) nameserver.Nameserver {
	t.Helper()
	ns := nstest.NS(t, ctx, r, nsName, nsAddr)
	cache.store(zoneKey, []nameserver.Nameserver{ns}, true)
	return ns
}
