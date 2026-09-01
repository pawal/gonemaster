package transport

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/querytrace"
)

func TestBuildQueryWithClass(t *testing.T) {
	msg := BuildQueryWithClass("example.com", dns.TypeA, dns.ClassCHAOS)
	if len(msg.Question) != 1 {
		t.Fatalf("expected 1 question")
	}
	if msg.Question[0].Header().Class != dns.ClassCHAOS {
		t.Fatalf("unexpected qclass: %d", msg.Question[0].Header().Class)
	}
	if dns.RRToType(msg.Question[0]) != dns.TypeA {
		t.Fatalf("unexpected qtype: %d", dns.RRToType(msg.Question[0]))
	}
	if msg.RecursionDesired {
		t.Fatalf("expected recursion disabled by default")
	}
}

func TestApplyProfileDefaultsDoesNotOverrideExplicit(t *testing.T) {
	prof := dnstest.DefaultProfile(t)

	client := &Client{}
	client.SetRecursionDesired(true)
	client.ApplyProfileDefaults(prof)

	if !client.RecursionDesired {
		t.Fatalf("expected explicit recursion setting to remain true")
	}
}

func TestApplyProfileDefaultsSetsEDNSSizeForDNSSEC(t *testing.T) {
	client := &Client{DNSSEC: true}
	client.ApplyProfileDefaults(nil)

	if client.EDNSSize != constants.EDNSUDPPayloadDNSSECDefault {
		t.Fatalf("unexpected EDNS size: %d", client.EDNSSize)
	}
}

func TestDialerInvalidSource(t *testing.T) {
	client := &Client{SourceIP: "not-an-ip"}
	if _, err := client.Dialer("udp"); err == nil {
		t.Fatalf("expected error for invalid source IP")
	}
}

func TestEnsurePort(t *testing.T) {
	if got := ensurePort("192.0.2.1"); got != "192.0.2.1:53" {
		t.Fatalf("unexpected port for IPv4: %s", got)
	}
	if got := ensurePort("192.0.2.1:5353"); got != "192.0.2.1:5353" {
		t.Fatalf("unexpected port for IPv4 with port: %s", got)
	}
	if got := ensurePort("2001:db8::1"); got != "[2001:db8::1]:53" {
		t.Fatalf("unexpected port for IPv6: %s", got)
	}
}

// singleOPT returns the one OPT record prepareMessage put in the additional section.
func singleOPT(t *testing.T, msg *dns.Msg) *dns.OPT {
	t.Helper()

	var opt *dns.OPT
	for _, rr := range msg.Extra {
		typed, ok := rr.(*dns.OPT)
		if !ok {
			continue
		}
		if opt != nil {
			t.Fatalf("expected a single OPT record in additional section, got more than one")
		}
		opt = typed
	}
	if opt == nil {
		t.Fatalf("expected explicit OPT record in additional section")
	}
	return opt
}

// receivedQuery parses a query captured by a stub server. The handler's message
// exposes header bits but not EDNS fields until the wire bytes are unpacked.
func receivedQuery(t *testing.T, queries <-chan *dns.Msg) *dns.Msg {
	t.Helper()

	var req *dns.Msg
	select {
	case req = <-queries:
	case <-time.After(time.Second):
		t.Fatal("server did not receive the query")
	}

	parsed := &dns.Msg{Data: append([]byte(nil), req.Data...)}
	if err := parsed.Unpack(); err != nil {
		t.Fatalf("unpack received query: %v", err)
	}
	return parsed
}

