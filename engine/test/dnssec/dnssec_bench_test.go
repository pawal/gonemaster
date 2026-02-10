package dnssec

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func BenchmarkDNSSEC18Parallel(b *testing.B) {
	origParentNS := parentNameservers
	origM4 := method4
	origM5 := method5
	defer func() {
		parentNameservers = origParentNS
		method4 = origM4
		method5 = origM5
	}()

	delay := 200 * time.Microsecond

	benchCase := func(b *testing.B, parallel int) {
		keyTemplate := &dns.DNSKEY{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn("template"),
				Rrtype: dns.TypeDNSKEY,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Flags:     dns.ZONE,
			Protocol:  3,
			Algorithm: 8,
			PublicKey: "AwEAAc==",
		}
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
				cds := &dns.CDS{
					DS: dns.DS{
						Hdr: dns.RR_Header{
							Name:   dns.Fqdn(qname),
							Rrtype: dns.TypeCDS,
							Class:  dns.ClassINET,
							Ttl:    60,
						},
						KeyTag:     keytag,
						Algorithm:  8,
						DigestType: 1,
						Digest:     "DEADBEEF",
					},
				}
				cdsSig := rrsigRecord(qname, dns.TypeCDS, badKeytag, 1, 2)
				return answerPacket(qname, dns.TypeCDS, cds, cdsSig), nil
			case "CDNSKEY":
				time.Sleep(delay)
				cdnskey := &dns.CDNSKEY{
					DNSKEY: dns.DNSKEY{
						Hdr: dns.RR_Header{
							Name:   dns.Fqdn(qname),
							Rrtype: dns.TypeCDNSKEY,
							Class:  dns.ClassINET,
							Ttl:    60,
						},
						Flags:     dns.ZONE,
						Protocol:  3,
						Algorithm: 8,
						PublicKey: "AwEAAc==",
					},
				}
				cdnskeySig := rrsigRecord(qname, dns.TypeCDNSKEY, badKeytag, 1, 2)
				return answerPacket(qname, dns.TypeCDNSKEY, cdnskey, cdnskeySig), nil
			case "DNSKEY":
				time.Sleep(delay)
				key := &dns.DNSKEY{
					Hdr: dns.RR_Header{
						Name:   dns.Fqdn(qname),
						Rrtype: dns.TypeDNSKEY,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					Flags:     dns.ZONE,
					Protocol:  3,
					Algorithm: 8,
					PublicKey: "AwEAAc==",
				}
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

		parentNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{parentNS}, nil
		}
		method4 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{child1, child2}, nil
		}
		method5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return nil, nil
		}

		p, err := profile.Default()
		if err != nil {
			b.Fatalf("profile default: %v", err)
		}
		if err := p.Set("resolver.defaults.parallel", parallel); err != nil {
			b.Fatalf("set parallel: %v", err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx := profile.WithContext(context.Background(), p)
			ctx = logger.WithContext(ctx, logger.New())
			z := zone.Zone{Name: dnsname.New(fmt.Sprintf("bench-%d.example", i))}

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
