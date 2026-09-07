package connectivity

import (
	"context"
	"strings"
	"sync"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// Connectivity05 asks whether an authoritative address delivers the apex
// DNSKEY answer over UDP. The fixtures below stand in for the transport, so a
// handler returns what the transport would have returned: a response carrying
// Protocol "tcp" is one the transport fetched over TCP after truncation, and an
// empty packet is silence. Answers are packed so their wire length is real,
// and the tests derive the expected size and probe payload from the fixture
// rather than hardcoding them.

const testZone = "example"

// dnskeyAnswer builds a DNSKEY answer of keys long keys, packed so that the
// message carries its wire length.
func dnskeyAnswer(t *testing.T, keys int, keyLen int) packet.Packet {
	t.Helper()
	value := strings.Repeat("A", keyLen)
	rrs := make([]dns.RR, keys)
	for i := range rrs {
		rrs[i] = tctest.DNSKEYRR(testZone, dns.RSASHA256, tctest.PublicKey(value))
	}
	p := tctest.Response(
		tctest.Question(testZone, dns.TypeDNSKEY),
		tctest.Answers(rrs...),
	)
	if err := p.Msg.Pack(); err != nil {
		t.Fatalf("pack DNSKEY answer: %v", err)
	}
	return p
}

// largeAnswer is bigger than the default advertised payload and smaller than
// the probe ceiling. smallAnswer fits the default payload.
func largeAnswer(t *testing.T) packet.Packet {
	t.Helper()
	p := dnskeyAnswer(t, 4, 800)
	if got := answerSize(p); got <= defaultEDNSPayload || got > maxEDNSPayload {
		t.Fatalf("fixture invariant: large answer is %d bytes, want %d < size <= %d", got, defaultEDNSPayload, maxEDNSPayload)
	}
	return p
}

func smallAnswer(t *testing.T) packet.Packet {
	t.Helper()
	p := dnskeyAnswer(t, 1, 40)
	if got := answerSize(p); got > defaultEDNSPayload {
		t.Fatalf("fixture invariant: small answer is %d bytes, want at most %d", got, defaultEDNSPayload)
	}
	return p
}

func truncated(p packet.Packet) packet.Packet {
	p.Msg.Truncated = true
	return p
}

func withProtocol(p packet.Packet, proto string) packet.Packet {
	p.Protocol = proto
	return p
}

// queryKind names the four query shapes connectivity05 issues, so a handler can
// answer each one differently and a test can assert which ones were sent.
func queryKind(opts *nameserver.QueryOptions) string {
	switch {
	case opts == nil:
		return "other"
	case opts.EDNSSize != nil:
		return "probe"
	case opts.EDNSDetails != nil:
		return "small"
	case opts.UseVC != nil && *opts.UseVC:
		return "tcp"
	default:
		return "reference"
	}
}

// recorder answers queries by kind and remembers every query it saw. Addresses
// are probed in parallel, so it is mutex-guarded.
type recorder struct {
	mu      sync.Mutex
	answers map[string]packet.Packet
	seen    []tctest.Query
}

func newRecorder(answers map[string]packet.Packet) *recorder {
	return &recorder{answers: answers}
}

func (r *recorder) handle(q tctest.Query) packet.Packet {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, q)
	return r.answers[queryKind(q.Opts)]
}

func (r *recorder) kinds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.seen))
	for i, q := range r.seen {
		out[i] = queryKind(q.Opts)
	}
	return out
}

func (r *recorder) sent(kind string) bool {
	for _, got := range r.kinds() {
		if got == kind {
			return true
		}
	}
	return false
}

func (r *recorder) first(kind string) (tctest.Query, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, q := range r.seen {
		if queryKind(q.Opts) == kind {
			return q, true
		}
	}
	return tctest.Query{}, false
}

// runConnectivity05 runs the testcase against the given addresses, all answered
// by rec.
func runConnectivity05(t *testing.T, rec *recorder, addrs ...string) []*logger.Entry {
	t.Helper()
	ctx := tctest.Context(t)

	nsList := make([]nameserver.Nameserver, len(addrs))
	for i, addr := range addrs {
		nsList[i] = tctest.NS(t, ctx, "ns1.example", addr, rec.handle)
	}
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nsList, nil
	})

	z := zone.Zone{Name: dnsname.New(testZone)}
	entries, err := Connectivity05(ctx, &z)
	if err != nil {
		t.Fatalf("connectivity05: %v", err)
	}
	return entries
}