func TestPrepareMessageEncodesOPT(t *testing.T) {
	do := true
	size1232 := uint16(1232)
	version1 := uint8(1)
	z := uint16(0x1234)
	rcode16 := uint8(16)

	tests := []struct {
		name          string
		client        *Client
		wantRecursion bool
		wantUDPSize   uint16
		wantVersion   uint8
		wantRcode     uint8
		wantDO        bool
		wantZ         uint16
		wantOptions   int
	}{
		{
			name: "EDNS details with client size",
			client: &Client{
				RecursionDesired: true,
				EDNSSize:         1232,
				EDNSDetails: &EDNSDetails{
					Do:      &do,
					Version: &version1,
					Rcode:   &rcode16,
					Data:    []dns.EDNS0{&dns.NSID{}},
				},
			},
			wantRecursion: true,
			wantUDPSize:   1232,
			wantVersion:   1,
			wantRcode:     16,
			wantDO:        true,
			wantOptions:   1,
		},
		{
			name:        "EDNS version with default size",
			client:      &Client{EDNSDetails: &EDNSDetails{Version: &version1}},
			wantUDPSize: dns.MinMsgSize,
			wantVersion: 1,
		},
		{
			name:        "EDNS size 512",
			client:      &Client{EDNSSize: dns.MinMsgSize},
			wantUDPSize: dns.MinMsgSize,
		},
		{
			name: "EDNS Z bits",
			client: &Client{
				EDNSDetails: &EDNSDetails{
					Do:      &do,
					Size:    &size1232,
					Version: &version1,
					Z:       &z,
					Rcode:   &rcode16,
					Data:    []dns.EDNS0{&dns.NSID{}},
				},
			},
			wantUDPSize: 1232,
			wantVersion: 1,
			wantRcode:   16,
			wantDO:      true,
			wantZ:       z & 0x1FFF,
			wantOptions: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prepared := tc.client.prepareMessage(BuildQuery("example.com", dns.TypeA))

			if prepared.RecursionDesired != tc.wantRecursion {
				t.Fatalf("recursion desired: got %v want %v", prepared.RecursionDesired, tc.wantRecursion)
			}
			if len(prepared.Pseudo) != 0 {
				t.Fatalf("expected pseudo section to be empty when explicit OPT is used, got %d entries", len(prepared.Pseudo))
			}
			if prepared.UDPSize != 0 {
				t.Fatalf("expected UDPSize to be moved into OPT record, got %d", prepared.UDPSize)
			}

			opt := singleOPT(t, prepared)
			if got := opt.UDPSize(); got != tc.wantUDPSize {
				t.Fatalf("OPT UDP size: got %d want %d", got, tc.wantUDPSize)
			}
			if got := opt.Version(); got != tc.wantVersion {
				t.Fatalf("OPT version: got %d want %d", got, tc.wantVersion)
			}
			if got := opt.Rcode(); got != uint16(tc.wantRcode) {
				t.Fatalf("OPT extended rcode: got %d want %d", got, tc.wantRcode)
			}
			if opt.Security() != tc.wantDO {
				t.Fatalf("OPT DO bit: got %v want %v", opt.Security(), tc.wantDO)
			}
			if got := opt.Z(); got != tc.wantZ {
				t.Fatalf("OPT Z value: got %d want %d", got, tc.wantZ)
			}
			if len(opt.Options) != tc.wantOptions {
				t.Fatalf("EDNS options in OPT: got %d want %d", len(opt.Options), tc.wantOptions)
			}

			if err := prepared.Pack(); err != nil {
				t.Fatalf("pack prepared query: %v", err)
			}
			var unpacked dns.Msg
			unpacked.Data = append([]byte(nil), prepared.Data...)
			if err := unpacked.Unpack(); err != nil {
				t.Fatalf("unpack prepared query: %v", err)
			}
			if unpacked.UDPSize != tc.wantUDPSize || unpacked.Version != tc.wantVersion || unpacked.Rcode != uint16(tc.wantRcode) {
				t.Fatalf("unpacked EDNS fields: udp=%d version=%d rcode=%d want udp=%d version=%d rcode=%d",
					unpacked.UDPSize, unpacked.Version, unpacked.Rcode, tc.wantUDPSize, tc.wantVersion, tc.wantRcode)
			}
			if wireZ := extractSingleOptZFromWire(t, prepared.Data); wireZ != tc.wantZ {
				t.Fatalf("wire OPT Z mismatch: got %d want %d", wireZ, tc.wantZ)
			}
		})
	}
}

// extractSingleOptZFromWire reads the OPT record's Z bits straight off the
// wire. dns.Msg has no field for an arbitrary Z value, so the packed TTL is the
// only place the encoded bits can be checked.
func extractSingleOptZFromWire(t *testing.T, wire []byte) uint16 {
	t.Helper()

	if len(wire) < 12 {
		t.Fatalf("wire message too short: %d", len(wire))
	}
	var counts [4]uint16
	for i := range counts {
		counts[i] = binary.BigEndian.Uint16(wire[4+2*i:])
	}
	if counts != [4]uint16{1, 0, 0, 1} {
		t.Fatalf("unexpected DNS section counts: qd=%d an=%d ns=%d ar=%d", counts[0], counts[1], counts[2], counts[3])
	}

	offset, ok := skipName(wire, 12)
	if !ok || offset+4 > len(wire) {
		t.Fatalf("failed to parse question section")
	}
	offset, ok = skipName(wire, offset+4) // past qtype and qclass
	if !ok || offset+10 > len(wire) {
		t.Fatalf("failed to parse OPT owner name/header")
	}
	if typ := binary.BigEndian.Uint16(wire[offset:]); typ != dns.TypeOPT {
		t.Fatalf("expected additional record type OPT, got %d", typ)
	}
	ttl := binary.BigEndian.Uint32(wire[offset+4:])
	if rdlen := int(binary.BigEndian.Uint16(wire[offset+8:])); offset+10+rdlen != len(wire) {
		t.Fatalf("invalid OPT rdata length: rdlen=%d offset=%d total=%d", rdlen, offset, len(wire))
	}
	return uint16(ttl & 0x1FFF)
}

