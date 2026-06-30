package transport

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/profile"
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
	prof, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}

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

func TestPrepareMessageWithEDNSDetails(t *testing.T) {
	do := true
	version := uint8(1)
	rcode := uint8(16)

	client := &Client{
		RecursionDesired: true,
		EDNSSize:         1232,
		EDNSDetails: &EDNSDetails{
			Do:      &do,
			Version: &version,
			Rcode:   &rcode,
			Data: []dns.EDNS0{
				&dns.NSID{},
			},
		},
	}

	msg := BuildQuery("example.com", dns.TypeA)
	prepared := client.prepareMessage(msg)

	if !prepared.RecursionDesired {
		t.Fatalf("expected recursion desired to be true")
	}
	if len(prepared.Pseudo) != 0 {
		t.Fatalf("expected pseudo section to be empty when explicit OPT is used, got %d entries", len(prepared.Pseudo))
	}
	if prepared.UDPSize != 0 {
		t.Fatalf("expected UDPSize to be moved into OPT record, got %d", prepared.UDPSize)
	}
	var opt *dns.OPT
	for _, rr := range prepared.Extra {
		if typed, ok := rr.(*dns.OPT); ok {
			opt = typed
			break
		}
	}
	if opt == nil {
		t.Fatalf("expected explicit OPT record in additional section")
	}
	if got := opt.UDPSize(); got != 1232 {
		t.Fatalf("unexpected OPT UDP size: got %d want 1232", got)
	}
	if !opt.Security() {
		t.Fatalf("expected OPT DO bit set")
	}
	if got := opt.Version(); got != 1 {
		t.Fatalf("unexpected OPT version: got %d want 1", got)
	}
	if got := opt.Rcode(); got != 16 {
		t.Fatalf("unexpected OPT extended rcode: got %d want 16", got)
	}
	if len(opt.Options) != 1 {
		t.Fatalf("expected one EDNS option in OPT, got %d", len(opt.Options))
	}

	if err := prepared.Pack(); err != nil {
		t.Fatalf("pack prepared query: %v", err)
	}
	var unpacked dns.Msg
	unpacked.Data = append([]byte(nil), prepared.Data...)
	if err := unpacked.Unpack(); err != nil {
		t.Fatalf("unpack prepared query: %v", err)
	}
	if unpacked.UDPSize != 1232 || unpacked.Version != 1 || unpacked.Rcode != 16 {
		t.Fatalf("unexpected unpacked EDNS fields: udp=%d version=%d rcode=%d", unpacked.UDPSize, unpacked.Version, unpacked.Rcode)
	}
}

func TestPrepareMessageWithEDNSVersionAndDefaultSizeEncodesOPT(t *testing.T) {
	version := uint8(1)
	client := &Client{
		EDNSDetails: &EDNSDetails{
			Version: &version,
		},
	}

	prepared := client.prepareMessage(BuildQuery("example.com", dns.TypeA))

	var opt *dns.OPT
	for _, rr := range prepared.Extra {
		if typed, ok := rr.(*dns.OPT); ok {
			opt = typed
			break
		}
	}
	if opt == nil {
		t.Fatalf("expected explicit OPT record in additional section")
	}
	if got := opt.UDPSize(); got != dns.MinMsgSize {
		t.Fatalf("unexpected OPT UDP size: got %d want %d", got, dns.MinMsgSize)
	}
	if got := opt.Version(); got != 1 {
		t.Fatalf("unexpected OPT version: got %d want 1", got)
	}

	if err := prepared.Pack(); err != nil {
		t.Fatalf("pack prepared query: %v", err)
	}
	var unpacked dns.Msg
	unpacked.Data = append([]byte(nil), prepared.Data...)
	if err := unpacked.Unpack(); err != nil {
		t.Fatalf("unpack prepared query: %v", err)
	}
	if unpacked.UDPSize != dns.MinMsgSize || unpacked.Version != 1 {
		t.Fatalf("unexpected unpacked EDNS fields: udp=%d version=%d", unpacked.UDPSize, unpacked.Version)
	}
}

func TestPrepareMessageWithEDNSSize512EncodesOPT(t *testing.T) {
	client := &Client{EDNSSize: dns.MinMsgSize}

	prepared := client.prepareMessage(BuildQuery("example.com", dns.TypeA))

	var opt *dns.OPT
	for _, rr := range prepared.Extra {
		if typed, ok := rr.(*dns.OPT); ok {
			opt = typed
			break
		}
	}
	if opt == nil {
		t.Fatalf("expected explicit OPT record in additional section")
	}
	if got := opt.UDPSize(); got != dns.MinMsgSize {
		t.Fatalf("unexpected OPT UDP size: got %d want %d", got, dns.MinMsgSize)
	}

	if err := prepared.Pack(); err != nil {
		t.Fatalf("pack prepared query: %v", err)
	}
	var unpacked dns.Msg
	unpacked.Data = append([]byte(nil), prepared.Data...)
	if err := unpacked.Unpack(); err != nil {
		t.Fatalf("unpack prepared query: %v", err)
	}
	if unpacked.UDPSize != dns.MinMsgSize {
		t.Fatalf("unexpected unpacked UDP size: got %d want %d", unpacked.UDPSize, dns.MinMsgSize)
	}
}

