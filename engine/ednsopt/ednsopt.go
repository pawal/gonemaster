// Package ednsopt reads and writes the OPT pseudo-record header fields.
package ednsopt

import dns "codeberg.org/miekg/dns"

// OPT header bit layout, RFC 6891 section 6.1.3. The DNS library keeps these
// unexported, so gonemaster encodes them here. FlagCO and FlagDE are exported
// because a caller's 16-bit Z value carries them above the 13-bit Z field.
const (
	flagDO = 1 << 15 // DNSSEC OK
	FlagCO = 1 << 14 // Compact Answers OK
	FlagDE = 1 << 13 // DELEG OK

	maskRcode   = 0xFF000000 // extended RCODE, the upper 8 bits of a 12-bit rcode
	maskVersion = 0x00FF0000 // EDNS version
	maskZ       = 0x1FFF     // Z, the low 13 bits
)

// UDPSize returns the advertised UDP payload size.
func UDPSize(rr *dns.OPT) uint16 { return rr.Hdr.Class }

// SetUDPSize sets the advertised UDP payload size.
func SetUDPSize(rr *dns.OPT, size uint16) { rr.Hdr.Class = size }

// Version returns the EDNS version.
func Version(rr *dns.OPT) uint8 { return uint8(rr.Hdr.TTL & maskVersion >> 16) }

// SetVersion sets the EDNS version.
func SetVersion(rr *dns.OPT, v uint8) { rr.Hdr.TTL = rr.Hdr.TTL&^maskVersion | uint32(v)<<16 }

// Rcode returns the extended rcode, already shifted into the 12-bit rcode space.
func Rcode(rr *dns.OPT) uint16 { return uint16(rr.Hdr.TTL&maskRcode>>24) << 4 }

// SetRcode stores the upper 8 bits of a 12-bit rcode.
func SetRcode(rr *dns.OPT, v uint16) { rr.Hdr.TTL = rr.Hdr.TTL&^maskRcode | uint32(v>>4)<<24 }

// Z returns the Z bits, the low 13 bits of the TTL.
func Z(rr *dns.OPT) uint16 { return uint16(rr.Hdr.TTL & maskZ) }

// SetZ sets the Z bits; only the low 13 bits of z are used.
func SetZ(rr *dns.OPT, z uint16) { rr.Hdr.TTL = rr.Hdr.TTL&^maskZ | uint32(z&maskZ) }

// Security reports the DO bit.
func Security(rr *dns.OPT) bool { return rr.Hdr.TTL&flagDO == flagDO }

// SetSecurity sets the DO bit.
func SetSecurity(rr *dns.OPT, do bool) { setFlag(rr, flagDO, do) }

// CompactAnswers reports the CO bit.
func CompactAnswers(rr *dns.OPT) bool { return rr.Hdr.TTL&FlagCO == FlagCO }

// SetCompactAnswers sets the CO bit.
func SetCompactAnswers(rr *dns.OPT, co bool) { setFlag(rr, FlagCO, co) }

// Delegation reports the DE bit.
func Delegation(rr *dns.OPT) bool { return rr.Hdr.TTL&FlagDE == FlagDE }

// SetDelegation sets the DE bit.
func SetDelegation(rr *dns.OPT, de bool) { setFlag(rr, FlagDE, de) }

func setFlag(rr *dns.OPT, bit uint32, on bool) {
	if on {
		rr.Hdr.TTL |= bit
		return
	}
	rr.Hdr.TTL &^= bit
}
