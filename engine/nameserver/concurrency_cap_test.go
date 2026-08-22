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

// concurrencyProbe records how many hook calls overlap and how many ran.
type concurrencyProbe struct {
	active atomic.Int32
	max    atomic.Int32
	calls  atomic.Int32
}

// enter marks one call in flight and returns the function that ends it.
func (p *concurrencyProbe) enter() func() {
	p.calls.Add(1)
	cur := p.active.Add(1)
	for {
		prev := p.max.Load()
		if cur <= prev || p.max.CompareAndSwap(prev, cur) {
			break
		}
	}
	return func() { p.active.Add(-1) }
}

func (p *concurrencyProbe) maxActive() int32 { return p.max.Load() }

func (p *concurrencyProbe) callCount() int32 { return p.calls.Load() }

// okPacket is the minimal NOERROR answer the cap hooks hand back.
func okPacket() packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	return packet.Packet{Msg: msg}
}

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

	var probe concurrencyProbe
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	hook := func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		done := probe.enter()
		started <- struct{}{}
		<-release
		done()
		return okPacket(), nil
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

	for range 2 {
		select {
		case <-started:
		case <-time.After(300 * time.Millisecond):
			t.Fatalf("expected both queries to enter network hook without cap")
		}
	}

	if got := probe.maxActive(); got < 2 {
		t.Fatalf("max active queries = %d, want at least 2", got)
	}

	close(release)
	wg.Wait()

	if got := probe.callCount(); got != 2 {
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

	var probe concurrencyProbe
	started := make(chan string, 2)
	release := make(chan struct{})
	hook := func(_ context.Context, qname string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		done := probe.enter()
		started <- qname
		<-release
		done()
		return okPacket(), nil
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

	if got := probe.maxActive(); got != 1 {
		t.Fatalf("max active queries = %d, want 1", got)
	}
	if got := probe.callCount(); got != 2 {
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
	var probe concurrencyProbe
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		done := probe.enter()
		defer done()
		started <- struct{}{}
		<-release
		return okPacket(), nil
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

	if got := probe.callCount(); got != 1 {
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
	if got := probe.callCount(); got != 2 {
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
