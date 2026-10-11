package nameserver

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"sync/atomic"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

// requireLimit asserts that log holds exactly one LIMIT_REACHED entry with these args.
func requireLimit(t *testing.T, log *logger.Logger, bound string, domain string, limit int, count int) {
	t.Helper()
	if got := dnstest.CountTag(log.Entries(), "LIMIT_REACHED"); got != 1 {
		t.Fatalf("LIMIT_REACHED entries = %d, want 1", got)
	}
	entry := dnstest.EntryByTag(log.Entries(), "LIMIT_REACHED")
	args := entry.Args
	if args["bound"] != bound || args["domain"] != domain || args["limit"] != limit || args["count"] != count {
		t.Fatalf("LIMIT_REACHED args = %v, want bound %s domain %s limit %d count %d", args, bound, domain, limit, count)
	}
	if entry.Module != "System" {
		t.Fatalf("LIMIT_REACHED module = %s, want System", entry.Module)
	}
}

func nsNames(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("ns%02d.example", i)
	}
	return out
}

func TestCapNames(t *testing.T) {
	for _, tc := range []struct {
		name string
		n    int
		want int
	}{
		{"at the bound", 32, 32},
		{"past the bound", 40, 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := testContext(t)
			names := nsNames(tc.n)
			if got := CapNames(ctx, "example", names); !slices.Equal(got, names[:tc.want]) {
				t.Fatalf("CapNames kept %v, want the first %d", got, tc.want)
			}
			if tc.n == tc.want {
				if got := dnstest.CountTag(logger.FromContext(ctx).Entries(), "LIMIT_REACHED"); got != 0 {
					t.Fatalf("LIMIT_REACHED entries = %d, want 0", got)
				}
				return
			}
			requireLimit(t, logger.FromContext(ctx), "ns_names_per_zone", "example", 32, 40)
		})
	}
}

func TestCapNamesLogsOncePerLogger(t *testing.T) {
	ctx, _ := testContext(t)
	first := logger.FromContext(ctx)
	CapNames(ctx, "example", nsNames(40))
	CapNames(ctx, "example", nsNames(40))
	requireLimit(t, first, "ns_names_per_zone", "example", 32, 40)

	second := logger.New()
	CapNames(logger.WithContext(ctx, second), "example", nsNames(40))
	requireLimit(t, second, "ns_names_per_zone", "example", 32, 40)
}

func TestCapAddresses(t *testing.T) {
	ctx, _ := testContext(t)
	var in []netip.Addr
	for i := 18; i >= 1; i-- {
		in = append(in, netip.AddrFrom4([4]byte{192, 0, 2, byte(i)}))
	}
	in = append(in, netip.MustParseAddr("192.0.2.5"))
	var got []string
	for _, addr := range CapAddresses(ctx, "ns.example", in) {
		got = append(got, addr.String())
	}
	want := []string{
		"192.0.2.1", "192.0.2.10", "192.0.2.11", "192.0.2.12", "192.0.2.13", "192.0.2.14", "192.0.2.15", "192.0.2.16",
		"192.0.2.17", "192.0.2.18", "192.0.2.2", "192.0.2.3", "192.0.2.4", "192.0.2.5", "192.0.2.6", "192.0.2.7",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("CapAddresses = %v, want %v", got, want)
	}
	requireLimit(t, logger.FromContext(ctx), "addresses_per_ns", "ns.example", 16, 18)
}

func TestCapNSMap(t *testing.T) {
	ctx, _ := testContext(t)
	m := map[string][]netip.Addr{}
	for _, name := range nsNames(33) {
		m[name] = nil
	}
	for i := range 17 {
		m["ns00.example"] = append(m["ns00.example"], netip.AddrFrom4([4]byte{192, 0, 2, byte(i + 1)}))
	}
	CapNSMap(ctx, "example", m)
	if _, ok := m["ns32.example"]; ok || len(m) != 32 {
		t.Fatalf("map holds %d names, ns32 kept %t; want 32 without ns32", len(m), ok)
	}
	if got := len(m["ns00.example"]); got != 16 {
		t.Fatalf("ns00 keeps %d addresses, want 16", got)
	}
	if got := dnstest.CountTag(logger.FromContext(ctx).Entries(), "LIMIT_REACHED"); got != 2 {
		t.Fatalf("LIMIT_REACHED entries = %d, want 2", got)
	}
}

func TestRunAddressCap(t *testing.T) {
	ctx, _ := testContext(t)
	log := logger.FromContext(ctx)
	var calls atomic.Int64
	hook := func(context.Context, string, string, string, *QueryOptions) (packet.Packet, error) {
		calls.Add(1)
		return packet.Packet{}, nil
	}
	servers := make([]Nameserver, MaxRunAddresses+2)
	for i := range servers {
		addr := netip.AddrFrom4([4]byte{198, 18, byte(i >> 8), byte(i)})
		servers[i] = newNS(t, ctx, fmt.Sprintf("ns%d.example", i), addr.String())
		servers[i].SetQueryHook(hook)
	}
	for _, ns := range servers {
		if _, err := ns.QueryWithOptions(ctx, "example", "SOA", nil); err != nil {
			t.Fatalf("query %s: %v", ns.String(), err)
		}
	}
	if got := calls.Load(); got != 512 {
		t.Fatalf("queries sent = %d, want 512", got)
	}
	requireLimit(t, log, "addresses_per_run", "ns512.example", 512, 513)

	if _, err := servers[0].QueryWithOptions(ctx, "example", "NS", nil); err != nil {
		t.Fatalf("requery: %v", err)
	}
	if got := calls.Load(); got != 513 {
		t.Fatalf("queries sent after a requery = %d, want 513", got)
	}
}

func TestSnapshotForRunStartsWithFreshLimits(t *testing.T) {
	base := NewCacheStore()
	for i := range MaxRunAddresses {
		base.admitAddress(netip.AddrFrom4([4]byte{198, 18, byte(i >> 8), byte(i)}))
	}
	if base.admitAddress(netip.MustParseAddr("198.19.0.1")) {
		t.Fatal("base store admitted a 513th address")
	}
	if !base.SnapshotForRun().admitAddress(netip.MustParseAddr("198.19.0.1")) {
		t.Fatal("run snapshot refused its first address")
	}
}
