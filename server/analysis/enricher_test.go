package analysis

import (
	"context"
	"sync"
	"testing"
	"time"

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
