package nameserver

import (
	"context"
	"sync"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnssecutil"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

// signedDNSKEYResponse builds a self-signed DNSKEY answer whose RRSIG is valid
// right now, so verification reaches the crypto path instead of stopping at the
// validity-period check. The message is round-tripped through the wire format
// so the records the cache ends up holding are exactly as unpacked from a real
// response: no memoized key tag, nothing canonicalized by the signer.
func signedDNSKEYResponse(t *testing.T, qname string) *dns.Msg {
	t.Helper()

	kp := dnstest.GenKey(t, qname, dns.ECDSAP256SHA256, false)
	key := kp.Key
	rrset := []dns.RR{key}
	now := time.Now().UTC()
	sig := dnstest.SignRRset(t, kp, rrset, qname, qname, now.Add(-time.Hour), now.Add(24*time.Hour))

	msg := dnsutil.SetQuestion(&dns.Msg{}, dnsutil.Fqdn(qname), dns.TypeDNSKEY)
	msg.Response = true
	msg.Answer = []dns.RR{key, sig}

	if err := msg.Pack(); err != nil {
		t.Fatalf("pack answer: %v", err)
	}
	parsed := &dns.Msg{}
	parsed.Data = append([]byte(nil), msg.Data...)
	if err := parsed.Unpack(); err != nil {
		t.Fatalf("unpack answer: %v", err)
	}
	return parsed
}

// dnskeyAndSig picks the key and the signature out of a cached answer. Both
// point into the shared cached message, which is the whole point of the test.
func dnskeyAndSig(t *testing.T, msg *dns.Msg) (*dns.DNSKEY, *dns.RRSIG, []dns.RR) {
	t.Helper()
	var key *dns.DNSKEY
	var sig *dns.RRSIG
	var rrset []dns.RR
	for _, rr := range msg.Answer {
		switch v := rr.(type) {
		case *dns.DNSKEY:
			key = v
			rrset = append(rrset, v)
		case *dns.RRSIG:
			sig = v
		}
	}
	if key == nil || sig == nil {
		t.Fatalf("cached answer lost its DNSKEY or RRSIG")
	}
	return key, sig, rrset
}

// warmSharedCache primes one cache entry the way a first job would, then hands
// out the per-job run stores that adopt it, which is what the server does for
// concurrent jobs while the hot cache is warm.
func warmSharedCache(ctx context.Context, t *testing.T, qname string, addr string, answer *dns.Msg, jobs int) []*CacheStore {
	t.Helper()

	base := NewCacheStore()
	warm, err := NewWithCache(base, "ns1.example", addr, nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	warm.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{Msg: answer}, nil
	})
	if _, err := warm.QueryWithOptions(ctx, qname, "DNSKEY", nil); err != nil {
		t.Fatalf("warm-up query: %v", err)
	}

	stores := make([]*CacheStore, jobs)
	for i := range stores {
		stores[i] = base.SnapshotForRun()
	}
	return stores
}

// Two concurrent jobs verifying signatures out of one warmed cache entry must
// not step on each other: SnapshotForRun shares warmed query data across runs,
// a cache hit hands back a shallow copy, and the library's verification path
// writes through the shared RR pointers.
//
// The assertions are deliberately weak - both verifications must succeed -
// because the real detector is `go test -race`. A reported race and a signature
// that suddenly fails to verify are the same underlying bug.
func TestConcurrentVerifyOfSharedCachedRRSIG(t *testing.T) {
	const qname = "shared-rrsig.example"
	const addr = "192.0.2.53"
	const jobs = 2

	ctx, _ := testContext(t)
	answer := signedDNSKEYResponse(t, qname)
	stores := warmSharedCache(ctx, t, qname, addr, answer, jobs)

	var wg sync.WaitGroup
	wg.Add(jobs)
	errs := make([]error, jobs)
	for i := range jobs {
		go func() {
			defer wg.Done()
			ns, err := NewWithCache(stores[i], "ns1.example", addr, nil)
			if err != nil {
				errs[i] = err
				return
			}
			// No query hook: the answer must come from the shared cache.
			resp, err := ns.QueryWithOptions(ctx, qname, "DNSKEY", nil)
			if err != nil {
				errs[i] = err
				return
			}
			if resp.Msg == nil {
				t.Errorf("job %d got no cached message", i)
				return
			}
			key, sig, rrset := dnskeyAndSig(t, resp.Msg)
			for range 50 {
				if err := dnssecutil.VerifyRRSIG(sig, rrset, key, time.Now()); err != nil {
					errs[i] = err
					return
				}
			}
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("job %d: %v", i, err)
		}
	}
}

// The same sharing bites the key tag. dns.DNSKEY.KeyTag() memoizes its result
// into the record it is called on, and engine code calls it on DNSKEYs that
// live in the shared cache, so two jobs computing the tag of one cached key
// wrote to the same field. dnssecutil.KeyTag computes on a copy instead, which
// this test pins down with the race detector, by requiring every job to see the
// same tag, and by requiring the shared record to come out unmemoized.
func TestConcurrentKeyTagOfSharedCachedDNSKEY(t *testing.T) {
	const qname = "shared-keytag.example"
	const addr = "192.0.2.54"
	const jobs = 2

	ctx, _ := testContext(t)
	answer := signedDNSKEYResponse(t, qname)
	cachedKey, _, _ := dnskeyAndSig(t, answer)
	if cachedKey.Tag != 0 {
		t.Fatalf("test setup: the cached key arrives memoized (Tag = %d)", cachedKey.Tag)
	}
	// A private copy carries the memoization, so the expected value is known
	// without writing to the record the jobs share.
	reference := *cachedKey
	want := reference.KeyTag()

	stores := warmSharedCache(ctx, t, qname, addr, answer, jobs)

	var wg sync.WaitGroup
	wg.Add(jobs)
	tags := make([][]uint16, jobs)
	errs := make([]error, jobs)
	for i := range jobs {
		go func() {
			defer wg.Done()
			ns, err := NewWithCache(stores[i], "ns1.example", addr, nil)
			if err != nil {
				errs[i] = err
				return
			}
			resp, err := ns.QueryWithOptions(ctx, qname, "DNSKEY", nil)
			if err != nil {
				errs[i] = err
				return
			}
			if resp.Msg == nil {
				t.Errorf("job %d got no cached message", i)
				return
			}
			key, _, _ := dnskeyAndSig(t, resp.Msg)
			for range 50 {
				tags[i] = append(tags[i], dnssecutil.KeyTag(key))
			}
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("job %d: %v", i, err)
		}
	}
	for i, got := range tags {
		if len(got) == 0 {
			t.Fatalf("job %d computed no key tags", i)
		}
		for _, tag := range got {
			if tag != want {
				t.Fatalf("job %d: key tag %d, want %d", i, tag, want)
			}
		}
	}
	if cachedKey.Tag != 0 {
		t.Errorf("the shared cached key was memoized into: Tag = %d, want 0", cachedKey.Tag)
	}
}