// requireIntArg reads an integer argument off an entry.
func requireIntArg(t *testing.T, entry *logger.Entry, key string, want int) {
	t.Helper()
	got, ok := entry.Args[key].(int)
	if !ok {
		t.Fatalf("%s: arg %q is %#v, want an int", entry.Tag, key, entry.Args[key])
	}
	if got != want {
		t.Fatalf("%s: arg %q = %d, want %d", entry.Tag, key, got, want)
	}
}

func requireNoCN05Tags(t *testing.T, entries []*logger.Entry) {
	t.Helper()
	if tags := tctest.TagsWithPrefix(entries, "CN05_"); len(tags) > 0 {
		t.Fatalf("expected no CN05_ tags, got %v", tags)
	}
}

func TestConnectivity05AnswerFitsUDP(t *testing.T) {
	answer := smallAnswer(t)
	rec := newRecorder(map[string]packet.Packet{
		"reference": withProtocol(answer, protocolUDP),
	})

	entries := runConnectivity05(t, rec, "192.0.2.1")

	entry := tctest.RequireTag(t, entries, tagAnswerFitsUDP)
	requireIntArg(t, entry, "size", answerSize(answer))
	requireIntArg(t, entry, "payload", defaultEDNSPayload)
	if got := entry.Args["query_type"]; got != deliveryQueryType {
		t.Fatalf("query_type = %#v, want %s", got, deliveryQueryType)
	}
	if rec.sent("probe") {
		t.Fatalf("an answer that fits must not trigger a probe; queries: %v", rec.kinds())
	}
	if got := tctest.TagsWithPrefix(entries, "CN05_"); len(got) != 1 {
		t.Fatalf("expected only %s, got %v", tagAnswerFitsUDP, got)
	}
}

func TestConnectivity05ProbeDeliversLargeAnswer(t *testing.T) {
	answer := largeAnswer(t)
	size := answerSize(answer)
	rec := newRecorder(map[string]packet.Packet{
		"reference": withProtocol(answer, protocolTCP),
		"probe":     withProtocol(answer, protocolUDP),
	})

	entries := runConnectivity05(t, rec, "192.0.2.1")

	needsTCP := tctest.RequireTag(t, entries, tagAnswerNeedsTCP)
	requireIntArg(t, needsTCP, "size", size)
	requireIntArg(t, needsTCP, "payload", defaultEDNSPayload)

	delivered := tctest.RequireTag(t, entries, tagDeliveredUDP)
	requireIntArg(t, delivered, "size", size)
	requireIntArg(t, delivered, "payload", int(probePayload(size)))

	probe, ok := rec.first("probe")
	if !ok {
		t.Fatalf("expected a probe query; queries: %v", rec.kinds())
	}
	opts := probe.Opts
	if opts.EDNSSize == nil || *opts.EDNSSize != probePayload(size) {
		t.Fatalf("probe EDNSSize = %v, want %d", opts.EDNSSize, probePayload(size))
	}
	if opts.Fallback == nil || *opts.Fallback {
		t.Fatalf("probe Fallback = %v, want false", opts.Fallback)
	}
	if opts.Retry == nil || *opts.Retry != 1 {
		t.Fatalf("probe Retry = %v, want 1", opts.Retry)
	}
	if !opts.Diagnostic {
		t.Fatalf("probe must be diagnostic so its loss is not read as a server fault")
	}
}

func TestConnectivity05ProbeGetsNoAnswer(t *testing.T) {
	answer := largeAnswer(t)
	size := answerSize(answer)
	rec := newRecorder(map[string]packet.Packet{
		"reference": withProtocol(answer, protocolTCP),
		"probe":     {},
	})

	entries := runConnectivity05(t, rec, "192.0.2.1")

	tctest.RequireTag(t, entries, tagAnswerNeedsTCP)
	lost := tctest.RequireTag(t, entries, tagNoUDPAnswer)
	requireIntArg(t, lost, "size", size)
	requireIntArg(t, lost, "payload", int(probePayload(size)))
}

func TestConnectivity05ProbeTruncatedCapsTheAnswer(t *testing.T) {
	answer := largeAnswer(t)
	size := answerSize(answer)
	rec := newRecorder(map[string]packet.Packet{
		"reference": withProtocol(answer, protocolTCP),
		"probe":     truncated(withProtocol(dnskeyAnswer(t, 1, 40), protocolUDP)),
	})

	entries := runConnectivity05(t, rec, "192.0.2.1")

	capped := tctest.RequireTag(t, entries, tagServerCapsUDP)
	requireIntArg(t, capped, "size", size)
	requireIntArg(t, capped, "payload", int(probePayload(size)))

	// Capping is the behaviour operators should be steered towards, so neither
	// delivery warning may fire.
	for _, tag := range []string{tagNoUDPAnswer, tagUDPLossSizeDependent} {
		if tctest.Has(entries, tag) {
			t.Fatalf("a capped answer must not emit %s", tag)
		}
	}
}

