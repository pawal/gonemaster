package recursor

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

type fakeQueryer struct {
	resp packet.Packet
	err  error
}

func (f fakeQueryer) QueryWithClass(_ context.Context, _ string, _ string, _ string) (packet.Packet, error) {
	return f.resp, f.err
}

// observingQueryer blocks until its ctx is cancelled, then records the
// cancellation's cause. Used to verify race-loss observability.
type observingQueryer struct {
	mu     sync.Mutex
	cause  error
	called bool
}

func (q *observingQueryer) QueryWithClass(ctx context.Context, _ string, _ string, _ string) (packet.Packet, error) {
	q.mu.Lock()
	q.called = true
	q.mu.Unlock()
	<-ctx.Done()
	q.mu.Lock()
	q.cause = context.Cause(ctx)
	q.mu.Unlock()
	return packet.Packet{}, ctx.Err()
}

func (q *observingQueryer) observedCause() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.cause
}

func (q *observingQueryer) wasCalled() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.called
}

// refusedPacket returns an authoritative REFUSED response from answerFrom.
func refusedPacket(answerFrom string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeRefused
	return packet.Packet{Msg: msg, AnswerFrom: answerFrom}
}

// TestRecurseOrderedRaceLossSetsCause exercises the parallel ordered batch:
// goroutine 0 returns a CONTINUE response (REFUSED), goroutine 1 returns a
// RETURN response (answer), and goroutine 2 blocks. After processing #1 the
// recursor cancels the batch; #2 must observe context.Cause == ErrRaceLost.
func TestRecurseOrderedRaceLossSetsCause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		prof := testhelpers.DefaultProfile(t)
		prof.Resolver.Defaults.Parallel = 3
		prof.Resolver.Defaults.Unordered = false
		ctx := profile.WithContext(context.Background(), prof)

		loser := &observingQueryer{}
		winner := fakeQueryer{resp: answerPacket("www.example", "203.0.113.50")}
		noise := fakeQueryer{resp: refusedPacket("203.0.113.51")}

		// state.ns is consumed from the END, so the order in the slice is
		// reverse of the batch order. Batch = [noise, winner, loser].
		state := &recurseState{ns: []queryer{loser, winner, noise}}

		r := &Recursor{}
		respDone := make(chan error, 1)
		go func() {
			_, _, e := r.recurse(ctx, "www.example", "A", "IN", state)
			respDone <- e
		}()

		if e := <-respDone; e != nil {
			t.Fatalf("recurse returned error: %v", e)
		}

		if !loser.wasCalled() {
			t.Fatalf("loser goroutine was never invoked")
		}
		cause := loser.observedCause()
		if cause == nil {
			t.Fatalf("loser observed no cancellation cause")
		}
		if !errors.Is(cause, ErrRaceLost) {
			t.Fatalf("loser observed cause = %v, want ErrRaceLost", cause)
		}
	})
}

// TestRecurseUnorderedRaceLossSetsCause exercises the unordered fan-out.
func TestRecurseUnorderedRaceLossSetsCause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		prof := testhelpers.DefaultProfile(t)
		prof.Resolver.Defaults.Parallel = 2
		prof.Resolver.Defaults.Unordered = true
		ctx := profile.WithContext(context.Background(), prof)

		loser := &observingQueryer{}
		winner := fakeQueryer{resp: answerPacket("www.example", "203.0.113.60")}

		state := &recurseState{ns: []queryer{loser, winner}}

		r := &Recursor{}
		respDone := make(chan error, 1)
		go func() {
			_, _, e := r.recurse(ctx, "www.example", "A", "IN", state)
			respDone <- e
		}()

		if e := <-respDone; e != nil {
			t.Fatalf("recurse returned error: %v", e)
		}

		if !loser.wasCalled() {
			t.Fatalf("loser goroutine was never invoked")
		}
		cause := loser.observedCause()
		if cause == nil {
			t.Fatalf("loser observed no cancellation cause")
		}
		if !errors.Is(cause, ErrRaceLost) {
			t.Fatalf("loser observed cause = %v, want ErrRaceLost", cause)
		}
	})
}

