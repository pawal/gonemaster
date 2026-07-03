package cachefile

import (
	"fmt"
	"net/netip"
	"path/filepath"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/nameserver"
)

// benchMsg builds a realistic-sized DNS response (question + a handful of A
// records) packed to wire format, used as the payload for benchmark entries.
func benchMsg(b *testing.B) []byte {
	b.Helper()
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, "example.com.", dns.TypeA)
	msg.Response = true
	for i := 0; i < 8; i++ {
		rr := &dns.A{Hdr: dns.Header{Name: "example.com.", Class: dns.ClassINET, TTL: 3600}}
		rr.Addr = netip.AddrFrom4([4]byte{192, 0, 2, byte(i)})
		msg.Answer = append(msg.Answer, rr)
	}
	if err := msg.Pack(); err != nil {
		b.Fatalf("pack: %v", err)
	}
	return msg.Data
}

// benchNameserverCache seeds a cache store with n unique nameserver entries
// spread over 256 addresses.
func benchNameserverCache(b *testing.B, n int) *nameserver.CacheStore {
	b.Helper()
	msg := benchMsg(b)
	cache := nameserver.NewCacheStore()
	entries := make([]nameserver.Entry, 0, n)
	for i := 0; i < n; i++ {
		entries = append(entries, nameserver.Entry{
			Address:    fmt.Sprintf("192.0.2.%d", i%256),
			Key:        fmt.Sprintf("example%d.com./A/IN", i),
			Message:    msg,
			AnswerFrom: "192.0.2.53:53",
		})
	}
	if err := cache.ImportEntries(entries); err != nil {
		b.Fatalf("seed: %v", err)
	}
	return cache
}

func benchmarkSave(b *testing.B, n int, compress bool) {
	cache := benchNameserverCache(b, n)
	path := filepath.Join(b.TempDir(), "cache.json")
	var opts []SaveOption
	if compress {
		opts = append(opts, WithCompression())
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Save(path, cache, nil, nil, opts...); err != nil {
			b.Fatalf("save: %v", err)
		}
	}
}

func benchmarkRestore(b *testing.B, n int, compress bool) {
	cache := benchNameserverCache(b, n)
	name := "cache.json"
	if compress {
		name = "cache.json.gz"
	}
	path := filepath.Join(b.TempDir(), name)
	if err := Save(path, cache, nil, nil); err != nil {
		b.Fatalf("save: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Restore(path, nameserver.NewCacheStore(), nil, nil); err != nil {
			b.Fatalf("restore: %v", err)
		}
	}
}

func BenchmarkCachefileSave(b *testing.B) {
	for _, n := range []int{1000, 10000, 100000} {
		b.Run(fmt.Sprintf("n=%d/plain", n), func(b *testing.B) { benchmarkSave(b, n, false) })
		b.Run(fmt.Sprintf("n=%d/gzip", n), func(b *testing.B) { benchmarkSave(b, n, true) })
	}
}

func BenchmarkCachefileRestore(b *testing.B) {
	for _, n := range []int{1000, 10000, 100000} {
		b.Run(fmt.Sprintf("n=%d/plain", n), func(b *testing.B) { benchmarkRestore(b, n, false) })
		b.Run(fmt.Sprintf("n=%d/gzip", n), func(b *testing.B) { benchmarkRestore(b, n, true) })
	}
}
