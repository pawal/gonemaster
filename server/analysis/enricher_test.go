package analysis

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

// recordingResolver captures every context it is asked to recurse with, so a
// test can assert what the enricher passed down to the recursor. It returns an
// empty answer so the ASN lookup exhausts its sources without doing real DNS.
type recordingResolver struct {
	mu   sync.Mutex
	ctxs []context.Context
}

func (r *recordingResolver) Recurse(ctx context.Context, name, qtype, qclass string) (packet.Packet, error) {
	r.mu.Lock()
	r.ctxs = append(r.ctxs, ctx)
	r.mu.Unlock()
	return packet.Packet{}, nil
}

func (r *recordingResolver) contexts() []context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]context.Context(nil), r.ctxs...)
}

// The gonemaster recursor refuses to build a nameserver without a cache store
// in its context (nameserver.NewWithContext -> "nil cache store"). The
// projection enricher used to call the recursor with a bare context.Background,
// so every per-address ASN/prefix lookup failed closed and silently returned
// nothing - which is why a full-TLD cohort showed 142 prefixes and every ASN
// had an empty address/nameserver/prefix roster. These tests pin the contract:
// the enricher must hand the resolver a context that carries the nameserver
// cache the recursor demands, for both the address and the label lookups.
func TestEnrichAddressAttachesNameserverCache(t *testing.T) {
	withCymruProfile(t)
	rr := &recordingResolver{}
	e := NewAsnlookupEnricher(rr, time.Minute)

	e.EnrichAddress(context.Background(), "8.8.8.8")

	ctxs := rr.contexts()
	if len(ctxs) == 0 {
		t.Fatal("resolver was never called; the address lookup was short-circuited before recursion")
	}
	for i, ctx := range ctxs {
		if nameserver.CacheFromContext(ctx) == nil {
			t.Fatalf("Recurse call %d had no nameserver cache in its context; the recursor would fail closed and enrichment would silently return empty", i)
		}
	}
}

func TestEnrichASNLabelAttachesNameserverCache(t *testing.T) {
	withCymruProfile(t)
	rr := &recordingResolver{}
	e := NewAsnlookupEnricher(rr, time.Minute)

	e.EnrichASNLabel(context.Background(), 15169)

	ctxs := rr.contexts()
	if len(ctxs) == 0 {
		t.Fatal("resolver was never called; the label lookup was short-circuited before recursion")
	}
	for i, ctx := range ctxs {
		if nameserver.CacheFromContext(ctx) == nil {
			t.Fatalf("Recurse call %d had no nameserver cache in its context", i)
		}
	}
}

// A repeated lookup for the same address within the TTL must be served from the
// enricher's own cache rather than re-querying the resolver. This is what makes
// the month-long TTL configured in the server meaningful: a stable ASN/prefix
// mapping is resolved once, not on every cohort rebuild.
func TestEnrichAddressCachesWithinTTL(t *testing.T) {
	withCymruProfile(t)
	rr := &recordingResolver{}
	e := NewAsnlookupEnricher(rr, time.Hour)

	e.EnrichAddress(context.Background(), "8.8.8.8")
	afterFirst := len(rr.contexts())
	if afterFirst == 0 {
		t.Fatal("first lookup never reached the resolver")
	}

	e.EnrichAddress(context.Background(), "8.8.8.8")
	if afterSecond := len(rr.contexts()); afterSecond != afterFirst {
		t.Fatalf("second lookup re-queried the resolver (%d calls vs %d); result was not cached", afterSecond, afterFirst)
	}
}

// labelResolver answers every query with one canned packet, optionally held
// until gate closes.
type labelResolver struct {
	calls  atomic.Int32
	answer func() packet.Packet
	gate   chan struct{}
}

func (r *labelResolver) Recurse(ctx context.Context, name, qtype, qclass string) (packet.Packet, error) {
	r.calls.Add(1)
	if r.gate != nil {
		<-r.gate
	}
	return r.answer(), nil
}

func foundLabel() packet.Packet {
	msg := new(dns.Msg)
	rr := &dns.TXT{Hdr: dns.Header{Name: "AS199973.asnlookup.zonemaster.net.", Class: dns.ClassINET, TTL: 60}}
	rr.Txt = []string{"199973 | SE | ripencc | 2013-11-20 | MIGR-AS - Migrationsverket, SE"}
	msg.Answer = []dns.RR{rr}
	return packet.Packet{Msg: msg}
}

