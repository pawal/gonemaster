package engine

import (
	"context"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnssecchain"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/internal/nstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func dnskeyAnswer(owner string, key *dns.DNSKEY) packet.Packet {
	return dnstest.Response(dnstest.Question(owner, dns.TypeDNSKEY), dnstest.Reply(),
		dnstest.Answers(key), dnstest.Secure())
}

// undelegatedZone builds a zone whose NS set resolves from fake glue, with a
// hooked child server that serves a signed-looking DNSKEY answer and carries a
// fake DS. It returns the zone with its NS set already memoized.
func undelegatedZone(t *testing.T, ctx context.Context) *zone.Zone {
	t.Helper()

	r := nstest.HintedRecursor(t, map[string]map[string][]string{
		"example": {"ns1.example": {"192.0.2.55"}},
	})

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	zp := &z

	servers, err := zp.NS(ctx)
	if err != nil {
		t.Fatalf("resolve NS: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 fake child server, got %d", len(servers))
	}
	child := servers[0]

	key := dnstest.GenKey(t, "example", dns.ECDSAP256SHA256, true).Key
	child.SetQueryHook(func(_ context.Context, qname, qtype, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if qtype == "DNSKEY" {
			return dnskeyAnswer("example", key), nil
		}
		return packet.Packet{}, nil
	})

	// Warm the DNSKEY cache the way a testcase would.
	on := true
	if _, err := child.QueryWithOptions(ctx, "example", "DNSKEY", &nameserver.QueryOptions{DNSSEC: &on}); err != nil {
		t.Fatalf("warm DNSKEY: %v", err)
	}

	// Attach a fake DS so the extractor reports an input-provided delegation.
	ds := key.ToDS(dns.SHA256)
	if err := child.AddFakeDS("example", []nameserver.DSData{{
		KeyTag:     ds.KeyTag,
		Algorithm:  ds.Algorithm,
		DigestType: ds.DigestType,
		Digest:     ds.Digest,
	}}); err != nil {
		t.Fatalf("add fake DS: %v", err)
	}

	return zp
}

func TestEmitDNSSECChainNilSinkIsNoop(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	zp := undelegatedZone(t, ctx)
	// Must not panic when no sink is configured.
	emitDNSSECChain(ctx, RunRequest{}, zp)
}

func TestEmitDNSSECChainInvokesSink(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	ctx = WithRunner(ctx, &Runner{StartedAt: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)})
	zp := undelegatedZone(t, ctx)

	var got *dnssecchain.Summary
	calls := 0
	req := RunRequest{DNSSECChainSink: func(s *dnssecchain.Summary) {
		calls++
		got = s
	}}

	emitDNSSECChain(ctx, req, zp)

	if calls != 1 {
		t.Fatalf("sink called %d times, want 1", calls)
	}
	if got == nil {
		t.Fatal("sink received nil summary")
	}
	if got.Zone != "example" {
		t.Errorf("zone = %q, want example", got.Zone)
	}
	if got.Delegation != dnssecchain.DelegationUndelegated {
		t.Errorf("delegation = %q, want undelegated", got.Delegation)
	}
	if got.Parent.DSSource != dnssecchain.DSSourceInput {
		t.Errorf("ds_source = %q, want input", got.Parent.DSSource)
	}
}

func TestEmitDNSSECChainColdCacheSkipsSink(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	// A zone with no memoized NS and a cold cache yields no summary.
	z, err := zone.NewWithRecursor("example.test", nil)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	calls := 0
	req := RunRequest{DNSSECChainSink: func(*dnssecchain.Summary) { calls++ }}
	emitDNSSECChain(ctx, req, &z)

	if calls != 0 {
		t.Errorf("sink called %d times on cold cache, want 0", calls)
	}
}
