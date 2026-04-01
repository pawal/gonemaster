package nameserver

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func TestNameserverConcurrencyCapDisabledAllowsParallelQueries(t *testing.T) {
	t.Parallel()

	cache := NewCacheStore()
	nsA, err := NewWithCache(cache, "ns-a.example", "192.0.2.90", nil)
	if err != nil {
		t.Fatalf("new nameserver A: %v", err)
	}
	nsB, err := NewWithCache(cache, "ns-b.example", "192.0.2.90", nil)
	if err != nil {
		t.Fatalf("new nameserver B: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.NameserverConcurrency = 0

	var active int32
	var maxActive int32
	var calls int32
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	hook := func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		cur := atomic.AddInt32(&active, 1)
		for {
			prev := atomic.LoadInt32(&maxActive)
			if cur <= prev || atomic.CompareAndSwapInt32(&maxActive, prev, cur) {
				break
			}
		}
		atomic.AddInt32(&calls, 1)
		started <- struct{}{}
		<-release
		atomic.AddInt32(&active, -1)
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	}
	nsA.SetQueryHook(hook)
	nsB.SetQueryHook(hook)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = nsA.QueryWithOptions(ctx, "parallel-a.example", "A", nil)
	}()
	go func() {
		defer wg.Done()
		_, _ = nsB.QueryWithOptions(ctx, "parallel-b.example", "A", nil)
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(300 * time.Millisecond):
			t.Fatalf("expected both queries to enter network hook without cap")
		}
	}

	if got := atomic.LoadInt32(&maxActive); got < 2 {
		t.Fatalf("max active queries = %d, want at least 2", got)
	}

	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("network calls = %d, want 2", got)
	}
}

func TestNameserverConcurrencyCapSerializesAcrossSnapshots(t *testing.T) {
	t.Parallel()

	root := NewCacheStore()
	runA := root.SnapshotForRun()
	runB := root.SnapshotForRun()

	nsA, err := NewWithCache(runA, "ns-a.example", "192.0.2.91", nil)
	if err != nil {
		t.Fatalf("new nameserver A: %v", err)
	}
	nsB, err := NewWithCache(runB, "ns-b.example", "192.0.2.91", nil)
	if err != nil {
		t.Fatalf("new nameserver B: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.NameserverConcurrency = 1

	var active int32
	var maxActive int32
	var calls int32
	started := make(chan string, 2)
	release := make(chan struct{})
	hook := func(_ context.Context, qname string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		cur := atomic.AddInt32(&active, 1)
		for {
			prev := atomic.LoadInt32(&maxActive)
			if cur <= prev || atomic.CompareAndSwapInt32(&maxActive, prev, cur) {
				break
			}
		}
		atomic.AddInt32(&calls, 1)
		started <- qname
		<-release
		atomic.AddInt32(&active, -1)
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	}
	nsA.SetQueryHook(hook)
	nsB.SetQueryHook(hook)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = nsA.QueryWithOptions(ctx, "serial-a.example", "A", nil)
	}()
	select {
	case <-started:
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("expected first query to start")
	}

	go func() {
		defer wg.Done()
		_, _ = nsB.QueryWithOptions(ctx, "serial-b.example", "A", nil)
	}()

	select {
	case q := <-started:
		t.Fatalf("second query started before cap was released: %s", q)
	case <-time.After(120 * time.Millisecond):
	}

	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Fatalf("max active queries = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("network calls = %d, want 2", got)
	}
}

func TestNameserverConcurrencyCapWaitCancellationReleasesInflight(t *testing.T) {
	t.Parallel()

	cache := NewCacheStore()
	ns, err := NewWithCache(cache, "ns.example", "192.0.2.92", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.NameserverConcurrency = 1

	release := make(chan struct{})
	started := make(chan struct{}, 2)
	var calls int32
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		atomic.AddInt32(&calls, 1)
		started <- struct{}{}
		<-release
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	firstDone := make(chan error, 1)
	go func() {
		_, err := ns.QueryWithOptions(ctx, "first.example", "A", nil)
		firstDone <- err
	}()

	select {
	case <-started:
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("expected first query to reach network hook")
	}

	waitCtx, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
	defer cancel()
	_, err = ns.QueryWithOptions(waitCtx, "second.example", "A", nil)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation while waiting for cap, got %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected waiting query not to hit network, got %d calls", got)
	}

	close(release)
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatalf("first query failed: %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("first query did not complete after release")
	}

	_, err = ns.QueryWithOptions(ctx, "second.example", "A", nil)
	if err != nil {
		t.Fatalf("retry after canceled wait failed: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("network calls = %d, want 2", got)
	}
}

func TestResolveNameserverConcurrencyLimit(t *testing.T) {
	t.Parallel()

	if got := resolveNameserverConcurrencyLimit(nil); got != 0 {
		t.Fatalf("nil profile limit = %d, want 0", got)
	}

	prof := profile.New()
	prof.Resolver.Defaults.NameserverConcurrency = -3
	if got := resolveNameserverConcurrencyLimit(prof); got != 0 {
		t.Fatalf("negative profile limit = %d, want 0", got)
	}

	prof.Resolver.Defaults.NameserverConcurrency = 7
	if got := resolveNameserverConcurrencyLimit(prof); got != 7 {
		t.Fatalf("positive profile limit = %d, want 7", got)
	}
}
