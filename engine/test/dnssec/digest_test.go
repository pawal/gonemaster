package dnssec

import (
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// A DS digest gonemaster cannot recompute must not be read as a rollover
// signal. Digest type 5 is GOST R 34.11-2012, which the library cannot compute,
// so comparing it claimed a mismatch that was never computed.
func TestDNSSEC18CDNSKEYUncomputableDigest(t *testing.T) {
	ctx := tctest.Context(t)

	key := makeSEPKey("example", "AwEAAc==")
	keytag := key.KeyTag()
	parentDS := &dns.DS{
		Hdr:        dns.Header{Name: "example.", Class: dns.ClassINET, TTL: 3600},
		KeyTag:     keytag,
		Algorithm:  key.Algorithm,
		DigestType: 5,
		Digest:     "9a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f9",
	}
	cdnskey := key.ToCDNSKEY()
	cdnskeySig := rrsigRecord("example", dns.TypeCDNSKEY, keytag, 1, 2)
	dnskeyRRSIG := rrsigRecord("example", dns.TypeDNSKEY, keytag, 1, 2)

	parentNS := tctest.NS(t, ctx, "pns1.example", "192.0.2.100", func(q tctest.Query) packet.Packet {
		if q.Type == "DS" {
			return dsPacketFromDS("example", parentDS)
		}
		return packet.Packet{}
	})
	childNS := tctest.NS(t, ctx, "ns1.example", "192.0.2.101", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "CDS":
			return answerPacket(q.Name, dns.TypeCDS)
		case "CDNSKEY":
			return answerPacket(q.Name, dns.TypeCDNSKEY, cdnskey, cdnskeySig)
		case "DNSKEY":
			return answerPacket(q.Name, dns.TypeDNSKEY, key, dnskeyRRSIG)
		default:
			return packet.Packet{}
		}
	})

	setDNSSEC18Mocks(t, []nameserver.Nameserver{parentNS}, []nameserver.Nameserver{childNS})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC18(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC18: %v", err)
	}
	tctest.RequireNoTag(t, entries, "DS18_CDNSKEY_ROLLOVER_SIGNALED")
	tctest.RequireTags(t, entries, "DS18_CDNSKEY_MATCHES_DS")
}