// skipName advances past an uncompressed wire-format domain name. These queries
// are built locally, so a compression pointer means the parse went wrong.
func skipName(wire []byte, offset int) (int, bool) {
	for offset < len(wire) {
		length := int(wire[offset])
		if length&0xC0 != 0 {
			return 0, false
		}
		offset++
		if length == 0 {
			return offset, true
		}
		offset += length
	}
	return 0, false
}

func TestExchangeNilMessage(t *testing.T) {
	client := &Client{}
	if _, err := client.Exchange(context.Background(), "192.0.2.53", nil); err == nil {
		t.Fatalf("expected error for nil message")
	}
}

func TestDialerLocalAddr(t *testing.T) {
	client := &Client{SourceIP: "192.0.2.9", SourcePort: 5300}
	udpDialer, err := client.Dialer("udp")
	if err != nil {
		t.Fatalf("dialer udp: %v", err)
	}
	if udpDialer.Timeout != defaultTimeout {
		t.Fatalf("unexpected timeout: %v", udpDialer.Timeout)
	}
	udpAddr, ok := udpDialer.LocalAddr.(*net.UDPAddr)
	if !ok || udpAddr.Port != 5300 {
		t.Fatalf("unexpected udp local addr: %#v", udpDialer.LocalAddr)
	}

	tcpDialer, err := client.Dialer("tcp")
	if err != nil {
		t.Fatalf("dialer tcp: %v", err)
	}
	tcpAddr, ok := tcpDialer.LocalAddr.(*net.TCPAddr)
	if !ok || tcpAddr.Port != 5300 {
		t.Fatalf("unexpected tcp local addr: %#v", tcpDialer.LocalAddr)
	}
}

type acceptCountingListener struct {
	net.Listener
	accepts atomic.Int32
}

func (l *acceptCountingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		l.accepts.Add(1)
	}
	return conn, err
}

// The exchange tests below drive real sockets. Goroutines blocked in socket
// I/O never become durably blocked, so these stay on the real clock rather
// than inside a synctest bubble.
func startUDPDNSServer(t *testing.T, handler dns.HandlerFunc) (string, func()) {
	t.Helper()

	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}

	ready := make(chan struct{})
	server := &dns.Server{
		PacketConn:        packetConn,
		Handler:           handler,
		NotifyStartedFunc: func(_ context.Context) { close(ready) },
	}

	done := make(chan struct{})
	go func() {
		_ = server.ListenAndServe()
		close(done)
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("udp dns server failed to start")
	}

	shutdown := func() {
		server.Shutdown(context.Background())
		_ = packetConn.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Logf("udp dns server shutdown timed out")
		}
	}

	return packetConn.LocalAddr().String(), shutdown
}

func startTCPDNSServer(t *testing.T, handler dns.HandlerFunc, configure func(*dns.Server)) (string, *acceptCountingListener, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	counting := &acceptCountingListener{Listener: listener}

	ready := make(chan struct{})
	server := &dns.Server{
		Net:               "tcp",
		Listener:          counting,
		Handler:           handler,
		NotifyStartedFunc: func(_ context.Context) { close(ready) },
	}
	if configure != nil {
		configure(server)
	}

	done := make(chan struct{})
	go func() {
		_ = server.ListenAndServe()
		close(done)
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("tcp dns server failed to start")
	}

	shutdown := func() {
		server.Shutdown(context.Background())
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Logf("tcp dns server shutdown timed out")
		}
	}

	return listener.Addr().String(), counting, shutdown
}

// startUDPServerOnAddr binds a UDP DNS server to addr (typically the same
// address as an already-started TCP server) and waits until the server is
// ready before returning. Returns a shutdown function the caller must defer.
func startUDPServerOnAddr(t *testing.T, addr string, handler dns.HandlerFunc) func() {
	t.Helper()

	packetConn, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("listen udp on tcp addr: %v", err)
	}

	ready := make(chan struct{})
	server := &dns.Server{
		PacketConn:        packetConn,
		Handler:           handler,
		NotifyStartedFunc: func(_ context.Context) { close(ready) },
	}

	done := make(chan struct{})
	go func() {
		_ = server.ListenAndServe()
		close(done)
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("udp dns server failed to start")
	}

	return func() {
		server.Shutdown(context.Background())
		_ = packetConn.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Logf("udp dns server shutdown timed out")
		}
	}
}

func writeSimpleAResponse(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	dnsutil.SetReply(resp, req)
	if len(req.Question) > 0 {
		a := &dns.A{
			Hdr: dns.Header{
				Name:  req.Question[0].Header().Name,
				Class: dns.ClassINET,
				TTL:   60,
			},
		}
		a.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 10})
		resp.Answer = []dns.RR{a}
	}
	_, _ = resp.WriteTo(w)
}

