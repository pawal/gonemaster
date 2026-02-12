package nameserver

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func BenchmarkTransportAdaptationMixedNameservers(b *testing.B) {
	cases := []struct {
		name         string
		adaptive     bool
		fastFail     int
		blacklistJit float64
	}{
		{name: "baseline", adaptive: false, fastFail: 0, blacklistJit: 0.5},
		{name: "adaptive_timeout", adaptive: true, fastFail: 0, blacklistJit: 0.5},
		{name: "adaptive_and_fast_fail", adaptive: true, fastFail: 2, blacklistJit: 0.5},
	}

	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			prof, err := profile.Default()
			if err != nil {
				b.Fatalf("profile default: %v", err)
			}
			prof.Resolver.Defaults.AdaptiveTimeout = tc.adaptive
			prof.Resolver.Defaults.FastFailTimeoutCount = tc.fastFail
			prof.Resolver.Defaults.Timeout = 1

			ctx := context.Background()
			ctx = profile.WithContext(ctx, prof)
			ctx = logger.WithContext(ctx, logger.New())
			ctx = WithCache(ctx, NewCacheStore())

			var networkCalls atomic.Int64
			var timeoutErrors atomic.Int64
			var servfailResponses atomic.Int64

			nameservers := make([]Nameserver, 0, 3)
			healthy := mustBenchmarkNameserver(b, "ns-healthy.example", "192.0.2.210")
			slow := mustBenchmarkNameserver(b, "ns-slow.example", "192.0.2.211")
			rateLimited := mustBenchmarkNameserver(b, "ns-rate.example", "192.0.2.212")

			if healthy.state != nil {
				healthy.state.blacklist.jitterFn = func() float64 { return tc.blacklistJit }
			}
			if slow.state != nil {
				slow.state.blacklist.jitterFn = func() float64 { return tc.blacklistJit }
			}
			if rateLimited.state != nil {
				rateLimited.state.blacklist.jitterFn = func() float64 { return tc.blacklistJit }
			}

			healthy.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
				networkCalls.Add(1)
				return benchmarkPacketWithRcode(dns.RcodeSuccess), nil
			})
			slow.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
				networkCalls.Add(1)
				timeoutErrors.Add(1)
				return packet.Packet{}, &net.DNSError{Err: "i/o timeout", IsTimeout: true}
			})
			rateLimited.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
				networkCalls.Add(1)
				servfailResponses.Add(1)
				return benchmarkPacketWithRcode(dns.RcodeServerFailure), nil
			})

			nameservers = append(nameservers, healthy, slow, rateLimited)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ns := nameservers[i%len(nameservers)]
				qname := fmt.Sprintf("bench-%s-%d.example.com", tc.name, i)
				_, _ = ns.QueryWithOptions(ctx, qname, "SOA", nil)
			}
			b.StopTimer()

			if b.N > 0 {
				total := float64(b.N)
				b.ReportMetric(float64(networkCalls.Load())/total, "network_calls/op")
				b.ReportMetric(float64(timeoutErrors.Load())/total, "timeout_err/op")
				b.ReportMetric(float64(servfailResponses.Load())/total, "servfail/op")
			}
		})
	}
}

func mustBenchmarkNameserver(b *testing.B, name string, address string) Nameserver {
	b.Helper()
	ns, err := New(name, address, nil)
	if err != nil {
		b.Fatalf("new nameserver %s/%s: %v", name, address, err)
	}
	return ns
}

func benchmarkPacketWithRcode(rcode int) packet.Packet {
	msg := new(dns.Msg)
	msg.Response = true
	msg.Rcode = rcode
	return packet.Packet{Msg: msg}
}

