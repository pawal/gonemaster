package packet

import (
	"net/netip"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

func TestUniquePush(t *testing.T) {
	msg := new(dns.Msg)
	pkt := New(msg)

	rr, err := dns.New("example. 60 IN A 192.0.2.1")
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
	a := &dns.A{Hdr: dns.Header{
		Name:  "Example.COM.",
		Class: dns.ClassINET,
		TTL:   60,
	}}
	a.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 1})
	msg.Answer = []dns.RR{a}

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
		&dns.SOA{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}},
	}
	nodata := New(nodataMsg)
	if nodata.Type() != "nodata" || !nodata.NoSuchRecord() {
		t.Fatalf("unexpected nodata classification")
	}

	referralMsg := new(dns.Msg)
	referralMsg.Rcode = dns.RcodeSuccess
	referralMsg.Ns = []dns.RR{
		&dns.NS{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}},
	}
	referral := New(referralMsg)
	if referral.Type() != "referral" || !referral.IsRedirect() {
		t.Fatalf("unexpected referral classification")
	}

	answerMsg := new(dns.Msg)
	answerMsg.Rcode = dns.RcodeSuccess
	answerMsg.Answer = []dns.RR{
		&dns.A{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}},
	}
	answer := New(answerMsg)
	if answer.Type() != "answer" {
		t.Fatalf("unexpected answer classification")
	}
}

func TestPacketClassificationEmitsSystemLogs(t *testing.T) {
	log := logger.New()

	nxdomainMsg := new(dns.Msg)
	dnsutil.SetQuestion(nxdomainMsg, "www.example.", dns.TypeA)
	nxdomainMsg.Rcode = dns.RcodeNameError
	nxdomain := Packet{Msg: nxdomainMsg, Log: log}
	if !nxdomain.NoSuchName() {
		t.Fatalf("expected nxdomain packet")
	}

	nodataMsg := new(dns.Msg)
	dnsutil.SetQuestion(nodataMsg, "www.example.", dns.TypeAAAA)
	nodataMsg.Rcode = dns.RcodeSuccess
	nodataMsg.Ns = []dns.RR{
		&dns.SOA{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}},
	}
	nodata := Packet{Msg: nodataMsg, Log: log}
	if !nodata.NoSuchRecord() {
		t.Fatalf("expected nodata packet")
	}

	referralMsg := new(dns.Msg)
	dnsutil.SetQuestion(referralMsg, "www.example.", dns.TypeA)
	referralMsg.Rcode = dns.RcodeSuccess
	referralMsg.Ns = []dns.RR{
		&dns.NS{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}},
	}
	referral := Packet{Msg: referralMsg, Log: log}
	if !referral.IsRedirect() {
		t.Fatalf("expected referral packet")
	}

	tags := map[string]bool{}
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		tags[entry.Tag] = true
	}
	if !tags["NO_SUCH_NAME"] {
		t.Fatalf("expected NO_SUCH_NAME log")
	}
	if !tags["NO_SUCH_RECORD"] {
		t.Fatalf("expected NO_SUCH_RECORD log")
	}
	if !tags["IS_REDIRECT"] {
		t.Fatalf("expected IS_REDIRECT log")
	}
}

func TestEdnsHelpers(t *testing.T) {
	msg := new(dns.Msg)
	msg.UDPSize = 1232
	msg.Security = true
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

func TestEdnsHelpersFromExplicitOPTRecord(t *testing.T) {
	msg := new(dns.Msg)
	opt := &dns.OPT{Hdr: dns.Header{Name: "."}}
	opt.SetUDPSize(1232)
	opt.SetVersion(1)
	opt.SetSecurity(true)
	opt.SetRcode(16)
	opt.SetZ(3)
	opt.Options = []dns.EDNS0{&dns.NSID{Nsid: "beef"}}
	msg.Extra = []dns.RR{opt}

	pkt := New(msg)
	if !pkt.HasEdns() {
		t.Fatalf("expected EDNS present from explicit OPT record")
	}
	if pkt.EdnsSize() != 1232 {
		t.Fatalf("unexpected EDNS size: %d", pkt.EdnsSize())
	}
	if pkt.EdnsVersion() != 1 {
		t.Fatalf("unexpected EDNS version: %d", pkt.EdnsVersion())
	}
	if pkt.EdnsRcode() != 1 {
		t.Fatalf("unexpected EDNS extended rcode: %d", pkt.EdnsRcode())
	}
	if pkt.EdnsZ() != 3 {
		t.Fatalf("unexpected EDNS Z: %d", pkt.EdnsZ())
	}
	if !pkt.DO() {
		t.Fatalf("expected DO bit set from OPT")
	}
	if len(pkt.EdnsData()) != 1 {
		t.Fatalf("expected 1 EDNS option from OPT, got %d", len(pkt.EdnsData()))
	}
	if _, ok := pkt.EdnsData()[0].(*dns.NSID); !ok {
		t.Fatalf("expected NSID option from OPT")
	}
}

func TestPacketBasicHelpers(t *testing.T) {
	msg := new(dns.Msg)
	msg.ID = 1234
	msg.Opcode = dns.OpcodeUpdate
	msg.Rcode = dns.RcodeRefused
	msg.Authoritative = true
	msg.RecursionAvailable = true
	msg.Truncated = true

	a := &dns.A{Hdr: dns.Header{
		Name:  "example.",
		Class: dns.ClassINET,
	}}
	a.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 10})
	msg.Answer = []dns.RR{a}
	ns := &dns.NS{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}}
	ns.Ns = "ns1.example."
	msg.Ns = []dns.RR{ns}

	msg.UDPSize = 1232
	msg.Security = false
	msg.Version = 2
	msg.Pseudo = []dns.RR{&dns.NSID{}}

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

	if pkt.EdnsRcode() != 0 || pkt.EdnsVersion() != 2 {
		t.Fatalf("unexpected edns fields: rcode=%d version=%d", pkt.EdnsRcode(), pkt.EdnsVersion())
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
	rr, err := dns.New("example. 60 IN A 192.0.2.1")
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