func TestEffectiveAttemptTimeout(t *testing.T) {
	tests := []struct {
		name        string
		timeout     time.Duration
		retrans     time.Duration
		setFallback bool
		useTCP      bool
		fromUDPFB   bool
		ctxTimeout  time.Duration
		want        time.Duration
	}{
		{
			name:    "uses configured timeout",
			timeout: 250 * time.Millisecond,
			retrans: 40 * time.Millisecond,
			useTCP:  true,
			want:    250 * time.Millisecond,
		},
		{
			name:    "uses retrans budget for UDP",
			timeout: 250 * time.Millisecond,
			retrans: 40 * time.Millisecond,
			want:    40 * time.Millisecond,
		},
		{
			name:        "does not cap UDP for fallback",
			timeout:     5 * time.Second,
			retrans:     3 * time.Second,
			setFallback: true,
			want:        3 * time.Second,
		},
		{
			name:      "uses retrans budget for TCP fallback",
			timeout:   250 * time.Millisecond,
			retrans:   40 * time.Millisecond,
			useTCP:    true,
			fromUDPFB: true,
			want:      40 * time.Millisecond,
		},
		{
			name:    "falls back to retrans when timeout unset",
			retrans: 80 * time.Millisecond,
			want:    80 * time.Millisecond,
		},
		{
			name:       "respects context deadline",
			timeout:    300 * time.Millisecond,
			retrans:    200 * time.Millisecond,
			useTCP:     true,
			ctxTimeout: 120 * time.Millisecond,
			want:       120 * time.Millisecond,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &Client{Timeout: tc.timeout, Retrans: tc.retrans}
			if tc.setFallback {
				client.SetFallback(true)
			}

			ctx := context.Background()
			if tc.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.ctxTimeout)
				defer cancel()
			}

			got := client.effectiveAttemptTimeout(ctx, tc.useTCP, tc.fromUDPFB)
			if tc.ctxTimeout > 0 {
				// Deadline clamps the budget; the remainder shrinks with wall clock.
				if got <= 0 || got > tc.want {
					t.Fatalf("attempt timeout %v outside (0, %v]", got, tc.want)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("attempt timeout: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestExchangeTCPDoesNotClampTimeoutToRetrans(t *testing.T) {
	serverAddr, _, shutdown := startTCPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		time.Sleep(120 * time.Millisecond)
		writeSimpleAResponse(w, req)
	}, nil)
	defer shutdown()

	client := &Client{}
	client.SetUseTCP(true)
	client.SetFallback(false)
	client.SetRetries(0)
	client.SetTimeout(300 * time.Millisecond)
	client.SetRetrans(40 * time.Millisecond)

	start := time.Now()
	_, err := client.Exchange(context.Background(), serverAddr, BuildQuery("tcp-timeout-vs-retrans.example", dns.TypeA))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected successful response within timeout, got %v", err)
	}
	if elapsed < 100*time.Millisecond || elapsed > 300*time.Millisecond {
		t.Fatalf("expected exchange around server delay without retrans clamp, took %v", elapsed)
	}
}

func TestExchangeFallbackTCPOnTruncatedUDP(t *testing.T) {
	serverAddr, listener, shutdownTCP := startTCPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		writeSimpleAResponse(w, req)
	}, nil)
	defer shutdownTCP()

	defer startUDPServerOnAddr(t, serverAddr, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		resp := new(dns.Msg)
		dnsutil.SetReply(resp, req)
		resp.Truncated = true
		_, _ = resp.WriteTo(w)
	})()

	client := &Client{}
	client.SetUseTCP(false)
	client.SetFallback(true)
	client.SetRetries(0)
	client.SetTimeout(1 * time.Second)
	client.SetRetrans(40 * time.Millisecond)

	_, err := client.Exchange(context.Background(), serverAddr, BuildQuery("tcp-fallback-truncated.example", dns.TypeA))
	if err != nil {
		t.Fatalf("expected TCP fallback success after truncated UDP response, got %v", err)
	}
	if got := listener.accepts.Load(); got < 1 {
		t.Fatalf("expected TCP fallback attempt, got %d TCP accepts", got)
	}
}

// TestExchangeDefaultQueryHasNoEDNSAndRDUnset checks that a default query is
// sent without an EDNS OPT and with RD unset, per DNSQueryAndResponseDefaults.
// In particular the 4096-byte receive-buffer bump in prepareWireMessage must
// not leak an OPT onto the wire.
func TestExchangeDefaultQueryHasNoEDNSAndRDUnset(t *testing.T) {
	queries := make(chan *dns.Msg, 1)
	serverAddr, shutdown := startUDPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		select {
		case queries <- req:
		default:
		}
		writeSimpleAResponse(w, req)
	})
	defer shutdown()

	client := &Client{}
	client.SetUseTCP(false)
	client.SetRetries(0)
	client.SetTimeout(1 * time.Second)

	if _, err := client.Exchange(context.Background(), serverAddr, BuildQuery("default-no-edns.example", dns.TypeA)); err != nil {
		t.Fatalf("expected successful UDP response, got %v", err)
	}

	hdr := receivedQuery(t, queries)

	if hdr.UDPSize != 0 {
		t.Fatalf("expected no EDNS OPT on default query, got advertised UDP size %d", hdr.UDPSize)
	}
	if hdr.Security {
		t.Fatalf("expected DO bit unset on default query")
	}
	if hdr.Version != 0 {
		t.Fatalf("expected EDNS version 0 on default query, got %d", hdr.Version)
	}
	if hdr.RecursionDesired {
		t.Fatalf("expected RD bit unset on default query")
	}
}