func TestConnectivity05ProbeRefusedIsInconclusive(t *testing.T) {
	answer := largeAnswer(t)
	refused := tctest.Response(tctest.Question(testZone, dns.TypeDNSKEY), tctest.Rcode(dns.RcodeRefused))
	rec := newRecorder(map[string]packet.Packet{
		"reference": withProtocol(answer, protocolTCP),
		"probe":     withProtocol(refused, protocolUDP),
	})

	entries := runConnectivity05(t, rec, "192.0.2.1")

	tctest.RequireTag(t, entries, tagAnswerNeedsTCP)
	if got := tctest.TagsWithPrefix(entries, "CN05_"); len(got) != 1 {
		t.Fatalf("expected only %s, got %v", tagAnswerNeedsTCP, got)
	}
}

// An address that truncates below the payload it was offered cannot be helped
// by a larger advertisement, so no probe is issued.
func TestConnectivity05SmallTruncatedAnswerSkipsProbe(t *testing.T) {
	answer := smallAnswer(t)
	rec := newRecorder(map[string]packet.Packet{
		"reference": withProtocol(answer, protocolTCP),
	})

	entries := runConnectivity05(t, rec, "192.0.2.1")

	entry := tctest.RequireTag(t, entries, tagAnswerNeedsTCP)
	requireIntArg(t, entry, "size", answerSize(answer))
	if rec.sent("probe") {
		t.Fatalf("expected no probe for an answer within the advertised payload; queries: %v", rec.kinds())
	}
}

// No client advertising 4096 bytes or less can receive an answer above that, so
// no probe is issued either.
func TestConnectivity05HugeAnswerSkipsProbe(t *testing.T) {
	answer := dnskeyAnswer(t, 8, 800)
	if got := answerSize(answer); got <= maxEDNSPayload {
		t.Fatalf("fixture invariant: huge answer is %d bytes, want more than %d", got, maxEDNSPayload)
	}
	rec := newRecorder(map[string]packet.Packet{
		"reference": withProtocol(answer, protocolTCP),
	})

	entries := runConnectivity05(t, rec, "192.0.2.1")

	tctest.RequireTag(t, entries, tagAnswerNeedsTCP)
	if rec.sent("probe") {
		t.Fatalf("expected no probe above the 4096-byte ceiling; queries: %v", rec.kinds())
	}
}

func TestConnectivity05LossIsSizeDependent(t *testing.T) {
	answer := largeAnswer(t)
	rec := newRecorder(map[string]packet.Packet{
		"reference": {},
		"small":     truncated(withProtocol(dnskeyAnswer(t, 1, 40), protocolUDP)),
		"tcp":       withProtocol(answer, protocolTCP),
	})

	entries := runConnectivity05(t, rec, "192.0.2.1")

	entry := tctest.RequireTag(t, entries, tagUDPLossSizeDependent)
	requireIntArg(t, entry, "size", answerSize(answer))
	requireIntArg(t, entry, "payload", defaultEDNSPayload)

	// The small-answer probe must match nameserver13 exactly, or the two
	// testcases stop sharing one exchange.
	small, ok := rec.first("small")
	if !ok {
		t.Fatalf("expected a small-answer query; queries: %v", rec.kinds())
	}
	if small.Opts.EDNSDetails.Size == nil || *small.Opts.EDNSDetails.Size != 512 {
		t.Fatalf("small-answer EDNS size = %v, want 512", small.Opts.EDNSDetails.Size)
	}
	if small.Opts.Fallback == nil || *small.Opts.Fallback {
		t.Fatalf("small-answer Fallback = %v, want false", small.Opts.Fallback)
	}
}

// Silence at 512 bytes as well means EDNS queries do not reach the address at
// all, which other testcases report.
func TestConnectivity05LossWithoutSmallAnswerIsSilent(t *testing.T) {
	rec := newRecorder(map[string]packet.Packet{
		"reference": {},
		"small":     {},
	})

	requireNoCN05Tags(t, runConnectivity05(t, rec, "192.0.2.1"))
}

// Without a TCP answer the size is unknown, and TCP failures belong to
// connectivity02.
func TestConnectivity05LossWithoutTCPAnswerIsSilent(t *testing.T) {
	rec := newRecorder(map[string]packet.Packet{
		"reference": {},
		"small":     truncated(withProtocol(dnskeyAnswer(t, 1, 40), protocolUDP)),
		"tcp":       {},
	})

	requireNoCN05Tags(t, runConnectivity05(t, rec, "192.0.2.1"))
}