func TestRecurseSkipsUpwardReferral(t *testing.T) {
	root := fakeQueryer{resp: referralPacket("child.example", "203.0.113.1")}
	child := fakeQueryer{resp: referralPacket("example", "203.0.113.2")}
	upward := fakeQueryer{resp: nxdomainPacket("203.0.113.3")}

	next := map[string][]queryer{
		"203.0.113.1": {child},
		"203.0.113.2": {upward},
	}

	state := &recurseState{
		ns: []queryer{root},
		nsFrom: func(_ context.Context, resp packet.Packet, _ *recurseState) ([]queryer, error) {
			return next[resp.AnswerFrom], nil
		},
	}

	r := &Recursor{}
	resp, out, err := r.recurse(context.Background(), "www.child.example", "A", "IN", state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg != nil {
		t.Fatalf("expected no response, got %s", resp.Rcode())
	}
	if !out.qnameSet || out.qname.String() != "www.child.example" {
		t.Fatalf("expected qname set to www.child.example, got %q", out.qname.String())
	}
	if out.common != 2 {
		t.Fatalf("expected common count 2, got %d", out.common)
	}
}

func TestRecurseFollowsDownwardReferral(t *testing.T) {
	root := fakeQueryer{resp: referralPacket("child.example", "203.0.113.11")}
	child := fakeQueryer{resp: referralPacket("grandchild.child.example", "203.0.113.12")}
	leaf := fakeQueryer{resp: answerPacket("www.grandchild.child.example", "203.0.113.13")}

	next := map[string][]queryer{
		"203.0.113.11": {child},
		"203.0.113.12": {leaf},
	}

	state := &recurseState{
		ns: []queryer{root},
		nsFrom: func(_ context.Context, resp packet.Packet, _ *recurseState) ([]queryer, error) {
			return next[resp.AnswerFrom], nil
		},
	}

	r := &Recursor{}
	resp, _, err := r.recurse(context.Background(), "www.grandchild.child.example", "A", "IN", state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg == nil || resp.Type() != "answer" {
		t.Fatalf("expected answer response, got %q", resp.Type())
	}
}

func TestRecurseIgnoresRootReferral(t *testing.T) {
	root := fakeQueryer{resp: referralPacket(".", "203.0.113.21")}

	state := &recurseState{
		ns: []queryer{root},
	}

	r := &Recursor{}
	resp, _, err := r.recurse(context.Background(), "www.example", "A", "IN", state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg != nil {
		t.Fatalf("expected no response for root referral, got %s", resp.Rcode())
	}
}

func TestRecurseFollowsOutOfBailiwickCNAME(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	r := fakeRootRecursor(t, "root.test", "192.0.2.53")

	rootNS, err := nameserver.NewWithContext(ctx, "root.test", "192.0.2.53", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if name != "alias.example.net" || qtype != "A" {
			return packet.Packet{}, nil
		}
		return answerPacket("alias.example.net", "192.0.2.53"), nil
	})

	state := &recurseState{
		ns: []queryer{
			fakeQueryer{resp: cnamePacket("www.example.com", "alias.example.net", "203.0.113.5")},
		},
	}

	resp, _, err := r.recurse(ctx, "www.example.com", "A", "IN", state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg == nil {
		t.Fatalf("expected response")
	}
	if len(resp.GetRecordsForName("A", dnsname.New("alias.example.net"), "answer")) == 0 {
		t.Fatalf("expected A answer for alias.example.net")
	}
}

func TestRecurseStopsOnCNAMEWithQtypeMismatch(t *testing.T) {
	resp := cnamePacket("www.example.com", "alias.example.net", "203.0.113.6")
	aRR := &dns.A{Hdr: dns.Header{Name: "other.example.net.", Class: dns.ClassINET, TTL: 60}}
	aRR.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 55})
	resp.Msg.Answer = append(resp.Msg.Answer, aRR)

	state := &recurseState{
		ns: []queryer{
			fakeQueryer{resp: resp},
		},
	}

	r := &Recursor{}
	out, _, err := r.recurse(context.Background(), "www.example.com", "A", "IN", state)
	var ce *CNAMEError
	if !errors.As(err, &ce) || ce.Reason != CNAMEUnresolved || ce.Detail != "qtype-mismatch" {
		t.Fatalf("expected *CNAMEError unresolved/qtype-mismatch, got: %v", err)
	}
	if out.Msg != nil {
		t.Fatalf("expected no response when qtype does not match CNAME target")
	}
}

func TestRecurseReturnsCandidateOnRefused(t *testing.T) {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeRefused
	state := &recurseState{
		ns: []queryer{
			fakeQueryer{resp: packet.Packet{Msg: msg}},
		},
	}

	r := &Recursor{}
	resp, _, err := r.recurse(context.Background(), "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg == nil || resp.Rcode() != "REFUSED" {
		t.Fatalf("expected refused response")
	}
}

func TestRecurseStopsOnInProgress(t *testing.T) {
	state := &recurseState{
		inProgress: map[string]map[string]bool{
			"example": {"A": true},
		},
	}

	r := &Recursor{}
	resp, _, err := r.recurse(context.Background(), "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg != nil {
		t.Fatalf("expected no response when already in progress")
	}
}

func TestResolveCNAMEWithTargetAnswer(t *testing.T) {
	resp := cnamePacket("www.example.com", "alias.example.net", "203.0.113.7")
	aRR2 := &dns.A{Hdr: dns.Header{Name: "alias.example.net.", Class: dns.ClassINET, TTL: 60}}
	aRR2.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 200})
	resp.Msg.Answer = append(resp.Msg.Answer, aRR2)

	r := &Recursor{}
	out, _, err := r.resolveCNAME(context.Background(), dnsname.New("www.example.com"), "A", "IN", resp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Msg == nil {
		t.Fatalf("expected response with target answer")
	}
	if !out.HasRRsOfTypeForName("A", dnsname.New("alias.example.net"), "answer") {
		t.Fatalf("expected target A record")
	}
}

