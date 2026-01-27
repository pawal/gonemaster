package recursor

import (
	"context"
	"net"
	"testing"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

type fakeQueryer struct {
	resp packet.Packet
	err  error
}

func (f fakeQueryer) QueryWithClass(_ context.Context, _ string, _ string, _ string) (packet.Packet, error) {
	return f.resp, f.err
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
		nsFrom: func(resp packet.Packet, _ *recurseState) ([]queryer, error) {
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
		nsFrom: func(resp packet.Packet, _ *recurseState) ([]queryer, error) {
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
	r := &Recursor{
		client:       &transport.Client{},
		recurseCache: map[string]map[string]map[string]*packet.Packet{},
	}

	err := r.AddFakeAddresses(".", map[string][]string{
		"root.test": {"192.0.2.53"},
	})
	if err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	rootNS, err := nameserver.New("root.test", "192.0.2.53", r.client)
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

	resp, _, err := r.recurse(context.Background(), "www.example.com", "A", "IN", state)
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
	resp.Msg.Answer = append(resp.Msg.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "other.example.net.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		A: net.IPv4(192, 0, 2, 55),
	})

	state := &recurseState{
		ns: []queryer{
			fakeQueryer{resp: resp},
		},
	}

	r := &Recursor{}
	out, _, err := r.recurse(context.Background(), "www.example.com", "A", "IN", state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	resp.Msg.Answer = append(resp.Msg.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "alias.example.net.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		A: net.IPv4(192, 0, 2, 200),
	})

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

func TestResolveCNAMELoopReturnsEmpty(t *testing.T) {
	resp := cnamePacket("www.example.com", "alias.example.com", "203.0.113.8")
	resp.Msg.Answer = append(resp.Msg.Answer, &dns.CNAME{
		Hdr: dns.RR_Header{
			Name:   "alias.example.com.",
			Rrtype: dns.TypeCNAME,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Target: "www.example.com.",
	})

	r := &Recursor{}
	out, _, err := r.resolveCNAME(context.Background(), dnsname.New("www.example.com"), "A", "IN", resp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Msg != nil {
		t.Fatalf("expected no response for CNAME loop")
	}
}

func referralPacket(zone string, answerFrom string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	zone = dns.Fqdn(zone)
	msg.Ns = []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   zone,
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    3600,
			},
			Ns: "ns1." + zone,
		},
	}
	return packet.Packet{Msg: msg, AnswerFrom: answerFrom}
}

func nxdomainPacket(answerFrom string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeNameError
	return packet.Packet{Msg: msg, AnswerFrom: answerFrom}
}

func answerPacket(qname string, answerFrom string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(qname),
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			A: net.IPv4(192, 0, 2, 1),
		},
	}
	return packet.Packet{Msg: msg, AnswerFrom: answerFrom}
}

func cnamePacket(qname string, target string, answerFrom string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.CNAME{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(qname),
				Rrtype: dns.TypeCNAME,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Target: dns.Fqdn(target),
		},
	}
	return packet.Packet{Msg: msg, AnswerFrom: answerFrom}
}
