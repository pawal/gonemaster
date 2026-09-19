package server

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
	"codeberg.org/miekg/dns/rdata"
)

// The delegation the fixture serves for example.com.
const (
	lookupFixtureNS     = "a.iana-servers.net."
	lookupFixtureIP     = "199.43.135.53"
	lookupFixtureKeyTag = 370
)

// startLookupDNS serves that delegation on loopback and returns resolvers
// pointing at it, so a lookup test drives the real query path offline.
func startLookupDNS(t *testing.T) lookupResolvers {
	t.Helper()

	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}

	ready := make(chan struct{})
	server := &dns.Server{
		PacketConn:        packetConn,
		Handler:           dns.HandlerFunc(answerLookupFixture),
		NotifyStartedFunc: func(context.Context) { close(ready) },
	}
	go func() { _ = server.ListenAndServe() }()
	t.Cleanup(func() {
		server.Shutdown(context.Background())
		_ = packetConn.Close()
	})

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("lookup dns fixture failed to start")
	}

	addr := packetConn.LocalAddr().String()
	host := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "udp", addr)
		},
	}
	return lookupResolvers{servers: []string{addr}, host: host}
}

// answerLookupFixture answers the NS, DS and A queries the lookup makes, and
// an empty NOERROR for anything else.
func answerLookupFixture(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	dnsutil.SetReply(resp, req)
	resp.Authoritative = true
	resp.RecursionAvailable = true

	if len(req.Question) == 1 {
		q := req.Question[0]
		hdr := dns.Header{Name: q.Header().Name, Class: dns.ClassINET, TTL: 3600}
		switch {
		case dns.RRToType(q) == dns.TypeNS && q.Header().Name == "example.com.":
			resp.Answer = []dns.RR{&dns.NS{Hdr: hdr, NS: rdata.NS{Ns: lookupFixtureNS}}}
		case dns.RRToType(q) == dns.TypeDS && q.Header().Name == "example.com.":
			resp.Answer = []dns.RR{&dns.DS{Hdr: hdr, DS: rdata.DS{
				KeyTag:     lookupFixtureKeyTag,
				Algorithm:  13,
				DigestType: 2,
				Digest:     "be74359954660069d5c63d200c39f5603827d7dd02b56f120ee9f3a86764247c",
			}}}
		case dns.RRToType(q) == dns.TypeA && q.Header().Name == lookupFixtureNS:
			resp.Answer = []dns.RR{&dns.A{Hdr: hdr, A: rdata.A{Addr: netip.MustParseAddr(lookupFixtureIP)}}}
		}
	}
	_, _ = resp.WriteTo(w)
}