func TestPrepareMessageWithEDNSZEncodesSingleOPT(t *testing.T) {
	do := true
	size := uint16(1232)
	version := uint8(1)
	z := uint16(0x1234)
	rcode := uint8(16)

	client := &Client{
		EDNSDetails: &EDNSDetails{
			Do:      &do,
			Size:    &size,
			Version: &version,
			Z:       &z,
			Rcode:   &rcode,
			Data: []dns.EDNS0{
				&dns.NSID{},
			},
		},
	}

	prepared := client.prepareMessage(BuildQuery("example.com", dns.TypeA))

	if len(prepared.Pseudo) != 0 {
		t.Fatalf("expected pseudo section to be empty when explicit OPT is used, got %d entries", len(prepared.Pseudo))
	}
	if prepared.UDPSize != 0 {
		t.Fatalf("expected UDPSize to be moved into OPT record, got %d", prepared.UDPSize)
	}

	var opt *dns.OPT
	for _, rr := range prepared.Extra {
		if typed, ok := rr.(*dns.OPT); ok {
			opt = typed
			break
		}
	}
	if opt == nil {
		t.Fatalf("expected explicit OPT record in additional section")
	}
	if got := opt.Z(); got != (z & 0x1FFF) {
		t.Fatalf("unexpected OPT Z value: got %d want %d", got, z&0x1FFF)
	}
	if got := opt.UDPSize(); got != size {
		t.Fatalf("unexpected OPT UDP size: got %d want %d", got, size)
	}
	if got := opt.Version(); got != version {
		t.Fatalf("unexpected OPT version: got %d want %d", got, version)
	}
	if !opt.Security() {
		t.Fatalf("expected OPT DO bit set")
	}
	if got := opt.Rcode(); got != 16 {
		t.Fatalf("unexpected OPT extended rcode: got %d want 16", got)
	}
	if len(opt.Options) != 1 {
		t.Fatalf("expected one EDNS option in OPT, got %d", len(opt.Options))
	}

	if err := prepared.Pack(); err != nil {
		t.Fatalf("pack prepared query: %v", err)
	}
	wireZ := extractSingleOptZFromWire(t, prepared.Data)
	if wireZ != (z & 0x1FFF) {
		t.Fatalf("wire OPT Z mismatch: got %d want %d", wireZ, z&0x1FFF)
	}
}

func extractSingleOptZFromWire(t *testing.T, wire []byte) uint16 {
	t.Helper()

	if len(wire) < 12 {
		t.Fatalf("wire message too short: %d", len(wire))
	}
	qd := binary.BigEndian.Uint16(wire[4:6])
	an := binary.BigEndian.Uint16(wire[6:8])
	ns := binary.BigEndian.Uint16(wire[8:10])
	ar := binary.BigEndian.Uint16(wire[10:12])
	if qd != 1 || an != 0 || ns != 0 || ar != 1 {
		t.Fatalf("unexpected DNS section counts: qd=%d an=%d ns=%d ar=%d", qd, an, ns, ar)
	}

	offset, ok := skipName(wire, 12)
	if !ok || offset+4 > len(wire) {
		t.Fatalf("failed to parse question section")
	}
	offset += 4 // qtype + qclass

	offset, ok = skipName(wire, offset)
	if !ok || offset+10 > len(wire) {
		t.Fatalf("failed to parse OPT owner name/header")
	}
	typ := binary.BigEndian.Uint16(wire[offset : offset+2])
	offset += 2
	_ = binary.BigEndian.Uint16(wire[offset : offset+2]) // class
	offset += 2
	ttl := binary.BigEndian.Uint32(wire[offset : offset+4])
	offset += 4
	rdlen := int(binary.BigEndian.Uint16(wire[offset : offset+2]))
	offset += 2

	if typ != dns.TypeOPT {
		t.Fatalf("expected additional record type OPT, got %d", typ)
	}
	if offset+rdlen > len(wire) {
		t.Fatalf("invalid OPT rdata length: rdlen=%d offset=%d total=%d", rdlen, offset, len(wire))
	}

	return uint16(ttl & 0x1FFF)
}

