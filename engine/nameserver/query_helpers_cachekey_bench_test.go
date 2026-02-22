package nameserver

import (
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/transport"
)

func BenchmarkBuildCacheKey(b *testing.B) {
	b.Run("basic", func(b *testing.B) {
		b.ReportAllocs()
		var key string
		for i := 0; i < b.N; i++ {
			k, _, _, err := buildCacheKey("benchmark.example", "A", "IN", nil)
			if err != nil {
				b.Fatalf("buildCacheKey: %v", err)
			}
			key = k
		}
		if key == "" {
			b.Fatalf("empty key")
		}
	})

	b.Run("edns_details", func(b *testing.B) {
		b.ReportAllocs()
		dnssec := true
		usevc := true
		recurse := true
		size := uint16(1232)
		version := uint8(1)
		z := uint16(2)
		rcode := uint8(3)
		data := []dns.EDNS0{&dns.NSID{Nsid: "beef"}}
		opts := &QueryOptions{
			DNSSEC:  &dnssec,
			UseVC:   &usevc,
			Recurse: &recurse,
			EDNSDetails: &transport.EDNSDetails{
				Size:    &size,
				Version: &version,
				Z:       &z,
				Rcode:   &rcode,
				Data:    data,
			},
		}

		var key string
		for i := 0; i < b.N; i++ {
			k, _, _, err := buildCacheKey("benchmark.example", "AAAA", "IN", opts)
			if err != nil {
				b.Fatalf("buildCacheKey: %v", err)
			}
			key = k
		}
		if key == "" {
			b.Fatalf("empty key")
		}
	})
}