func TestConnectivity05GroupsIdenticalOutcomes(t *testing.T) {
	answer := largeAnswer(t)
	rec := newRecorder(map[string]packet.Packet{
		"reference": withProtocol(answer, protocolTCP),
		"probe":     {},
	})

	entries := runConnectivity05(t, rec, "192.0.2.1", "192.0.2.2")

	for _, tag := range []string{tagAnswerNeedsTCP, tagNoUDPAnswer} {
		if got := tctest.Count(entries, tag); got != 1 {
			t.Fatalf("%s: %d entries, want 1", tag, got)
		}
		entry := tctest.RequireTag(t, entries, tag)
		if got := tctest.ServerEndpoints(t, entry.Args); len(got) != 2 {
			t.Fatalf("%s: servers = %v, want two entries", tag, got)
		}
	}
}

// The fits-UDP summary is a single entry reporting the largest answer seen.
func TestConnectivity05FitsUDPSummaryReportsLargestAnswer(t *testing.T) {
	small := dnskeyAnswer(t, 1, 40)
	bigger := dnskeyAnswer(t, 2, 200)
	if answerSize(bigger) <= answerSize(small) {
		t.Fatalf("fixture invariant: bigger answer must exceed the smaller one")
	}

	byAddr := map[string]packet.Packet{
		"192.0.2.1": withProtocol(small, protocolUDP),
		"192.0.2.2": withProtocol(bigger, protocolUDP),
	}
	rec := newRecorder(nil)
	ctx := tctest.Context(t)
	var nsList []nameserver.Nameserver
	for _, addr := range []string{"192.0.2.1", "192.0.2.2"} {
		answer := byAddr[addr]
		nsList = append(nsList, tctest.NS(t, ctx, "ns1.example", addr, func(q tctest.Query) packet.Packet {
			rec.handle(q)
			return answer
		}))
	}
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nsList, nil
	})

	z := zone.Zone{Name: dnsname.New(testZone)}
	entries, err := Connectivity05(ctx, &z)
	if err != nil {
		t.Fatalf("connectivity05: %v", err)
	}

	if got := tctest.Count(entries, tagAnswerFitsUDP); got != 1 {
		t.Fatalf("%s: %d entries, want 1", tagAnswerFitsUDP, got)
	}
	entry := tctest.RequireTag(t, entries, tagAnswerFitsUDP)
	requireIntArg(t, entry, "size", answerSize(bigger))
	if got := tctest.ServerEndpoints(t, entry.Args); len(got) != 2 {
		t.Fatalf("%s: servers = %v, want two entries", tagAnswerFitsUDP, got)
	}
}

func TestConnectivity05IPv6Disabled(t *testing.T) {
	rec := newRecorder(map[string]packet.Packet{
		"reference": withProtocol(smallAnswer(t), protocolUDP),
	})
	profile.Effective().Net.IPv6 = false

	entries := runConnectivity05(t, rec, "2001:db8::1")

	entry := tctest.RequireTag(t, entries, "IPV6_DISABLED")
	if got := entry.Args["query_type"]; got != deliveryQueryType {
		t.Fatalf("query_type = %#v, want %s", got, deliveryQueryType)
	}
	if len(rec.kinds()) != 0 {
		t.Fatalf("expected no queries to a disabled address, got %v", rec.kinds())
	}
	requireNoCN05Tags(t, entries)
}

// A non-NOERROR reference answer says nothing about delivery; the rcode is
// reported by other testcases.
func TestConnectivity05ReferenceRefusedIsSilent(t *testing.T) {
	refused := tctest.Response(tctest.Question(testZone, dns.TypeDNSKEY), tctest.Rcode(dns.RcodeRefused))
	rec := newRecorder(map[string]packet.Packet{
		"reference": withProtocol(refused, protocolUDP),
	})

	requireNoCN05Tags(t, runConnectivity05(t, rec, "192.0.2.1"))
}

// A synthesized response records no transport, and an unknown transport is
// never read as UDP.
func TestConnectivity05UnknownTransportIsSilent(t *testing.T) {
	rec := newRecorder(map[string]packet.Packet{
		"reference": smallAnswer(t),
	})

	requireNoCN05Tags(t, runConnectivity05(t, rec, "192.0.2.1"))
}

func TestProbePayloadRoundsUp(t *testing.T) {
	for _, tc := range []struct {
		size int
		want uint16
	}{
		{1233, 1280},
		{2305, 2560},
		{2304, 2560},
		{4000, 4096},
		{4096, 4096},
	} {
		if got := probePayload(tc.size); got != tc.want {
			t.Fatalf("probePayload(%d) = %d, want %d", tc.size, got, tc.want)
		}
	}
}
