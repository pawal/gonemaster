package nameserver

import (
	"context"
	"net/netip"
	"strings"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

// cachedReturnFixture warms one cache entry and returns a nameserver bound to a
// logger the test controls, so a second query is served from the cache and goes
// through the CACHED_RETURN path.
func cachedReturnFixture(t *testing.T, log *logger.Logger) (context.Context, Nameserver, string) {
	t.Helper()

	prof := dnstest.DefaultProfile(t)
	log.SetProfile(prof)
	ctx := profile.WithContext(context.Background(), prof)
	ctx = logger.WithContext(ctx, log)

	const qname = "cached-return.example"
	answer := dnsutil.SetQuestion(&dns.Msg{}, dnsutil.Fqdn(qname), dns.TypeA)
	answer.Response = true
	a := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 3600}}
	a.Addr = netip.MustParseAddr("192.0.2.7")
	answer.Answer = []dns.RR{a}

	ns, err := NewWithCache(NewCacheStore(), "ns1.example", "192.0.2.53", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{Msg: answer}, nil
	})
	if _, err := ns.QueryWithOptions(ctx, qname, "A", nil); err != nil {
		t.Fatalf("warm-up query: %v", err)
	}
	return ctx, ns, qname
}

// CACHED_RETURN carries a full text rendering of the cached response, which is
// the most expensive log argument the engine builds. Below the capture level no
// entry may be stored.
func TestCachedReturnNotStoredWhenNotCaptured(t *testing.T) {
	log := logger.New()
	ctx, ns, qname := cachedReturnFixture(t, log)
	if err := log.SetCaptureLevel("INFO"); err != nil {
		t.Fatalf("set capture level: %v", err)
	}

	before := len(log.Entries())
	resp, err := ns.QueryWithOptions(ctx, qname, "A", nil)
	if err != nil {
		t.Fatalf("cached query: %v", err)
	}
	if resp.Msg == nil {
		t.Fatal("expected the cached answer to be returned")
	}
	if entry := dnstest.EntryByTag(log.Entries()[before:], "CACHED_RETURN"); entry != nil {
		t.Fatalf("CACHED_RETURN was stored below the capture level: %+v", entry)
	}
}

// Not storing the entry is only half of it: the packet render has to be skipped
// too, otherwise the expensive work still happens and is thrown away. The
// render copies the message and formats every record, so it is plainly visible
// in the allocation count of a cache hit. The assertion is a wide margin rather
// than an exact count, so it survives unrelated churn in the query path.
func TestCachedReturnRenderSkippedWhenNotCaptured(t *testing.T) {
	allocsWithFloor := func(floor string) float64 {
		log := logger.New()
		ctx, ns, qname := cachedReturnFixture(t, log)
		if err := log.SetCaptureLevel(floor); err != nil {
			t.Fatalf("set capture level: %v", err)
		}
		return testing.AllocsPerRun(50, func() {
			if _, err := ns.QueryWithOptions(ctx, qname, "A", nil); err != nil {
				t.Fatalf("cached query: %v", err)
			}
		})
	}

	captured := allocsWithFloor("DEBUG3")
	gated := allocsWithFloor("INFO")
	if gated > captured*2/3 {
		t.Fatalf("a gated cache hit allocated %.0f against %.0f when captured; the packet render is still happening", gated, captured)
	}
}

// The mirror case: with no capture level the entry is still produced, and it
// still carries the rendered packet. This is what keeps --min-level DEBUG3 and
// --count working.
func TestCachedReturnRenderedWhenCaptured(t *testing.T) {
	log := logger.New()
	ctx, ns, qname := cachedReturnFixture(t, log)

	before := len(log.Entries())
	if _, err := ns.QueryWithOptions(ctx, qname, "A", nil); err != nil {
		t.Fatalf("cached query: %v", err)
	}

	entry := dnstest.EntryByTag(log.Entries()[before:], "CACHED_RETURN")
	if entry == nil {
		t.Fatal("expected a CACHED_RETURN entry when everything is captured")
	}
	rendered, _ := entry.Args["packet"].(string)
	if !strings.Contains(rendered, qname) {
		t.Fatalf("expected the rendered packet to mention %q, got %q", qname, rendered)
	}
}

// A capture level below DEBUG3 must not gate the entry away either, so an
// operator asking for full debug output still gets the packets.
func TestCachedReturnKeptAtDebug3CaptureLevel(t *testing.T) {
	log := logger.New()
	ctx, ns, qname := cachedReturnFixture(t, log)
	if err := log.SetCaptureLevel("DEBUG3"); err != nil {
		t.Fatalf("set capture level: %v", err)
	}

	before := len(log.Entries())
	if _, err := ns.QueryWithOptions(ctx, qname, "A", nil); err != nil {
		t.Fatalf("cached query: %v", err)
	}
	if entry := dnstest.EntryByTag(log.Entries()[before:], "CACHED_RETURN"); entry == nil {
		t.Fatal("expected a CACHED_RETURN entry at a DEBUG3 capture level")
	}
}