// TestExchangeTruncatedUDPFallsBackToPlainTCPExactlyOnce checks that a
// truncated UDP response triggers exactly one plain-TCP requery, with no
// EDNS upgrade and no EDNS-on-TC UDP requery (DNSQueryAndResponseDefaults).
func TestExchangeTruncatedUDPFallsBackToPlainTCPExactlyOnce(t *testing.T) {
	tcpQueries := make(chan *dns.Msg, 1)
	serverAddr, listener, shutdownTCP := startTCPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		select {
		case tcpQueries <- req:
		default:
		}
		writeSimpleAResponse(w, req)
	}, nil)
	defer shutdownTCP()

	var udpQueries atomic.Int32
	defer startUDPServerOnAddr(t, serverAddr, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		udpQueries.Add(1)
		resp := new(dns.Msg)
		dnsutil.SetReply(resp, req)
		resp.Truncated = true
		_, _ = resp.WriteTo(w)
	})()

	client := &Client{}
	client.SetUseTCP(false)
	client.SetFallback(true)
	client.SetRetries(0)
	client.SetTimeout(1 * time.Second)
	client.SetRetrans(40 * time.Millisecond)

	if _, err := client.Exchange(context.Background(), serverAddr, BuildQuery("tc-plain-tcp-fallback.example", dns.TypeA)); err != nil {
		t.Fatalf("expected TCP fallback success after truncated UDP, got %v", err)
	}

	if got := listener.accepts.Load(); got != 1 {
		t.Fatalf("expected exactly one TCP fallback attempt, got %d", got)
	}
	if got := udpQueries.Load(); got != 1 {
		t.Fatalf("expected exactly one UDP query (no EDNS-on-TC requery), got %d", got)
	}

	tcpHdr := receivedQuery(t, tcpQueries)
	if tcpHdr.UDPSize != 0 || tcpHdr.Security {
		t.Fatalf("expected plain TCP fallback query without EDNS, got UDPSize=%d security=%t", tcpHdr.UDPSize, tcpHdr.Security)
	}
}

func TestExchangeAcceptsOversizedUDPWithoutTCPFallback(t *testing.T) {
	serverAddr, listener, shutdownTCP := startTCPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		writeSimpleAResponse(w, req)
	}, nil)
	defer shutdownTCP()

	defer startUDPServerOnAddr(t, serverAddr, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		resp := new(dns.Msg)
		dnsutil.SetReply(resp, req)
		if len(req.Question) > 0 {
			a := &dns.A{
				Hdr: dns.Header{
					Name:  req.Question[0].Header().Name,
					Class: dns.ClassINET,
					TTL:   60,
				},
			}
			a.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 10})
			resp.Answer = append(resp.Answer, a)
		}

		// Keep the response valid but larger than the classic 512-byte UDP
		// receive buffer used when the request has no EDNS.
		for i := range 64 {
			additional := &dns.A{
				Hdr: dns.Header{
					Name:  "extra.example.",
					Class: dns.ClassINET,
					TTL:   60,
				},
			}
			additional.Addr = netip.AddrFrom4([4]byte{192, 0, 2, byte(i + 1)})
			resp.Extra = append(resp.Extra, additional)
		}

		_, _ = resp.WriteTo(w)
	})()

	client := &Client{}
	client.SetUseTCP(false)
	client.SetFallback(true)
	client.SetRetries(0)
	client.SetTimeout(1 * time.Second)
	client.SetRetrans(40 * time.Millisecond)

	resp, err := client.Exchange(context.Background(), serverAddr, BuildQuery("udp-oversized-response.example", dns.TypeA))
	if err != nil {
		t.Fatalf("expected successful UDP response, got %v", err)
	}
	if resp.Msg == nil || len(resp.Msg.Answer) == 0 {
		t.Fatalf("expected answer records in UDP response, got %#v", resp.Msg)
	}
	if got := listener.accepts.Load(); got != 0 {
		t.Fatalf("expected no TCP fallback attempt, got %d TCP accepts", got)
	}
}

