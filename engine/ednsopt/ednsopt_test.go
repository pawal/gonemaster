package ednsopt

import (
	"testing"

	dns "codeberg.org/miekg/dns"
)

// The library decodes the OPT header itself, so pack and unpack to confirm our
// bit layout still agrees with it.
func TestFieldsSurviveWireRoundTrip(t *testing.T) {
	opt := &dns.OPT{Hdr: dns.Header{Name: "."}}
	SetUDPSize(opt, 1232)
	SetVersion(opt, 1)
	SetRcode(opt, 16)
	SetZ(opt, 3)
	SetSecurity(opt, true)
	SetCompactAnswers(opt, true)
	SetDelegation(opt, true)

	msg := new(dns.Msg)
	msg.Response = true
	msg.Extra = []dns.RR{opt}
	if err := msg.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	wire := new(dns.Msg)
	wire.Data = msg.Data
	if err := wire.Unpack(); err != nil {
		t.Fatalf("unpack: %v", err)
	}

	if wire.UDPSize != 1232 {
		t.Errorf("UDPSize = %d, want 1232", wire.UDPSize)
	}
	if wire.Version != 1 {
		t.Errorf("Version = %d, want 1", wire.Version)
	}
	if wire.Rcode != 16 {
		t.Errorf("Rcode = %d, want 16", wire.Rcode)
	}
	if wire.Z != 3 {
		t.Errorf("Z = %d, want 3", wire.Z)
	}
	if !wire.Security {
		t.Error("DO bit lost over the wire")
	}
	if !wire.CompactAnswers {
		t.Error("CO bit lost over the wire")
	}
	if !wire.Delegation {
		t.Error("DE bit lost over the wire")
	}
}

// Z is the low 13 bits; the DO, CO and DE bits sit above it and are separate.
func TestZExcludesFlagBits(t *testing.T) {
	opt := &dns.OPT{Hdr: dns.Header{Name: "."}}
	SetSecurity(opt, true)
	SetCompactAnswers(opt, true)
	SetDelegation(opt, true)
	if got := Z(opt); got != 0 {
		t.Errorf("Z = %#x with only flag bits set, want 0", got)
	}

	SetZ(opt, 0xFFFF)
	if got := Z(opt); got != 0x1FFF {
		t.Errorf("Z = %#x, want %#x", got, 0x1FFF)
	}
	if !Security(opt) || !CompactAnswers(opt) || !Delegation(opt) {
		t.Error("SetZ cleared a flag bit above the Z field")
	}
}

// Setters must not disturb neighbouring fields in the shared TTL word.
func TestSettersAreIndependent(t *testing.T) {
	opt := &dns.OPT{Hdr: dns.Header{Name: "."}}
	SetRcode(opt, 0xFF0)
	SetVersion(opt, 0xFF)
	SetZ(opt, 0x1FFF)
	SetSecurity(opt, true)

	SetVersion(opt, 0)
	if got := Rcode(opt); got != 0xFF0 {
		t.Errorf("Rcode = %#x after SetVersion, want %#x", got, 0xFF0)
	}
	if got := Z(opt); got != 0x1FFF {
		t.Errorf("Z = %#x after SetVersion, want %#x", got, 0x1FFF)
	}
	if !Security(opt) {
		t.Error("SetVersion cleared the DO bit")
	}

	SetRcode(opt, 0)
	if got := Version(opt); got != 0 {
		t.Errorf("Version = %d after SetRcode, want 0", got)
	}
	if got := Z(opt); got != 0x1FFF {
		t.Errorf("Z = %#x after SetRcode, want %#x", got, 0x1FFF)
	}

	SetSecurity(opt, false)
	if got := Z(opt); got != 0x1FFF {
		t.Errorf("Z = %#x after clearing DO, want %#x", got, 0x1FFF)
	}
}
