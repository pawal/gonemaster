package nameserver

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

func TestCacheStoreSnapshotForRunSharesWarmAddressCaches(t *testing.T) {
	base := NewCacheStore()
	ns, err := NewWithCache(base, "ns.example", "192.0.2.10", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if ns.state == nil || ns.state.cache == nil || ns.state.errorCache == nil {
		t.Fatalf("expected initialized nameserver state")
	}

	snapshot := base.SnapshotForRun()
	if snapshot == nil {
		t.Fatalf("expected snapshot")
	}
	if got := snapshot.AddressCacheCount(); got != 1 {
		t.Fatalf("snapshot address cache count = %d, want 1", got)
	}
	if got := snapshot.ErrorCacheCount(); got != 1 {
		t.Fatalf("snapshot error cache count = %d, want 1", got)
	}
	if got := snapshot.NameserverObjectCount(); got != 0 {
		t.Fatalf("snapshot nameserver object count = %d, want 0", got)
	}
}

func TestCacheStoreMergeWarmDataFromPreservesObjectIsolation(t *testing.T) {
	base := NewCacheStore()
	snapshot := base.SnapshotForRun()
	if _, err := NewWithCache(snapshot, "ns.example", "192.0.2.11", nil); err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if got := snapshot.NameserverObjectCount(); got != 1 {
		t.Fatalf("snapshot nameserver object count = %d, want 1", got)
	}

	base.MergeWarmDataFrom(snapshot)

	if got := base.AddressCacheCount(); got != 1 {
		t.Fatalf("base address cache count = %d, want 1", got)
	}
	if got := base.ErrorCacheCount(); got != 1 {
		t.Fatalf("base error cache count = %d, want 1", got)
	}
	if got := base.NameserverObjectCount(); got != 0 {
		t.Fatalf("base nameserver object count = %d, want 0", got)
	}
}

func TestCacheStoreMergeWarmDataFromNilNoop(t *testing.T) {
	base := NewCacheStore()
	base.MergeWarmDataFrom(nil)
	if got := base.AddressCacheCount(); got != 0 {
		t.Fatalf("base address cache count = %d, want 0", got)
	}
}

func TestCacheStoreSnapshotForRunCoalescesInflightAcrossSnapshots(t *testing.T) {
	base := NewCacheStore()
	snapshotA := base.SnapshotForRun()
	snapshotB := base.SnapshotForRun()

	nsA, err := NewWithCache(snapshotA, "ns.example", "192.0.2.77", nil)
	if err != nil {
		t.Fatalf("new nameserver A: %v", err)
	}
	nsB, err := NewWithCache(snapshotB, "ns.example", "192.0.2.77", nil)
	if err != nil {
		t.Fatalf("new nameserver B: %v", err)
	}

	var callsA atomic.Int32
	var callsB atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})

	queryHook := func(counter *atomic.Int32) func(context.Context, string, string, string, *QueryOptions) (packet.Packet, error) {
		return func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
			counter.Add(1)
			select {
			case started <- struct{}{}:
			default:
			}
			<-release

			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			msg.Answer = []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "coalesce.example.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					A: net.IPv4(192, 0, 2, 77),
				},
			}
			return packet.Packet{Msg: msg}, nil
		}
	}

	nsA.SetQueryHook(queryHook(&callsA))
	nsB.SetQueryHook(queryHook(&callsB))

	errCh := make(chan error, 2)
	go func() {
		_, err := nsA.QueryWithOptions(context.Background(), "coalesce.example", "A", nil)
		errCh <- err
	}()

	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected one network query to start")
	}

	go func() {
		_, err := nsB.QueryWithOptions(context.Background(), "coalesce.example", "A", nil)
		errCh <- err
	}()

	time.Sleep(60 * time.Millisecond)
	if total := callsA.Load() + callsB.Load(); total != 1 {
		t.Fatalf("expected second query to wait on inflight owner, got %d calls before release", total)
	}
	close(release)

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("query %d error: %v", i+1, err)
		}
	}

	totalCalls := callsA.Load() + callsB.Load()
	if totalCalls != 1 {
		t.Fatalf("expected one deduplicated network call, got %d (A=%d B=%d)", totalCalls, callsA.Load(), callsB.Load())
	}
}

func TestCacheStoreSnapshotForRunDoesNotCoalesceAcrossDifferentRoots(t *testing.T) {
	rootA := NewCacheStore()
	rootB := NewCacheStore()
	snapshotA := rootA.SnapshotForRun()
	snapshotB := rootB.SnapshotForRun()

	nsA, err := NewWithCache(snapshotA, "ns.example", "192.0.2.78", nil)
	if err != nil {
		t.Fatalf("new nameserver A: %v", err)
	}
	nsB, err := NewWithCache(snapshotB, "ns.example", "192.0.2.78", nil)
	if err != nil {
		t.Fatalf("new nameserver B: %v", err)
	}

	var callsA atomic.Int32
	var callsB atomic.Int32
	started := make(chan struct{}, 2)
	release := make(chan struct{})

	queryHook := func(counter *atomic.Int32) func(context.Context, string, string, string, *QueryOptions) (packet.Packet, error) {
		return func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
			counter.Add(1)
			started <- struct{}{}
			<-release
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}, nil
		}
	}

	nsA.SetQueryHook(queryHook(&callsA))
	nsB.SetQueryHook(queryHook(&callsB))

	errCh := make(chan error, 2)
	go func() {
		_, err := nsA.QueryWithOptions(context.Background(), "isolate.example", "A", nil)
		errCh <- err
	}()
	go func() {
		_, err := nsB.QueryWithOptions(context.Background(), "isolate.example", "A", nil)
		errCh <- err
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("expected two independent network queries to start")
		}
	}
	close(release)

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("query %d error: %v", i+1, err)
		}
	}

	if callsA.Load() != 1 || callsB.Load() != 1 {
		t.Fatalf("expected no coalescing across different roots, got A=%d B=%d", callsA.Load(), callsB.Load())
	}
}