func emptyLabel() packet.Packet { return packet.Packet{Msg: new(dns.Msg)} }

func failedLabel() packet.Packet { return packet.Packet{} }

// fakeClock is a settable time source for cache expiry.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newLabelEnricher(t *testing.T, answer func() packet.Packet) (*AsnlookupEnricher, *labelResolver, *fakeClock) {
	t.Helper()
	withCymruProfile(t)
	rr := &labelResolver{answer: answer}
	clk := &fakeClock{t: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	e := NewAsnlookupEnricher(rr, 30*24*time.Hour)
	e.now = clk.now
	return e, rr, clk
}

func TestEnrichASNLabelCachesFoundForTTL(t *testing.T) {
	e, rr, clk := newLabelEnricher(t, foundLabel)

	label, ok := e.EnrichASNLabel(context.Background(), 199973)
	if !ok || label != "MIGR-AS - Migrationsverket, SE" {
		t.Fatalf("label = %q, %v", label, ok)
	}
	first := rr.calls.Load()
	clk.advance(30*24*time.Hour - time.Minute)
	e.EnrichASNLabel(context.Background(), 199973)
	if got := rr.calls.Load(); got != first {
		t.Fatalf("lookup inside the TTL made %d calls, want %d", got, first)
	}
	clk.advance(2 * time.Minute)
	e.EnrichASNLabel(context.Background(), 199973)
	if got := rr.calls.Load(); got == first {
		t.Fatal("lookup past the TTL was served from the cache")
	}
}

func TestEnrichASNLabelNegativeTTLs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer func() packet.Packet
		ttl    time.Duration
	}{
		{"empty", emptyLabel, asnEmptyTTL},
		{"error", failedLabel, asnErrorTTL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, rr, clk := newLabelEnricher(t, tc.answer)

			if _, ok := e.EnrichASNLabel(context.Background(), 64500); ok {
				t.Fatal("expected no label")
			}
			first := rr.calls.Load()
			clk.advance(tc.ttl - time.Second)
			e.EnrichASNLabel(context.Background(), 64500)
			if got := rr.calls.Load(); got != first {
				t.Fatalf("lookup inside %s made %d calls, want %d", tc.ttl, got, first)
			}
			clk.advance(2 * time.Second)
			e.EnrichASNLabel(context.Background(), 64500)
			if got := rr.calls.Load(); got == first {
				t.Fatalf("lookup past %s was served from the cache", tc.ttl)
			}
		})
	}
}

func TestEnrichASNLabelCanceledNotCached(t *testing.T) {
	e, rr, _ := newLabelEnricher(t, failedLabel)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	e.EnrichASNLabel(ctx, 64500)
	first := rr.calls.Load()
	e.EnrichASNLabel(context.Background(), 64500)
	if got := rr.calls.Load(); got == first {
		t.Fatal("a canceled lookup was cached")
	}
}

func TestEnrichASNLabelSingleFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, rr, _ := newLabelEnricher(t, foundLabel)
		rr.gate = make(chan struct{})

		const n = 8
		labels := make(chan string, n)
		for range n {
			go func() {
				label, _ := e.EnrichASNLabel(context.Background(), 199973)
				labels <- label
			}()
		}
		synctest.Wait()
		calls := rr.calls.Load()
		close(rr.gate)
		for range n {
			if label := <-labels; label != "MIGR-AS - Migrationsverket, SE" {
				t.Errorf("label = %q", label)
			}
		}
		if calls != 1 {
			t.Fatalf("%d concurrent callers made %d backend lookups, want 1", n, calls)
		}
	})
}

// withCymruProfile installs a default (Cymru) effective profile so the ASN
// lookup gets past its style/sources guards and actually reaches the resolver,
// then restores the prior effective profile when the test ends.
func withCymruProfile(t *testing.T) {
	t.Helper()
	prev := profile.Effective()
	p, err := profile.Default()
	if err != nil {
		t.Fatalf("load default profile: %v", err)
	}
	if p.ASNDB.Style == "" {
		t.Fatalf("default profile has no ASN database style; lookup would never reach the resolver")
	}
	profile.SetEffective(p)
	t.Cleanup(func() { profile.SetEffective(prev) })
}