func TestExchangeDoesNotFallbackTCPOnUDPFailure(t *testing.T) {
	serverAddr, listener, shutdownTCP := startTCPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		writeSimpleAResponse(w, req)
	}, nil)
	defer shutdownTCP()

	defer startUDPServerOnAddr(t, serverAddr, func(_ context.Context, _ dns.ResponseWriter, _ *dns.Msg) {
		// Intentionally blackhole UDP queries so fallback path is exercised.
	})()

	client := &Client{}
	client.SetUseTCP(false)
	client.SetFallback(true)
	client.SetRetries(0)
	client.SetTimeout(300 * time.Millisecond)
	client.SetRetrans(40 * time.Millisecond)

	start := time.Now()
	_, err := client.Exchange(context.Background(), serverAddr, BuildQuery("tcp-fallback-udp-cap.example", dns.TypeA))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected UDP failure without TCP fallback")
	}
	if got := listener.accepts.Load(); got != 0 {
		t.Fatalf("expected no TCP fallback on UDP error, got %d TCP accepts", got)
	}
	if elapsed < 20*time.Millisecond || elapsed > 300*time.Millisecond {
		t.Fatalf("expected exchange to fail within UDP timeout budget, took %v", elapsed)
	}
}

func TestExchangeReturnsPromptlyOnContextCancel(t *testing.T) {
	serverAddr, shutdown := startUDPDNSServer(t, func(_ context.Context, _ dns.ResponseWriter, _ *dns.Msg) {
		// Intentionally return no response to force a client-side read wait.
	})
	defer shutdown()

	client := &Client{}
	client.SetUseTCP(false)
	client.SetFallback(false)
	client.SetRetries(0)
	client.SetTimeout(2 * time.Second)
	client.SetRetrans(2 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(40*time.Millisecond, cancel)

	start := time.Now()
	_, err := client.Exchange(ctx, serverAddr, BuildQuery("cancel-fast.example", dns.TypeA))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
	if elapsed > 700*time.Millisecond {
		t.Fatalf("expected cancellation to stop exchange promptly, took %v (err=%v)", elapsed, err)
	}
}

// TestExchangeEmitsAttemptEventPerTimeout is the step-3 verification: a
// nameserver that accepts UDP packets but never answers must produce exactly
// one timeout AttemptEvent per attempt in the (1 + retries) budget. This
// per-attempt timeout timing is something the nameserver layer's aggregate
// per-Exchange recording cannot break down, so the trace is where individual
// attempt latency becomes visible.
func TestExchangeEmitsAttemptEventPerTimeout(t *testing.T) {
	// Black-hole server: reads the query, never writes a reply.
	addr, shutdown := startUDPDNSServer(t, func(_ context.Context, _ dns.ResponseWriter, _ *dns.Msg) {})
	defer shutdown()

	rec := &dnstest.RecordingTrace{}
	ctx := querytrace.WithContext(context.Background(), rec)

	client := &Client{}
	client.SetUseTCP(false)
	client.SetFallback(false)
	client.SetRetries(2) // attempts = 1 + 2 = 3
	client.SetTimeout(100 * time.Millisecond)
	client.SetRetrans(50 * time.Millisecond)

	_, err := client.Exchange(ctx, addr, BuildQuery("timeout.example.", dns.TypeSOA))
	if err == nil {
		t.Fatal("expected a timeout error from a non-responding server, got nil")
	}

	events := rec.Attempts()
	if len(events) != 3 {
		t.Fatalf("expected 3 attempt events (1 + 2 retries), got %d: %+v", len(events), events)
	}
	for i, ev := range events {
		if ev.Outcome != querytrace.OutcomeTimeout {
			t.Errorf("attempt %d: expected OutcomeTimeout, got %q (err %q)", i+1, ev.Outcome, ev.Err)
		}
		if ev.Attempt != i+1 {
			t.Errorf("event index %d: expected Attempt=%d, got %d", i, i+1, ev.Attempt)
		}
		if ev.Protocol != "udp" {
			t.Errorf("attempt %d: expected protocol udp, got %q", i+1, ev.Protocol)
		}
		if ev.NSAddr != addr {
			t.Errorf("attempt %d: expected NSAddr %q, got %q", i+1, addr, ev.NSAddr)
		}
		if ev.QType != "SOA" {
			t.Errorf("attempt %d: expected QType SOA, got %q", i+1, ev.QType)
		}
		if ev.Elapsed <= 0 {
			t.Errorf("attempt %d: expected positive Elapsed for a timed-out attempt, got %v", i+1, ev.Elapsed)
		}
	}
}

// TestExchangeWithRetriesDisabledMakesOneAttempt pins two transport promises
// that hold for every caller: a client with retries explicitly set to zero
// makes exactly one attempt, and ApplyProfileDefaults does not restore the
// profile's retry count over that explicit zero. The per-attempt window is
// asserted separately, because withdrawing retransmits must not also shorten
// how long a server has to answer.
func TestExchangeWithRetriesDisabledMakesOneAttempt(t *testing.T) {
	// Black-hole server: reads the query, never writes a reply.
	addr, shutdown := startUDPDNSServer(t, func(_ context.Context, _ dns.ResponseWriter, _ *dns.Msg) {})
	defer shutdown()

	prof := dnstest.DefaultProfile(t)
	if prof.Resolver.Defaults.Retry == 0 {
		t.Fatal("shipped profile must carry a non-zero retry for this test to mean anything")
	}

	rec := &dnstest.RecordingTrace{}
	ctx := querytrace.WithContext(context.Background(), rec)

	client := &Client{}
	client.SetUseTCP(false)
	client.SetFallback(false)
	client.SetTimeout(100 * time.Millisecond)
	client.SetRetrans(50 * time.Millisecond)
	client.SetRetries(0)
	client.ApplyProfileDefaults(prof)

	if client.Retries != 0 {
		t.Fatalf("ApplyProfileDefaults restored Retries to %d, so an explicit zero is a no-op", client.Retries)
	}

	_, err := client.Exchange(ctx, addr, BuildQuery("degraded.example.", dns.TypeSOA))
	if err == nil {
		t.Fatal("expected a timeout error from a non-responding server, got nil")
	}

	events := rec.Attempts()
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 attempt event with retries disabled, got %d: %+v", len(events), events)
	}
	if events[0].Outcome != querytrace.OutcomeTimeout {
		t.Errorf("expected OutcomeTimeout, got %q (err %q)", events[0].Outcome, events[0].Err)
	}
	if events[0].Elapsed < 40*time.Millisecond {
		t.Errorf("expected the attempt to wait out its ~50ms window, got %v", events[0].Elapsed)
	}
}

