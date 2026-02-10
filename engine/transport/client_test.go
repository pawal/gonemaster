package transport

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func TestBuildQueryWithClass(t *testing.T) {
	msg := BuildQueryWithClass("example.com", dns.TypeA, dns.ClassCHAOS)
	if len(msg.Question) != 1 {
		t.Fatalf("expected 1 question")
	}
	if msg.Question[0].Qclass != dns.ClassCHAOS {
		t.Fatalf("unexpected qclass: %d", msg.Question[0].Qclass)
	}
	if msg.RecursionDesired {
		t.Fatalf("expected recursion disabled by default")
	}
}

func TestApplyProfileDefaultsRecurse(t *testing.T) {
	prof, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}
	prof.Resolver.Defaults.Recurse = true
	prof.Resolver.Defaults.UseVC = true

	client := &Client{}
	client.ApplyProfileDefaults(prof)

	if !client.RecursionDesired {
		t.Fatalf("expected recursion enabled from defaults")
	}
	if !client.UseTCP {
		t.Fatalf("expected TCP enabled from defaults")
	}
}

func TestApplyProfileDefaultsDoesNotOverrideExplicit(t *testing.T) {
	prof, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}
	prof.Resolver.Defaults.Recurse = false

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
	z := uint16(3)
	rcode := uint8(16)

	client := &Client{
		RecursionDesired: true,
		EDNSSize:         1232,
		EDNSDetails: &EDNSDetails{
			Do:      &do,
			Version: &version,
			Z:       &z,
			Rcode:   &rcode,
			Data: []dns.EDNS0{
				&dns.EDNS0_NSID{Code: dns.EDNS0NSID},
			},
		},
	}

	msg := BuildQuery("example.com", dns.TypeA)
	prepared := client.prepareMessage(msg)

	if !prepared.RecursionDesired {
		t.Fatalf("expected recursion desired to be true")
	}

	opt := prepared.IsEdns0()
	if opt == nil {
		t.Fatalf("expected edns option")
	}
	if !opt.Do() || opt.Version() != 1 || opt.Z() != 3 || opt.ExtendedRcode() != 16 {
		t.Fatalf("unexpected edns values: do=%v version=%d z=%d rcode=%d", opt.Do(), opt.Version(), opt.Z(), opt.ExtendedRcode())
	}
	if len(opt.Option) != 1 {
		t.Fatalf("expected edns option data")
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

	server := &dns.Server{
		PacketConn: packetConn,
		Handler:    handler,
	}

	done := make(chan struct{})
	go func() {
		_ = server.ActivateAndServe()
		close(done)
	}()

	shutdown := func() {
		_ = server.Shutdown()
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

	server := &dns.Server{
		Net:      "tcp",
		Listener: counting,
		Handler:  handler,
	}
	if configure != nil {
		configure(server)
	}

	done := make(chan struct{})
	go func() {
		_ = server.ActivateAndServe()
		close(done)
	}()

	shutdown := func() {
		_ = server.Shutdown()
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Logf("tcp dns server shutdown timed out")
		}
	}

	return listener.Addr().String(), counting, shutdown
}

func writeSimpleAResponse(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetReply(req)
	if len(req.Question) > 0 {
		resp.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   req.Question[0].Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				A: net.IPv4(192, 0, 2, 10),
			},
		}
	}
	_ = w.WriteMsg(resp)
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

func TestEffectiveAttemptTimeoutCapsUDPFallbackWait(t *testing.T) {
	client := &Client{
		Timeout: 5 * time.Second,
		Retrans: 3 * time.Second,
	}
	client.SetFallback(true)

	got := client.effectiveAttemptTimeout(context.Background(), false, false)
	if got != udpFallbackWaitCap {
		t.Fatalf("expected UDP fallback cap %v, got %v", udpFallbackWaitCap, got)
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
	serverAddr, _, shutdown := startTCPDNSServer(t, func(w dns.ResponseWriter, req *dns.Msg) {
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

func TestExchangeFallbackTCPUsesRetransBudget(t *testing.T) {
	serverAddr, listener, shutdown := startTCPDNSServer(t, func(w dns.ResponseWriter, req *dns.Msg) {
		time.Sleep(120 * time.Millisecond)
		writeSimpleAResponse(w, req)
	}, nil)
	defer shutdown()

	client := &Client{}
	client.SetUseTCP(false)
	client.SetFallback(true)
	client.SetRetries(0)
	client.SetTimeout(300 * time.Millisecond)
	client.SetRetrans(40 * time.Millisecond)

	start := time.Now()
	_, err := client.Exchange(context.Background(), serverAddr, BuildQuery("tcp-fallback-retrans.example", dns.TypeA))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected fallback TCP timeout")
	}
	if got := listener.accepts.Load(); got < 1 {
		t.Fatalf("expected TCP fallback attempt, got %d TCP accepts", got)
	}
	if elapsed < 20*time.Millisecond || elapsed > 250*time.Millisecond {
		t.Fatalf("expected fallback timeout near retrans budget, took %v", elapsed)
	}
}

func TestExchangeFallbackTCPDoesNotWaitFullRetransOnUDPFailure(t *testing.T) {
	serverAddr, listener, shutdownTCP := startTCPDNSServer(t, func(w dns.ResponseWriter, req *dns.Msg) {
		writeSimpleAResponse(w, req)
	}, nil)
	defer shutdownTCP()

	packetConn, err := net.ListenPacket("udp", serverAddr)
	if err != nil {
		t.Fatalf("listen udp on tcp addr: %v", err)
	}
	udpServer := &dns.Server{
		PacketConn: packetConn,
		Handler: dns.HandlerFunc(func(_ dns.ResponseWriter, _ *dns.Msg) {
			// Intentionally blackhole UDP queries so fallback path is exercised.
		}),
	}
	udpDone := make(chan struct{})
	go func() {
		_ = udpServer.ActivateAndServe()
		close(udpDone)
	}()
	defer func() {
		_ = udpServer.Shutdown()
		_ = packetConn.Close()
		select {
		case <-udpDone:
		case <-time.After(2 * time.Second):
			t.Logf("udp dns server shutdown timed out")
		}
	}()

	client := &Client{}
	client.SetUseTCP(false)
	client.SetFallback(true)
	client.SetRetries(0)
	client.SetTimeout(5 * time.Second)
	client.SetRetrans(3 * time.Second)

	start := time.Now()
	_, err = client.Exchange(context.Background(), serverAddr, BuildQuery("tcp-fallback-udp-cap.example", dns.TypeA))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected fallback TCP success, got %v", err)
	}
	if got := listener.accepts.Load(); got < 1 {
		t.Fatalf("expected TCP fallback attempt, got %d TCP accepts", got)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected fallback path to avoid full 3s UDP wait, took %v", elapsed)
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
	serverAddr, shutdown := startUDPDNSServer(t, func(_ dns.ResponseWriter, _ *dns.Msg) {
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
