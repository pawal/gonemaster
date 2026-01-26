package packet

import (
	"net"
	"testing"

	"github.com/miekg/dns"

	"github.com/pawal/gonemaster/engine/dnsname"
)

func TestUniquePush(t *testing.T) {
	msg := new(dns.Msg)
	pkt := New(msg)

	rr, err := dns.NewRR("example. 60 IN A 192.0.2.1")
	if err != nil {
		t.Fatalf("new rr: %v", err)
	}

	if !pkt.UniquePush("answer", rr) {
		t.Fatalf("expected first push to succeed")
	}
	if pkt.UniquePush("answer", rr) {
		t.Fatalf("expected duplicate push to fail")
	}
}

func TestGetRecordsForNameIgnoresTrailingDot(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "Example.COM.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			A: []byte{192, 0, 2, 1},
		},
	}

	pkt := New(msg)
	recs := pkt.GetRecordsForName("A", dnsname.New("example.com"), "answer")
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
}

func TestPacketTypeClassification(t *testing.T) {
	nxdomainMsg := new(dns.Msg)
	nxdomainMsg.Rcode = dns.RcodeNameError
	nxdomain := New(nxdomainMsg)
	if nxdomain.Type() != "nxdomain" || !nxdomain.NoSuchName() {
		t.Fatalf("unexpected nxdomain classification")
	}

	nodataMsg := new(dns.Msg)
	nodataMsg.Rcode = dns.RcodeSuccess
	nodataMsg.Ns = []dns.RR{
		&dns.SOA{Hdr: dns.RR_Header{Name: "example.", Rrtype: dns.TypeSOA, Class: dns.ClassINET}},
	}
	nodata := New(nodataMsg)
	if nodata.Type() != "nodata" || !nodata.NoSuchRecord() {
		t.Fatalf("unexpected nodata classification")
	}

	referralMsg := new(dns.Msg)
	referralMsg.Rcode = dns.RcodeSuccess
	referralMsg.Ns = []dns.RR{
		&dns.NS{Hdr: dns.RR_Header{Name: "example.", Rrtype: dns.TypeNS, Class: dns.ClassINET}},
	}
	referral := New(referralMsg)
	if referral.Type() != "referral" || !referral.IsRedirect() {
		t.Fatalf("unexpected referral classification")
	}

	answerMsg := new(dns.Msg)
	answerMsg.Rcode = dns.RcodeSuccess
	answerMsg.Answer = []dns.RR{
		&dns.A{Hdr: dns.RR_Header{Name: "example.", Rrtype: dns.TypeA, Class: dns.ClassINET}},
	}
	answer := New(answerMsg)
	if answer.Type() != "answer" {
		t.Fatalf("unexpected answer classification")
	}
}

func TestEdnsHelpers(t *testing.T) {
	msg := new(dns.Msg)
	msg.SetEdns0(1232, true)
	pkt := New(msg)

	if !pkt.HasEdns() {
		t.Fatalf("expected EDNS present")
	}
	if pkt.EdnsSize() != 1232 {
		t.Fatalf("unexpected EDNS size: %d", pkt.EdnsSize())
	}
	if !pkt.DO() {
		t.Fatalf("expected DO bit set")
	}
}

func TestPacketBasicHelpers(t *testing.T) {
	msg := new(dns.Msg)
	msg.Id = 1234
	msg.Opcode = dns.OpcodeUpdate
	msg.Rcode = dns.RcodeRefused
	msg.Authoritative = true
	msg.RecursionAvailable = true
	msg.Truncated = true

	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "example.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
			},
			A: net.IPv4(192, 0, 2, 10),
		},
	}
	msg.Ns = []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   "example.",
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
			},
			Ns: "ns1.example.",
		},
	}

	msg.SetEdns0(1232, false)
	opt := msg.IsEdns0()
	opt.SetVersion(2)
	opt.SetZ(3)
	opt.Option = append(opt.Option, &dns.EDNS0_NSID{Code: dns.EDNS0NSID})

	pkt := New(msg)
	if pkt.ID() != 1234 {
		t.Fatalf("unexpected id %d", pkt.ID())
	}
	if pkt.Opcode() != "UPDATE" {
		t.Fatalf("unexpected opcode %q", pkt.Opcode())
	}
	if pkt.Rcode() != "REFUSED" {
		t.Fatalf("unexpected rcode %q", pkt.Rcode())
	}
	if !pkt.AA() || !pkt.RA() || !pkt.TC() {
		t.Fatalf("expected AA, RA, and TC flags set")
	}
	if len(pkt.Data()) == 0 {
		t.Fatalf("expected packet data")
	}
	if pkt.AnswerFromString() != "<unknown>" {
		t.Fatalf("unexpected answer from string %q", pkt.AnswerFromString())
	}
	pkt.AnswerFrom = "192.0.2.53"
	if pkt.AnswerFromString() != "192.0.2.53" {
		t.Fatalf("unexpected answer from string %q", pkt.AnswerFromString())
	}

	if pkt.EdnsRcode() != 0 || pkt.EdnsVersion() != 2 || pkt.EdnsZ() != 3 {
		t.Fatalf("unexpected edns fields: rcode=%d version=%d z=%d", pkt.EdnsRcode(), pkt.EdnsVersion(), pkt.EdnsZ())
	}
	if len(pkt.EdnsData()) != 1 {
		t.Fatalf("expected edns data")
	}

	if len(pkt.GetRecords("A", "answer")) != 1 {
		t.Fatalf("expected answer A record")
	}
	if !pkt.HasRRsOfTypeForName("A", dnsname.New("example"), "answer") {
		t.Fatalf("expected A record for example")
	}
}

func TestUniquePushInvalidInputs(t *testing.T) {
	rr, err := dns.NewRR("example. 60 IN A 192.0.2.1")
	if err != nil {
		t.Fatalf("new rr: %v", err)
	}

	var nilPacket *Packet
	if nilPacket.UniquePush("answer", rr) {
		t.Fatalf("expected nil packet push to fail")
	}

	pkt := New(new(dns.Msg))
	if pkt.UniquePush("unknown", rr) {
		t.Fatalf("expected unknown section to fail")
	}
	if pkt.UniquePush("answer", nil) {
		t.Fatalf("expected nil rr to fail")
	}
}