// TestExchangeEmitsAttemptEventOnSuccess confirms a single successful query
// emits exactly one OutcomeOK event (no spurious retries are traced) and that
// the query name/type are reported correctly.
func TestExchangeEmitsAttemptEventOnSuccess(t *testing.T) {
	addr, shutdown := startUDPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		writeSimpleAResponse(w, req)
	})
	defer shutdown()

	rec := &dnstest.RecordingTrace{}
	ctx := querytrace.WithContext(context.Background(), rec)

	client := &Client{}
	client.SetRetries(2)
	client.SetTimeout(time.Second)

	if _, err := client.Exchange(ctx, addr, BuildQuery("ok.example.", dns.TypeA)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := rec.Attempts()
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 attempt event on success, got %d: %+v", len(events), events)
	}
	if events[0].Outcome != querytrace.OutcomeOK {
		t.Errorf("expected OutcomeOK, got %q (err %q)", events[0].Outcome, events[0].Err)
	}
	if events[0].QType != "A" {
		t.Errorf("expected QType A, got %q", events[0].QType)
	}
}

// TestExchangeWithoutTraceDoesNotPanic confirms the nil-trace path (tracing
// disabled) is a no-op and changes nothing about a normal exchange.
func TestExchangeWithoutTraceDoesNotPanic(t *testing.T) {
	addr, shutdown := startUDPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		writeSimpleAResponse(w, req)
	})
	defer shutdown()

	client := &Client{}
	client.SetTimeout(time.Second)

	if _, err := client.Exchange(context.Background(), addr, BuildQuery("notrace.example.", dns.TypeA)); err != nil {
		t.Fatalf("unexpected error with tracing disabled: %v", err)
	}
}

// A client is shared by every query to one nameserver, so a write-through would
// apply one caller's profile defaults to the rest, and race them.
func TestExchangeDoesNotMutateClient(t *testing.T) {
	addr, shutdown := startUDPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		writeSimpleAResponse(w, req)
	})
	defer shutdown()

	// Every default-carrying field is unset, so a write-through would show.
	client := &Client{}
	before := *client

	if _, err := client.Exchange(context.Background(), addr, BuildQuery("nomutate.example.", dns.TypeA)); err != nil {
		t.Fatalf("exchange: %v", err)
	}

	if *client != before {
		t.Errorf("Exchange mutated the client:\n got %+v\nwant %+v", *client, before)
	}
}

// Under -race this fails if Exchange writes to the shared client.
func TestExchangeConcurrentOnOneClient(t *testing.T) {
	addr, shutdown := startUDPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		writeSimpleAResponse(w, req)
	})
	defer shutdown()

	client := &Client{}
	client.SetTimeout(2 * time.Second)

	const concurrency = 8
	errs := make(chan error, concurrency)
	start := make(chan struct{})
	for range concurrency {
		go func() {
			<-start
			_, err := client.Exchange(context.Background(), addr, BuildQuery("concurrent.example.", dns.TypeA))
			errs <- err
		}()
	}
	close(start)

	for range concurrency {
		if err := <-errs; err != nil {
			t.Errorf("concurrent exchange: %v", err)
		}
	}
}