func skipName(wire []byte, offset int) (int, bool) {
	for {
		if offset >= len(wire) {
			return 0, false
		}
		length := wire[offset]
		offset++
		switch length & 0xC0 {
		case 0x00:
			if length == 0 {
				return offset, true
			}
			offset += int(length)
			if offset > len(wire) {
				return 0, false
			}
		case 0xC0:
			if offset >= len(wire) {
				return 0, false
			}
			return offset + 1, true
		default:
			return 0, false
		}
	}
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

func TestEffectiveAttemptTimeoutUsesConfiguredTimeout(t *testing.T) {
	client := &Client{
		Timeout: 250 * time.Millisecond,
		Retrans: 40 * time.Millisecond,
	}

	got := client.effectiveAttemptTimeout(context.Background(), true, false)
	if got != 250*time.Millisecond {
		t.Fatalf("expected configured timeout 250ms, got %v", got)
	}
}

func TestEffectiveAttemptTimeoutUsesRetransBudgetForUDP(t *testing.T) {
	client := &Client{
		Timeout: 250 * time.Millisecond,
		Retrans: 40 * time.Millisecond,
	}

	got := client.effectiveAttemptTimeout(context.Background(), false, false)
	if got != 40*time.Millisecond {
		t.Fatalf("expected UDP retrans budget 40ms, got %v", got)
	}
}

func TestEffectiveAttemptTimeoutDoesNotCapUDPForFallback(t *testing.T) {
	client := &Client{
		Timeout: 5 * time.Second,
		Retrans: 3 * time.Second,
	}
	client.SetFallback(true)

	got := client.effectiveAttemptTimeout(context.Background(), false, false)
	if got != 3*time.Second {
		t.Fatalf("expected UDP timeout budget to stay at retrans 3s, got %v", got)
	}
}

func TestEffectiveAttemptTimeoutUsesRetransBudgetForTCPFallback(t *testing.T) {
	client := &Client{
		Timeout: 250 * time.Millisecond,
		Retrans: 40 * time.Millisecond,
	}

	got := client.effectiveAttemptTimeout(context.Background(), true, true)
	if got != 40*time.Millisecond {
		t.Fatalf("expected fallback TCP retrans budget 40ms, got %v", got)
	}
}

func TestEffectiveAttemptTimeoutRespectsContextDeadline(t *testing.T) {
	client := &Client{
		Timeout: 300 * time.Millisecond,
		Retrans: 200 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	got := client.effectiveAttemptTimeout(ctx, true, false)
	if got <= 0 || got > 120*time.Millisecond {
		t.Fatalf("expected timeout within context deadline, got %v", got)
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

func TestEffectiveAttemptTimeoutFallsBackToRetransWhenTimeoutUnset(t *testing.T) {
	client := &Client{
		Timeout: 0,
		Retrans: 80 * time.Millisecond,
	}

	got := client.effectiveAttemptTimeout(context.Background(), false, false)
	if got != 80*time.Millisecond {
		t.Fatalf("expected retrans fallback 80ms, got %v", got)
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

// recordingTrace is a QueryTrace that captures every event for assertions. It
// guards its slices with a mutex because the transport layer may fire events
// from multiple goroutines; the tests below are single-threaded but the engine
// is not, and we want the test double to model the real contract.
type recordingTrace struct {
	mu        sync.Mutex
	attemptEv []querytrace.AttemptEvent
	decisions []querytrace.DecisionEvent
}

func (r *recordingTrace) AttemptDone(ev querytrace.AttemptEvent) {
	r.mu.Lock()
	r.attemptEv = append(r.attemptEv, ev)
	r.mu.Unlock()
}

func (r *recordingTrace) Decision(ev querytrace.DecisionEvent) {
	r.mu.Lock()
	r.decisions = append(r.decisions, ev)
	r.mu.Unlock()
}

func (r *recordingTrace) attempts() []querytrace.AttemptEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]querytrace.AttemptEvent, len(r.attemptEv))
	copy(out, r.attemptEv)
	return out
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

	rec := &recordingTrace{}
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

	events := rec.attempts()
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

// TestExchangeEmitsAttemptEventOnSuccess confirms a single successful query
// emits exactly one OutcomeOK event (no spurious retries are traced) and that
// the query name/type are reported correctly.
func TestExchangeEmitsAttemptEventOnSuccess(t *testing.T) {
	addr, shutdown := startUDPDNSServer(t, func(_ context.Context, w dns.ResponseWriter, req *dns.Msg) {
		writeSimpleAResponse(w, req)
	})
	defer shutdown()

	rec := &recordingTrace{}
	ctx := querytrace.WithContext(context.Background(), rec)

	client := &Client{}
	client.SetRetries(2)
	client.SetTimeout(time.Second)

	if _, err := client.Exchange(ctx, addr, BuildQuery("ok.example.", dns.TypeA)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := rec.attempts()
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
