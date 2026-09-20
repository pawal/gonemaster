package ednsopt

import (
	"testing"

	dns "codeberg.org/miekg/dns"
)

func packUnpack(t *testing.T, msg *dns.Msg) *dns.Msg {
	t.Helper()
	if err := msg.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	wire := new(dns.Msg)
	wire.Data = msg.Data
	if err := wire.Unpack(); err != nil {
		t.Fatalf("unpack: %v", err)
	}
	return wire
}

// Every message-level EDNS field is an OPT trigger in Pack, so a residual value
// emits a second OPT alongside the explicit one.
func TestApplyExplicitClearsEveryTrigger(t *testing.T) {
	msg := new(dns.Msg)
	msg.UDPSize = 1232
	msg.Security = true
	msg.CompactAnswers = true
	msg.Delegation = true
	msg.Version = 1
	msg.Z = 3
	msg.Rcode = 16

	ApplyExplicit(msg, nil)

	for _, tc := range []struct {
		name string
		got  bool
	}{
		{"UDPSize", msg.UDPSize != 0},
		{"Security", msg.Security},
		{"CompactAnswers", msg.CompactAnswers},
		{"Delegation", msg.Delegation},
		{"Version", msg.Version != 0},
		{"Z", msg.Z != 0},
		{"Rcode", msg.Rcode > 0xF},
	} {
		if tc.got {
			t.Errorf("Msg.%s still set; Pack will emit a second OPT", tc.name)
		}
	}

	wire := packUnpack(t, msg)
	if wire.UDPSize != 1232 || !wire.Security || wire.Version != 1 || wire.Z != 3 {
		t.Errorf("settings lost: udp=%d do=%v ver=%d z=%d", wire.UDPSize, wire.Security, wire.Version, wire.Z)
	}
	if wire.Rcode != 16 {
		t.Errorf("Rcode = %d over the wire, want 16", wire.Rcode)
	}
}

// CO and DE sit above the 13-bit Z field, so they survive only as flag bits.
func TestApplyExplicitFoldsFlagBitsOutOfZ(t *testing.T) {
	z := uint16(FlagCO | FlagDE | 5)
	msg := new(dns.Msg)
	msg.UDPSize = 512

	ApplyExplicit(msg, &z)

	wire := packUnpack(t, msg)
	if !wire.CompactAnswers {
		t.Error("CO bit dropped; SetZ masks it away")
	}
	if !wire.Delegation {
		t.Error("DE bit dropped; SetZ masks it away")
	}
	if wire.Z != 5 {
		t.Errorf("Z = %d over the wire, want 5", wire.Z)
	}
}

// An OPT already in the additional section is replaced, never duplicated.
func TestApplyExplicitReplacesExistingOPT(t *testing.T) {
	stale := &dns.OPT{Hdr: dns.Header{Name: "."}}
	SetUDPSize(stale, 4096)

	msg := new(dns.Msg)
	msg.UDPSize = 512
	msg.Extra = []dns.RR{stale}

	ApplyExplicit(msg, nil)

	if len(msg.Extra) != 1 {
		t.Fatalf("additional section holds %d records, want 1", len(msg.Extra))
	}
	if got := UDPSize(msg.Extra[0].(*dns.OPT)); got != 512 {
		t.Errorf("UDP size = %d, want 512; the stale OPT survived", got)
	}
	packUnpack(t, msg)
}

// A TSIG or SIG0 must stay last, which only the default packing path guarantees.
func TestApplyExplicitSkipsNonEDNSPseudo(t *testing.T) {
	msg := new(dns.Msg)
	msg.UDPSize = 512
	msg.Pseudo = []dns.RR{&dns.TSIG{Hdr: dns.Header{Name: "key.example.", Class: dns.ClassANY}}}

	ApplyExplicit(msg, nil)

	if len(msg.Extra) != 0 {
		t.Errorf("additional section gained %d records, want none", len(msg.Extra))
	}
	if msg.UDPSize != 512 {
		t.Errorf("UDPSize = %d, want 512 left untouched", msg.UDPSize)
	}
	if len(msg.Pseudo) != 1 {
		t.Errorf("pseudo section holds %d records, want 1", len(msg.Pseudo))
	}
}

// EDNS options move into the explicit OPT and reach the wire.
func TestApplyExplicitCarriesOptions(t *testing.T) {
	msg := new(dns.Msg)
	msg.UDPSize = 512
	msg.Pseudo = []dns.RR{&dns.NSID{Nsid: "beef"}}

	ApplyExplicit(msg, nil)

	wire := packUnpack(t, msg)
	if len(wire.Pseudo) != 1 {
		t.Fatalf("pseudo section holds %d records over the wire, want 1", len(wire.Pseudo))
	}
	nsid, ok := wire.Pseudo[0].(*dns.NSID)
	if !ok || nsid.Nsid != "beef" {
		t.Errorf("NSID lost over the wire: %#v", wire.Pseudo[0])
	}
}