// The DNS library carries Z's top two bits as the named CO and DE flags and
// masks them out of SetZ, but EDNSDetails.Z promises all 15 bits.
func TestPrepareMessageZCarriesCOAndDE(t *testing.T) {
	cases := []struct {
		name           string
		z              uint16
		wantZ          uint16
		wantCompactAns bool
		wantDelegation bool
	}{
		{"reserved bits only", 0x0003, 0x0003, false, false},
		{"CO only", 0x4000, 0x0000, true, false},
		{"DE only", 0x2000, 0x0000, false, true},
		{"CO, DE and reserved bits", 0x6003, 0x0003, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			z := tc.z
			client := &Client{EDNSDetails: &EDNSDetails{Z: &z}}
			prepared := client.prepareMessage(BuildQuery("z-flags.example.", dns.TypeA))

			opt := singleOPT(t, prepared)
			if got := opt.Z(); got != tc.wantZ {
				t.Errorf("OPT Z = %#04x, want %#04x", got, tc.wantZ)
			}
			if got := opt.CompactAnswers(); got != tc.wantCompactAns {
				t.Errorf("CO = %v, want %v", got, tc.wantCompactAns)
			}
			if got := opt.Delegation(); got != tc.wantDelegation {
				t.Errorf("DE = %v, want %v", got, tc.wantDelegation)
			}
		})
	}
}

// The flags must survive packing, not just sit on the prepared message.
func TestPrepareMessageZReachesTheWire(t *testing.T) {
	queries := make(chan *dns.Msg, 1)
	serverAddr, shutdown := startUDPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		select {
		case queries <- req:
		default:
		}
		writeSimpleAResponse(w, req)
	})
	defer shutdown()

	z := uint16(0x6003)
	client := &Client{EDNSDetails: &EDNSDetails{Z: &z}}
	client.SetRetries(0)
	client.SetTimeout(time.Second)

	if _, err := client.Exchange(context.Background(), serverAddr, BuildQuery("z-flags-wire.example.", dns.TypeA)); err != nil {
		t.Fatalf("exchange: %v", err)
	}

	got := receivedQuery(t, queries)
	if !got.CompactAnswers {
		t.Error("CO bit did not reach the wire")
	}
	if !got.Delegation {
		t.Error("DE bit did not reach the wire")
	}
	if got.UDPSize == 0 {
		t.Error("EDNS OPT did not reach the wire")
	}
}

// The transport that carried a reply is not visible in the DNS message, so a
// TC-triggered TCP requery is indistinguishable from a plain UDP exchange
// unless the packet records it.
func TestExchangeRecordsProtocol(t *testing.T) {
	t.Run("udp", func(t *testing.T) {
		addr, shutdown := startUDPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
			writeSimpleAResponse(w, req)
		})
		defer shutdown()

		client := &Client{}
		client.SetRetries(0)
		client.SetTimeout(time.Second)

		pkt, err := client.Exchange(context.Background(), addr, BuildQuery("udp-protocol.example.", dns.TypeA))
		if err != nil {
			t.Fatalf("exchange: %v", err)
		}
		if pkt.Protocol != "udp" {
			t.Errorf("Protocol = %q, want udp", pkt.Protocol)
		}
	})

	t.Run("forced tcp", func(t *testing.T) {
		addr, _, shutdown := startTCPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
			writeSimpleAResponse(w, req)
		}, nil)
		defer shutdown()

		client := &Client{}
		client.SetUseTCP(true)
		client.SetRetries(0)
		client.SetTimeout(time.Second)

		pkt, err := client.Exchange(context.Background(), addr, BuildQuery("tcp-protocol.example.", dns.TypeA))
		if err != nil {
			t.Fatalf("exchange: %v", err)
		}
		if pkt.Protocol != "tcp" {
			t.Errorf("Protocol = %q, want tcp", pkt.Protocol)
		}
	})

	t.Run("tcp after truncated udp", func(t *testing.T) {
		addr, _, shutdownTCP := startTCPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
			writeSimpleAResponse(w, req)
		}, nil)
		defer shutdownTCP()

		defer startUDPServerOnAddr(t, addr, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
			resp := new(dns.Msg)
			dnsutil.SetReply(resp, req)
			resp.Truncated = true
			_, _ = resp.WriteTo(w)
		})()

		client := &Client{}
		client.SetUseTCP(false)
		client.SetFallback(true)
		client.SetRetries(0)
		client.SetTimeout(time.Second)
		client.SetRetrans(40 * time.Millisecond)

		pkt, err := client.Exchange(context.Background(), addr, BuildQuery("fallback-protocol.example.", dns.TypeA))
		if err != nil {
			t.Fatalf("exchange: %v", err)
		}
		if pkt.Protocol != "tcp" {
			t.Errorf("Protocol = %q, want tcp after the TC fallback", pkt.Protocol)
		}
	})
}
