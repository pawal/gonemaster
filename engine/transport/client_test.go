package transport

import (
	"context"
	"net"
	"testing"

	"github.com/miekg/dns"

	"github.com/pawal/gonemaster/engine/constants"
	"github.com/pawal/gonemaster/engine/profile"
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
	defer profile.ResetEffective()
	profile.Effective().Resolver.Defaults.Recurse = true
	profile.Effective().Resolver.Defaults.UseVC = true

	client := &Client{}
	client.ApplyProfileDefaults(nil)

	if !client.RecursionDesired {
		t.Fatalf("expected recursion enabled from defaults")
	}
	if !client.UseTCP {
		t.Fatalf("expected TCP enabled from defaults")
	}
}

func TestApplyProfileDefaultsDoesNotOverrideExplicit(t *testing.T) {
	defer profile.ResetEffective()
	profile.Effective().Resolver.Defaults.Recurse = false

	client := &Client{}
	client.SetRecursionDesired(true)
	client.ApplyProfileDefaults(nil)

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
