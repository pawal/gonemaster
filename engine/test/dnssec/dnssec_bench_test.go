package dnssec

import (
	"context"
	"fmt"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func BenchmarkDNSSEC18Parallel(b *testing.B) {
	origParentNS := parentApexNameservers
	origM4 := glueNameservers
	origM5 := apexNameservers
	defer func() {
		parentApexNameservers = origParentNS
		glueNameservers = origM4
		apexNameservers = origM5
	}()

	delay := 200 * time.Microsecond

	benchCase := func(b *testing.B, parallel int) {
		keyTemplate := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("template"), Class: dns.ClassINET, TTL: 60}}
		keyTemplate.Flags = dns.FlagZONE
		keyTemplate.Protocol = 3
		keyTemplate.Algorithm = 8
		keyTemplate.PublicKey = "AwEAAc=="
		keytag := keyTemplate.KeyTag()
		badKeytag := keytag + 1

		cache := nameserver.NewCacheStore()
		parentNS, err := nameserver.NewWithCache(cache, "pns1.example", "192.0.2.40", nil)
		if err != nil {
			b.Fatalf("new parent nameserver: %v", err)
		}
		parentNS.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			if qtype != "DS" {
				return packet.Packet{}, nil
			}
			time.Sleep(delay)
			return dsPacket(qname, keytag, 8, 1), nil
		})

		childHook := func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			switch qtype {
			case "CDS":
				time.Sleep(delay)
				cds := &dns.CDS{}
				cds.Hdr = dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}
				cds.KeyTag = keytag
				cds.Algorithm = 8
				cds.DigestType = 1
				cds.Digest = "DEADBEEF"
				cdsSig := rrsigRecord(qname, dns.TypeCDS, badKeytag, 1, 2)
				return answerPacket(qname, dns.TypeCDS, cds, cdsSig), nil
			case "CDNSKEY":
				time.Sleep(delay)
				cdnskey := &dns.CDNSKEY{}
				cdnskey.Hdr = dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}
				cdnskey.Flags = dns.FlagZONE
				cdnskey.Protocol = 3
				cdnskey.Algorithm = 8
				cdnskey.PublicKey = "AwEAAc=="
				cdnskeySig := rrsigRecord(qname, dns.TypeCDNSKEY, badKeytag, 1, 2)
				return answerPacket(qname, dns.TypeCDNSKEY, cdnskey, cdnskeySig), nil
			case "DNSKEY":
				time.Sleep(delay)
				key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 60}}
				key.Flags = dns.FlagZONE
				key.Protocol = 3
				key.Algorithm = 8
				key.PublicKey = "AwEAAc=="
				return dnskeyPacket(qname, key), nil
			default:
				return packet.Packet{}, nil
			}
		}

		child1, err := nameserver.NewWithCache(cache, "ns1.example", "192.0.2.41", nil)
		if err != nil {
			b.Fatalf("new child1 nameserver: %v", err)
		}
		child1.SetQueryHook(childHook)

		child2, err := nameserver.NewWithCache(cache, "ns2.example", "192.0.2.42", nil)
		if err != nil {
			b.Fatalf("new child2 nameserver: %v", err)
		}
		child2.SetQueryHook(childHook)

		parentApexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{parentNS}, nil
		}
		glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{child1, child2}, nil
		}
		apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		}

		p := dnstest.DefaultProfile(b)
		if err := p.Set("resolver.defaults.parallel", parallel); err != nil {
			b.Fatalf("set parallel: %v", err)
		}

		b.ReportAllocs()
		// Each iteration uses its own zone name so the run never reads a
		// previous iteration's cache entries.
		i := 0
		for b.Loop() {
			ctx := profile.WithContext(context.Background(), p)
			ctx = logger.WithContext(ctx, logger.New())
			z := zone.Zone{Name: dnsname.New(fmt.Sprintf("bench-%d.example", i))}
			i++

			if _, err := DNSSEC18(ctx, &z); err != nil {
				b.Fatalf("dnssec18: %v", err)
			}
		}
	}

	b.Run("parallel_1", func(b *testing.B) {
		benchCase(b, 1)
	})
	b.Run("parallel_2", func(b *testing.B) {
		benchCase(b, 2)
	})
}