func TestResolveCNAMETooManyRecordsReturnsTooMany(t *testing.T) {
	// Build an answer carrying CNAMEMaxRecords+1 distinct CNAME RRs so the
	// per-answer cardinality cap trips.
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	for i := 0; i <= constants.CNAMEMaxRecords; i++ {
		rr := &dns.CNAME{Hdr: dns.Header{Name: fmt.Sprintf("a%d.example.com.", i), Class: dns.ClassINET, TTL: 60}}
		rr.Target = fmt.Sprintf("b%d.example.com.", i)
		msg.Answer = append(msg.Answer, rr)
	}
	resp := packet.Packet{Msg: msg}

	r := &Recursor{}
	_, _, err := r.resolveCNAME(context.Background(), dnsname.New("www.example.com"), "A", "IN", resp, nil)
	var ce *CNAMEError
	if !errors.As(err, &ce) || ce.Reason != CNAMETooMany {
		t.Fatalf("expected *CNAMEError too-many, got: %v", err)
	}
}

func TestResolveCNAMEBrokenChainReturnsUnresolved(t *testing.T) {
	// Build an answer with two unrelated CNAME pairs: a -> b and c -> d.
	// Walking from the qname follows one pair and stops; counter != len(unique)
	// trips the broken-chain check.
	resp := cnamePacket("www.example.com", "alias.example.com", "203.0.113.9")
	cnameUnrelated := &dns.CNAME{Hdr: dns.Header{Name: "orphan.example.com.", Class: dns.ClassINET, TTL: 60}}
	cnameUnrelated.Target = "unused.example.com."
	resp.Msg.Answer = append(resp.Msg.Answer, cnameUnrelated)

	r := &Recursor{}
	out, _, err := r.resolveCNAME(context.Background(), dnsname.New("www.example.com"), "A", "IN", resp, nil)
	var ce *CNAMEError
	if !errors.As(err, &ce) || ce.Reason != CNAMEUnresolved || ce.Detail != "broken-chain" {
		t.Fatalf("expected *CNAMEError unresolved/broken-chain, got: %v", err)
	}
	if out.Msg != nil {
		t.Fatalf("expected no response for broken chain")
	}
}

func TestResolveCNAMEQtypeMismatchReturnsUnresolved(t *testing.T) {
	// Answer carries an A RR for an unrelated name, but none for the CNAME
	// target. Trips the qtype-mismatch path.
	resp := cnamePacket("www.example.com", "alias.example.net", "203.0.113.11")
	strayA := &dns.A{Hdr: dns.Header{Name: "stray.example.org.", Class: dns.ClassINET, TTL: 60}}
	strayA.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 11})
	resp.Msg.Answer = append(resp.Msg.Answer, strayA)

	r := &Recursor{}
	out, _, err := r.resolveCNAME(context.Background(), dnsname.New("www.example.com"), "A", "IN", resp, nil)
	var ce *CNAMEError
	if !errors.As(err, &ce) || ce.Reason != CNAMEUnresolved || ce.Detail != "qtype-mismatch" {
		t.Fatalf("expected *CNAMEError unresolved/qtype-mismatch, got: %v", err)
	}
	if out.Msg != nil {
		t.Fatalf("expected no response for qtype mismatch")
	}
}

func TestResolveCNAMEChainDepthExceededReturnsChainTooLong(t *testing.T) {
	// Pre-seed the state's tcount above CNAMEMaxChainLength so the chain-depth
	// check (post-state-update) trips.
	resp := cnamePacket("www.example.com", "alias.example.net", "203.0.113.10")

	state := &recurseState{tcount: constants.CNAMEMaxChainLength}
	r := &Recursor{}
	_, _, err := r.resolveCNAME(context.Background(), dnsname.New("www.example.com"), "A", "IN", resp, state)
	var ce *CNAMEError
	if !errors.As(err, &ce) || ce.Reason != CNAMEChainTooLong {
		t.Fatalf("expected *CNAMEError chain-too-long, got: %v", err)
	}
}

func TestCacheStoresAndReturnsCNAMEError(t *testing.T) {
	// Verifies the cache stores typed *CNAMEError and returns it on hit.
	// Guards against silent first-hit-only reporting: any subsequent lookup
	// for the same (name, qtype, qclass) must observe the same typed error.
	r := &Recursor{}
	r.SetNegativeCacheTTL(60 * time.Second)

	cnameErr := &CNAMEError{Reason: CNAMEUnresolved, Name: "www.example.com", Target: "loop.example.com", Detail: "loop"}
	r.cacheStoreNegative("k", "A", "IN", cnameErr)

	cached, gotErr, ok := r.cacheLookup("k", "A", "IN")
	if !ok {
		t.Fatalf("expected cache hit")
	}
	if cached.Msg != nil {
		t.Fatalf("expected empty packet on cache hit, got Msg=%v", cached.Msg)
	}
	var ce *CNAMEError
	if !errors.As(gotErr, &ce) || ce.Reason != CNAMEUnresolved || ce.Detail != "loop" {
		t.Fatalf("expected cached *CNAMEError unresolved/loop, got: %v", gotErr)
	}

	// Second lookup behaves the same (cache must not flip to nil err).
	_, gotErr2, ok2 := r.cacheLookup("k", "A", "IN")
	if !ok2 {
		t.Fatalf("expected second cache hit")
	}
	if !errors.As(gotErr2, &ce) || ce.Reason != CNAMEUnresolved {
		t.Fatalf("expected cached *CNAMEError on second hit, got: %v", gotErr2)
	}
}

func TestResolveCNAMELoopReturnsUnresolved(t *testing.T) {
	resp := cnamePacket("www.example.com", "alias.example.com", "203.0.113.8")
	cnameRR2 := &dns.CNAME{Hdr: dns.Header{Name: "alias.example.com.", Class: dns.ClassINET, TTL: 60}}
	cnameRR2.Target = "www.example.com."
	resp.Msg.Answer = append(resp.Msg.Answer, cnameRR2)

	r := &Recursor{}
	out, _, err := r.resolveCNAME(context.Background(), dnsname.New("www.example.com"), "A", "IN", resp, nil)
	var ce *CNAMEError
	if !errors.As(err, &ce) || ce.Reason != CNAMEUnresolved || ce.Detail != "loop" {
		t.Fatalf("expected *CNAMEError unresolved/loop, got: %v", err)
	}
	if out.Msg != nil {
		t.Fatalf("expected no response for CNAME loop")
	}
}

// TestResolveCNAMEDoesNotShareInProgress verifies that CNAME resolution
// can re-resolve nameserver addresses that the parent recursion already
// resolved. This reproduces a bug where the shared inProgress map blocked
// nameserver address resolution during CNAME following, causing false
// NO_RESPONSE_PTR_QUERY results for classless IN-ADDR.ARPA delegations.
func TestResolveCNAMEDoesNotShareInProgress(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := fakeRootRecursor(t, "root.test", "192.0.2.53")

	// authNS is the nameserver for the delegation zone. It is
	// out-of-bailiwick so the recursor must resolve its address via
	// getAddressesFor (no glue available).
	authNS, err := nameserver.NewWithContext(ctx, "ns.auth.test", "192.0.2.10", r.client)
	if err != nil {
		t.Fatalf("new auth nameserver: %v", err)
	}
	authNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		fqName := dnsutil.Fqdn(name)
		// First query: return CNAME (classless delegation).
		if strings.EqualFold(fqName, "ptr.rev.test.") && qtype == "PTR" {
			return cnamePacket("ptr.rev.test", "ptr.sub.rev.test", "192.0.2.10"), nil
		}
		// Second query (after CNAME follow): return the PTR answer.
		if strings.EqualFold(fqName, "ptr.sub.rev.test.") && qtype == "PTR" {
			return ptrAnswer("ptr.sub.rev.test", "host.example.test"), nil
		}
		return packet.Packet{}, nil
	})

	rootNS, err := nameserver.NewWithContext(ctx, "root.test", "192.0.2.53", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		fqName := dnsutil.Fqdn(name)
		switch {
		// Resolve ns.auth.test address.
		case strings.EqualFold(fqName, "ns.auth.test.") && strings.EqualFold(qtype, "A"):
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			aRR := &dns.A{Hdr: dns.Header{Name: "ns.auth.test.", Class: dns.ClassINET, TTL: 60}}
			aRR.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 10})
			msg.Answer = []dns.RR{aRR}
			return packet.Packet{Msg: msg, AnswerFrom: "192.0.2.53"}, nil
		case strings.EqualFold(fqName, "ns.auth.test.") && strings.EqualFold(qtype, "AAAA"):
			return noDataPacket("ns.auth.test"), nil
		// Referral to rev.test zone for any PTR query.
		case strings.EqualFold(qtype, "PTR"):
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			nsRR := &dns.NS{Hdr: dns.Header{Name: "rev.test.", Class: dns.ClassINET, TTL: 3600}}
			nsRR.Ns = "ns.auth.test."
			msg.Ns = []dns.RR{nsRR}
			return packet.Packet{Msg: msg, AnswerFrom: "192.0.2.53"}, nil
		default:
			return packet.Packet{}, nil
		}
	})

	resp, err := r.Recurse(ctx, "ptr.rev.test.", "PTR", "IN")
	if err != nil {
		t.Fatalf("Recurse: %v", err)
	}
	if resp.Msg == nil {
		t.Fatalf("expected PTR response after CNAME follow, got nil (inProgress leak)")
	}
	ptrs := resp.GetRecords("PTR", "answer")
	if len(ptrs) == 0 {
		t.Fatalf("expected PTR record in answer, got rcode=%s type=%s", resp.Rcode(), resp.Type())
	}
}

func ptrAnswer(owner string, target string) packet.Packet {
	return dnstest.Response(dnstest.NotAuthoritative(),
		dnstest.Answers(dnstest.PTRRR(owner, target)), dnstest.AnswerFrom("192.0.2.10"))
}

func noDataPacket(name string) packet.Packet {
	return dnstest.From(dnstest.NoData(name), dnstest.AnswerFrom("192.0.2.53"))
}

func TestRecurseEmitsRecurseDebugLogs(t *testing.T) {
	ctx, _, log := testhelpers.Context(t)
	state := &recurseState{
		ns: []queryer{
			fakeQueryer{resp: answerPacket("www.example", "203.0.113.50")},
		},
	}

	r := &Recursor{}
	resp, _, err := r.recurse(ctx, "www.example", "A", "IN", state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg == nil {
		t.Fatalf("expected response")
	}

	var recurseTag, recurseQueryTag bool
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		switch entry.Tag {
		case "RECURSE":
			recurseTag = true
		case "RECURSE_QUERY":
			recurseQueryTag = true
		}
	}
	if !recurseTag {
		t.Fatalf("expected RECURSE tag")
	}
	if !recurseQueryTag {
		t.Fatalf("expected RECURSE_QUERY tag")
	}
}

func TestRecurseLogsLoopProtection(t *testing.T) {
	ctx, _, log := testhelpers.Context(t)

	referralQ := &incrementingReferralQueryer{}
	state := &recurseState{
		ns: []queryer{referralQ},
		nsFrom: func(_ context.Context, _ packet.Packet, _ *recurseState) ([]queryer, error) {
			return []queryer{referralQ}, nil
		},
	}

	r := &Recursor{}
	if _, _, err := r.recurse(ctx, "www.example", "A", "IN", state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, entry := range log.Entries() {
		if entry == nil || entry.Tag != "LOOP_PROTECTION" {
			continue
		}
		if zoneName, ok := entry.Args["zone_name"].(string); !ok || zoneName == "" {
			t.Fatalf("expected zone_name in LOOP_PROTECTION args, got %#v", entry.Args)
		}
		if _, ok := entry.Args["name"]; ok {
			t.Fatalf("legacy key name should not be present: %#v", entry.Args)
		}
		return
	}
	t.Fatalf("expected LOOP_PROTECTION tag")
}

func referralPacket(zone string, answerFrom string) packet.Packet {
	ns := dnstest.TTL(3600, dnstest.NSRR(zone, "ns1."+dnsutil.Fqdn(zone)))
	return dnstest.Response(dnstest.NotAuthoritative(),
		dnstest.Authority(ns...), dnstest.AnswerFrom(answerFrom))
}

func nxdomainPacket(answerFrom string) packet.Packet {
	return dnstest.Response(dnstest.NotAuthoritative(), dnstest.NXDOMAIN(), dnstest.AnswerFrom(answerFrom))
}

func answerPacket(qname string, answerFrom string) packet.Packet {
	return dnstest.Response(dnstest.NotAuthoritative(),
		dnstest.Answers(dnstest.ARR(qname, "192.0.2.1")), dnstest.AnswerFrom(answerFrom))
}

func cnamePacket(qname string, target string, answerFrom string) packet.Packet {
	return dnstest.Response(dnstest.NotAuthoritative(),
		dnstest.Answers(dnstest.CNAMERR(qname, target)), dnstest.AnswerFrom(answerFrom))
}

type incrementingReferralQueryer struct {
	count int
}

func (q *incrementingReferralQueryer) QueryWithClass(_ context.Context, _ string, _ string, _ string) (packet.Packet, error) {
	q.count++
	zone := fmt.Sprintf("z%d.example", q.count)
	return referralPacket(zone, fmt.Sprintf("203.0.113.%d", q.count)), nil
}
