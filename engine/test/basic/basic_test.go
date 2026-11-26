package basic

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestBasic02NoDelegation(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	z := tctest.Zone(t, "example", tctest.Recursor(t, nil))

	entries, err := Basic02(ctx, z)
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}
	tctest.RequireTags(t, entries, "B02_NO_DELEGATION")
	if entries[0].Tag != "TEST_CASE_START" || entries[len(entries)-1].Tag != "TEST_CASE_END" {
		t.Fatalf("expected test case start/end markers")
	}
}

func TestBasic02AuthResponseSOA(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := tctest.Recursor(t, map[string]map[string][]string{
		".": {
			"a.root": {"192.0.2.1"},
		},
	})

	tctest.NSOn(t, ctx, r, "a.root", "192.0.2.1", func(q tctest.Query) packet.Packet {
		name := strings.ToLower(q.Name)
		kind := strings.ToUpper(q.Type)
		switch {
		case name == "." && kind == "NS":
			return nsPacket(".", "a.root")
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root")
		default:
			return packet.Packet{}
		}
	})

	z := tctest.Zone(t, ".", r)

	entries, err := Basic02(ctx, z)
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "B02_AUTH_RESPONSE_SOA")
	servers := tctest.Servers(t, entry.Args["servers"])
	if len(servers) != 1 {
		t.Fatalf("expected one typed server for B02_AUTH_RESPONSE_SOA, got %#v", entry.Args["servers"])
	}
	if servers[0]["ns"] != "a.root" || servers[0]["address"] != "192.0.2.1" {
		t.Fatalf("unexpected typed server payload: %#v", servers[0])
	}
	if got, want := tctest.Strings(t, entry.Args, "addresses"), []string{"192.0.2.1"}; !slices.Equal(got, want) {
		t.Fatalf("addresses = %v, want %v", got, want)
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
}

func TestBasic02ParallelQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		baseCtx, prof, _ := testhelpers.Context(t)
		prof.Resolver.Defaults.Parallel = 2

		r := tctest.Recursor(t, map[string]map[string][]string{
			".": {
				"a.root": {"192.0.2.1"},
				"b.root": {"192.0.2.2"},
			},
		})

		gate := tctest.NewGate()

		// Only a.root blocks; b.root records its arrival and answers.
		hook := func(id string, block bool) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				name := strings.ToLower(q.Name)
				kind := strings.ToUpper(q.Type)
				switch {
				case name == "." && kind == "NS":
					return nsPacketMulti(".", "a.root", "b.root")
				case name == "." && kind == "SOA":
					if block {
						gate.Arrive(id)
					} else {
						gate.Record(id)
					}
					return soaPacket(".", id, "hostmaster.root")
				default:
					return packet.Packet{}
				}
			}
		}

		tctest.NSOn(t, baseCtx, r, "a.root", "192.0.2.1", hook("a.root", true))
		tctest.NSOn(t, baseCtx, r, "b.root", "192.0.2.2", hook("b.root", false))

		z := tctest.Zone(t, ".", r)

		done := make(chan struct{})
		var entries []*logger.Entry
		var basicErr error
		go func() {
			entries, basicErr = Basic02(baseCtx, z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "a.root", "b.root")
		gate.Release()

		<-done
		if basicErr != nil {
			t.Fatalf("basic02: %v", basicErr)
		}

		var enabled []string
		var enabledAddresses []string
		for _, entry := range tctest.All(entries, "IPV4_ENABLED") {
			tctest.RequireArgShape(t, entry, tctest.ArgShape{})
			if ns, ok := entry.Args["ns"].(string); ok {
				enabled = append(enabled, ns)
			}
			if address, ok := entry.Args["address"].(string); ok {
				enabledAddresses = append(enabledAddresses, address)
			}
		}
		if len(enabled) < 2 {
			t.Fatalf("expected IPV4_ENABLED entries for both nameservers, got %v", enabled)
		}
		if enabled[0] != "a.root" || enabled[1] != "b.root" {
			t.Fatalf("expected deterministic nameserver order, got %v", enabled)
		}
		if len(enabledAddresses) < 2 {
			t.Fatalf("expected IPV4_ENABLED address args for both nameservers, got %v", enabledAddresses)
		}
		if enabledAddresses[0] != "192.0.2.1" || enabledAddresses[1] != "192.0.2.2" {
			t.Fatalf("expected deterministic address order, got %v", enabledAddresses)
		}
		entry := tctest.RequireTag(t, entries, "B02_AUTH_RESPONSE_SOA")
		servers := tctest.Servers(t, entry.Args["servers"])
		if len(servers) != 2 {
			t.Fatalf("expected two typed servers for B02_AUTH_RESPONSE_SOA, got %#v", entry.Args["servers"])
		}
		if servers[0]["ns"] != "a.root" || servers[0]["address"] != "192.0.2.1" {
			t.Fatalf("unexpected first typed server payload: %#v", servers[0])
		}
		if servers[1]["ns"] != "b.root" || servers[1]["address"] != "192.0.2.2" {
			t.Fatalf("unexpected second typed server payload: %#v", servers[1])
		}
		addresses, ok := entry.Args["addresses"].([]string)
		if !ok || len(addresses) != 2 {
			t.Fatalf("expected two typed addresses for B02_AUTH_RESPONSE_SOA, got %#v", entry.Args["addresses"])
		}
		if addresses[0] != "192.0.2.1" || addresses[1] != "192.0.2.2" {
			t.Fatalf("expected deterministic address order, got %v", addresses)
		}
		if _, ok := entry.Args["ns_list"]; ok {
			t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
		}
	})
}

// The root zone has no working server; each case yields its detail tags in order.
func TestBasic02NoWorkingNS(t *testing.T) {
	rootNS := func(names ...string) func() packet.Packet {
		return func() packet.Packet { return nsPacketMulti(".", names...) }
	}
	tests := []struct {
		name        string
		root        replies
		ipv6Root    bool
		wantDetails []string
	}{
		{"SERVFAIL", replies{". NS": rootNS("a.root"), ". SOA": rcode(dns.RcodeServerFailure)}, false,
			[]string{"B02_UNEXPECTED_RCODE a.root/192.0.2.1 SERVFAIL"}},
		{"not authoritative", replies{". NS": rootNS("a.root"), ". SOA": rcode(dns.RcodeSuccess)}, false,
			[]string{"B02_NS_NOT_AUTH a.root/192.0.2.1"}},
		{"no SOA record", replies{". NS": rootNS("a.root"), ". SOA": emptyAnswerPacket}, false,
			[]string{"B02_NS_BROKEN a.root/192.0.2.1"}},
		{"no response", replies{". NS": rootNS("a.root")}, false,
			[]string{"B02_NS_NO_RESPONSE a.root/192.0.2.1"}},
		{"name without an address", replies{". NS": rootNS("b.root")}, false,
			[]string{"B02_NS_NO_IP_ADDR b.root"}},
		{"name without an address beside a failing one", replies{". NS": rootNS("a.root", "b.root"), ". SOA": rcode(dns.RcodeServerFailure)}, false,
			[]string{"B02_NS_NO_IP_ADDR b.root", "B02_UNEXPECTED_RCODE a.root/192.0.2.1 SERVFAIL"}},
		{"IPv6 server with IPv6 disabled", replies{". NS": rootNS("a.root", "b.root"), ". SOA": rcode(dns.RcodeServerFailure)}, true,
			[]string{"B02_UNEXPECTED_RCODE a.root/192.0.2.1 SERVFAIL"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, prof, _ := testhelpers.Context(t)
			prof.Net.IPv6 = false

			roots := map[string][]string{"a.root": {"192.0.2.1"}}
			if tt.ipv6Root {
				roots["b.root"] = []string{"2001:db8::2"}
			}
			r := tctest.Recursor(t, map[string]map[string][]string{".": roots})
			for name, addrs := range roots {
				tctest.NSOn(t, ctx, r, name, addrs[0], tt.root.handler())
			}

			entries, err := Basic02(ctx, tctest.Zone(t, ".", r))
			if err != nil {
				t.Fatalf("basic02: %v", err)
			}

			tctest.RequireNoTag(t, entries, "B02_AUTH_RESPONSE_SOA")
			tctest.RequireArg(t, tctest.RequireTag(t, entries, "B02_NO_WORKING_NS"), "domain", ".")
			var details []string
			for _, entry := range entries {
				if !strings.HasPrefix(entry.Tag, "B02_") || entry.Tag == "B02_NO_WORKING_NS" {
					continue
				}
				if _, ok := entry.Args["nsname"]; ok {
					t.Fatalf("legacy key nsname in %s", entry.Tag)
				}
				detail := entry.Tag + " " + entry.Args["ns"].(string)
				if address, ok := entry.Args["address"].(string); ok {
					detail += "/" + address
				}
				if rcode, ok := entry.Args["rcode"].(string); ok {
					detail += " " + rcode
				}
				details = append(details, detail)
			}
			if !slices.Equal(details, tt.wantDetails) {
				t.Fatalf("details = %q, want %q", details, tt.wantDetails)
			}
			if tt.ipv6Root {
				tctest.RequireArg(t, tctest.RequireTag(t, entries, "IPV6_DISABLED"), "ns", "b.root")
			}
		})
	}
}

// Eight broken servers are reported in name order whatever order the probes finish in.
func TestBasic02DetailTagsSorted(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Resolver.Defaults.Parallel = 8

	var names, want []string
	roots := map[string][]string{}
	for i := 1; i <= 8; i++ {
		name := fmt.Sprintf("r%d.root", i)
		names = append(names, name)
		roots[name] = []string{fmt.Sprintf("192.0.2.%d", i)}
		want = append(want, name)
	}
	r := tctest.Recursor(t, map[string]map[string][]string{".": roots})
	for _, name := range names {
		tctest.NSOn(t, ctx, r, name, roots[name][0], replies{
			". NS":  func() packet.Packet { return nsPacketMulti(".", names...) },
			". SOA": emptyAnswerPacket,
		}.handler())
	}

	entries, err := Basic02(ctx, tctest.Zone(t, ".", r))
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}

	if got := tctest.ArgValues(entries, "B02_NS_BROKEN", "ns"); !slices.Equal(got, want) {
		t.Fatalf("B02_NS_BROKEN order = %v, want %v", got, want)
	}
}

// The lookup of ns1.example fails while ns.other resolves, so only ns1.example takes its address from the referral glue.
func TestBasic02GlueForUnresolvedName(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	nsOther := nsEntry{name: "ns.other", addr: net.IPv4(192, 0, 2, 70)}
	delegation := func() packet.Packet {
		return tctest.Response(tctest.NotAuthoritative(),
			tctest.Authority(tctest.NSRR("example", "ns1.example"), tctest.NSRR("example", "ns.other")),
			tctest.Additional(tctest.ARR("ns1.example", "192.0.2.53")))
	}
	var mu sync.Mutex
	parentNS := 0
	r := serveRoots(t, ctx, func(string) tctest.Handler {
		next := replies{
			"example SOA":   delegation,
			"example NS":    delegation,
			"ns.other A":    referral("other", nsOther),
			"ns.other AAAA": referral("other", nsOther),
		}.handler()
		return func(q tctest.Query) packet.Packet {
			if queryKey(q) == "example NS" {
				mu.Lock()
				parentNS++
				mu.Unlock()
			}
			return next(q)
		}
	}, "a.root")
	tctest.NSOn(t, ctx, r, "ns1.example", "192.0.2.53", apex("example", replies{
		"ns1.example A": emptyAnswerPacket,
	}.handler(), "ns1.example", "ns.other"))
	tctest.NSOn(t, ctx, r, "ns.other", "192.0.2.70", apex("other", replies{
		"example SOA": rcode(dns.RcodeServerFailure),
		"ns.other A":  func() packet.Packet { return aPacket("ns.other", nsOther.addr) },
	}.handler(), "ns.other"))

	entries, err := Basic02(ctx, tctest.Zone(t, "example", r))
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}

	tctest.RequireNoTag(t, entries, "B02_NO_WORKING_NS", "B02_NS_NO_IP_ADDR")
	if got, want := tctest.ArgValues(entries, "IPV4_ENABLED", "ns"), []string{"ns.other", "ns1.example"}; !slices.Equal(got, want) {
		t.Fatalf("probe order = %v, want %v", got, want)
	}
	auth := tctest.RequireTag(t, entries, "B02_AUTH_RESPONSE_SOA")
	if got, want := tctest.ServerEndpoints(t, auth.Args), []string{"ns1.example/192.0.2.53"}; !slices.Equal(got, want) {
		t.Fatalf("servers = %v, want %v", got, want)
	}
	if parentNS != 1 {
		t.Fatalf("the parent was asked example NS %d times, want 1", parentNS)
	}
}

// basic03Delegation delegates example to ns1.example, where wwwA answers the www.example A query.
func basic03Delegation(t *testing.T, ctx context.Context, wwwA func() packet.Packet) *zone.Zone {
	t.Helper()

	r := tctest.Recursor(t, map[string]map[string][]string{
		".": {
			"a.root": {"192.0.2.1"},
		},
		"example": {
			"ns1.example": {"192.0.2.53"},
		},
	})

	tctest.NSOn(t, ctx, r, "a.root", "192.0.2.1", func(q tctest.Query) packet.Packet {
		name := strings.ToLower(q.Name)
		kind := strings.ToUpper(q.Type)
		switch {
		case name == "example" && (kind == "SOA" || kind == "NS"):
			return referralPacket("example", "ns1.example", net.IPv4(192, 0, 2, 53))
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root")
		default:
			return packet.Packet{}
		}
	})

	tctest.NSOn(t, ctx, r, "ns1.example", "192.0.2.53", func(q tctest.Query) packet.Packet {
		name := strings.ToLower(q.Name)
		kind := strings.ToUpper(q.Type)
		switch {
		case name == "example" && kind == "SOA":
			return soaPacket("example", "ns1.example", "hostmaster.example")
		case name == "www.example" && kind == "A":
			return wwwA()
		default:
			return packet.Packet{}
		}
	})

	return tctest.Zone(t, "example", r)
}

func TestBasic03ARecords(t *testing.T) {
	cases := []struct {
		name      string
		wwwA      func() packet.Packet
		wantTag   string
		wantNoTag string
	}{
		{
			name:      "HasARecords",
			wwwA:      func() packet.Packet { return aPacket("www.example", net.IPv4(192, 0, 2, 99)) },
			wantTag:   "HAS_A_RECORDS",
			wantNoTag: "A_QUERY_NO_RESPONSES",
		},
		{
			name:      "NoARecords",
			wwwA:      emptyAnswerPacket,
			wantTag:   "NO_A_RECORDS",
			wantNoTag: "HAS_A_RECORDS",
		},
		{
			// No answer at all, not even an empty one.
			name:    "NoResponses",
			wwwA:    func() packet.Packet { return packet.Packet{} },
			wantTag: "A_QUERY_NO_RESPONSES",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)

			z := basic03Delegation(t, ctx, tc.wwwA)

			entries, err := Basic03(ctx, z)
			if err != nil {
				t.Fatalf("basic03: %v", err)
			}
			tctest.RequireTags(t, entries, tc.wantTag)
			if tc.wantNoTag != "" {
				tctest.RequireNoTag(t, entries, tc.wantNoTag)
			}
		})
	}
}

func TestBasic03ParallelQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		baseCtx, prof, _ := testhelpers.Context(t)
		prof.Resolver.Defaults.Parallel = 2

		r := tctest.Recursor(t, map[string]map[string][]string{
			".": {
				"a.root": {"192.0.2.1"},
			},
			"example": {
				"ns1.example": {"192.0.2.53"},
				"ns2.example": {"192.0.2.54"},
			},
		})

		tctest.NSOn(t, baseCtx, r, "a.root", "192.0.2.1", func(q tctest.Query) packet.Packet {
			name := strings.ToLower(q.Name)
			kind := strings.ToUpper(q.Type)
			switch {
			case name == "example" && (kind == "SOA" || kind == "NS"):
				return referralPacketMulti("example", []nsEntry{
					{name: "ns1.example", addr: net.IPv4(192, 0, 2, 53)},
					{name: "ns2.example", addr: net.IPv4(192, 0, 2, 54)},
				})
			case name == "." && kind == "SOA":
				return soaPacket(".", "a.root", "hostmaster.root")
			default:
				return packet.Packet{}
			}
		})

		gate := tctest.NewGate()

		// Only ns1.example blocks; ns2.example records its arrival and answers.
		nsHook := func(id string, addr net.IP, block bool) tctest.Handler {
			return func(q tctest.Query) packet.Packet {
				name := strings.ToLower(q.Name)
				kind := strings.ToUpper(q.Type)
				if name == "www.example" && kind == "A" {
					if block {
						gate.Arrive(id)
					} else {
						gate.Record(id)
					}
					return aPacket("www.example", addr)
				}
				if name == "example" && kind == "SOA" {
					return soaPacket("example", "ns1.example", "hostmaster.example")
				}
				return packet.Packet{}
			}
		}

		tctest.NSOn(t, baseCtx, r, "ns1.example", "192.0.2.53", nsHook("ns1.example", net.IPv4(192, 0, 2, 53), true))
		tctest.NSOn(t, baseCtx, r, "ns2.example", "192.0.2.54", nsHook("ns2.example", net.IPv4(192, 0, 2, 54), false))

		z := tctest.Zone(t, "example", r)

		done := make(chan struct{})
		var entries []*logger.Entry
		var basicErr error
		go func() {
			entries, basicErr = Basic03(baseCtx, z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "ns1.example", "ns2.example")
		gate.Release()

		<-done
		if basicErr != nil {
			t.Fatalf("basic03: %v", basicErr)
		}

		var enabled []string
		var enabledAddresses []string
		for _, entry := range tctest.All(entries, "IPV4_ENABLED") {
			tctest.RequireArgShape(t, entry, tctest.ArgShape{})
			if ns, ok := entry.Args["ns"].(string); ok {
				enabled = append(enabled, ns)
			}
			if address, ok := entry.Args["address"].(string); ok {
				enabledAddresses = append(enabledAddresses, address)
			}
		}
		if len(enabled) < 2 {
			t.Fatalf("expected IPV4_ENABLED entries for both nameservers, got %v", enabled)
		}
		if enabled[0] != "ns1.example" || enabled[1] != "ns2.example" {
			t.Fatalf("expected deterministic nameserver order, got %v", enabled)
		}
		if len(enabledAddresses) < 2 {
			t.Fatalf("expected IPV4_ENABLED address args for both nameservers, got %v", enabledAddresses)
		}
		if enabledAddresses[0] != "192.0.2.53" || enabledAddresses[1] != "192.0.2.54" {
			t.Fatalf("expected deterministic address order, got %v", enabledAddresses)
		}
		tctest.RequireTags(t, entries, "HAS_A_RECORDS")
	})
}

func TestBasic03ParallelOutputStable(t *testing.T) {
	runBasic03 := func(parallel int) []*logger.Entry {
		ctx, prof, _ := testhelpers.Context(t)
		prof.Resolver.Defaults.Parallel = parallel

		r := tctest.Recursor(t, map[string]map[string][]string{
			".": {
				"a.root": {"192.0.2.1"},
			},
			"example": {
				"ns1.example": {"192.0.2.53"},
				"ns2.example": {"192.0.2.54"},
			},
		})

		tctest.NSOn(t, ctx, r, "a.root", "192.0.2.1", replies{
			". SOA":       func() packet.Packet { return soaPacket(".", "a.root", "hostmaster.root") },
			"example SOA": referral("example", ns1Example, ns2Example),
			"example NS":  referral("example", ns1Example, ns2Example),
		}.handler())
		for _, ns := range []nsEntry{ns1Example, ns2Example} {
			tctest.NSOn(t, ctx, r, ns.name, ns.addr.String(), replies{
				"example SOA":   func() packet.Packet { return soaPacket("example", ns.name, "hostmaster.example") },
				"www.example A": func() packet.Packet { return aPacket("www.example", ns.addr) },
			}.handler())
		}

		z := tctest.Zone(t, "example", r)

		entries, err := Basic03(ctx, z)
		if err != nil {
			t.Fatalf("basic03: %v", err)
		}
		tctest.RequireTags(t, entries, "HAS_A_RECORDS")
		return entries
	}

	sequential := tctest.Normalize(runBasic03(1))
	parallel := tctest.Normalize(runBasic03(2))
	if len(sequential) == 0 {
		t.Fatalf("expected entries from the sequential run")
	}
	if !slices.Equal(sequential, parallel) {
		t.Fatalf("entries changed with parallelism: sequential=%v parallel=%v", sequential, parallel)
	}
}

var (
	ns1Example = nsEntry{name: "ns1.example", addr: net.IPv4(192, 0, 2, 53)}
	ns2Example = nsEntry{name: "ns2.example", addr: net.IPv4(192, 0, 2, 54)}
)

// basic01Entries runs Basic01 for name and fails the test on an error.
func basic01Entries(t *testing.T, ctx context.Context, name string, r *recursor.Recursor) []*logger.Entry {
	t.Helper()
	entries, err := Basic01(ctx, tctest.Zone(t, name, r))
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}
	return entries
}

// queryKey renders a query as "name TYPE", with the name lowercased.
func queryKey(q tctest.Query) string {
	return strings.ToLower(q.Name) + " " + strings.ToUpper(q.Type)
}

// replies maps a query key to the builder of its answer.
type replies map[string]func() packet.Packet

// handler answers from the map and leaves every other query unanswered.
func (r replies) handler() tctest.Handler {
	return func(q tctest.Query) packet.Packet {
		if build, ok := r[queryKey(q)]; ok {
			return build()
		}
		return packet.Packet{}
	}
}

// apex answers the SOA and NS queries for owner and passes every other query to next.
func apex(owner string, next tctest.Handler, nsnames ...string) tctest.Handler {
	return func(q tctest.Query) packet.Packet {
		switch queryKey(q) {
		case owner + " SOA":
			return soaPacket(owner, nsnames[0], "hostmaster."+nsnames[0])
		case owner + " NS":
			return nsPacketMulti(owner, nsnames...)
		}
		return next(q)
	}
}

// rootZone is apex for the root zone.
func rootZone(next tctest.Handler, roots ...string) tctest.Handler {
	return apex(".", next, roots...)
}

// serveRoots registers the roots at 192.0.2.1 onwards; next(root) answers what the root zone does not.
func serveRoots(t *testing.T, ctx context.Context, next func(root string) tctest.Handler, roots ...string) *recursor.Recursor {
	t.Helper()
	addrs := map[string][]string{}
	for i, root := range roots {
		addrs[root] = []string{fmt.Sprintf("192.0.2.%d", i+1)}
	}
	r := tctest.Recursor(t, map[string]map[string][]string{".": addrs})
	for _, root := range roots {
		tctest.NSOn(t, ctx, r, root, addrs[root][0], rootZone(next(root), roots...))
	}
	return r
}

// referral builds a referral of owner to servers.
func referral(owner string, servers ...nsEntry) func() packet.Packet {
	return func() packet.Packet { return referralPacketMulti(owner, servers) }
}

// soa builds an authoritative SOA answer for owner.
func soa(owner string, mname string) func() packet.Packet {
	return func() packet.Packet { return soaPacket(owner, mname, "hostmaster."+mname) }
}

// cname builds an authoritative CNAME answer for owner.
func cname(owner string, target string) func() packet.Packet {
	return func() packet.Packet {
		return tctest.Response(tctest.Answers(tctest.CNAMERR(owner, target)))
	}
}

// gluelessReferralPacket refers zoneName to nsName without an address record.
func gluelessReferralPacket(zoneName string, nsName string) packet.Packet {
	return tctest.Response(tctest.NotAuthoritative(), tctest.Authority(tctest.NSRR(zoneName, nsName)))
}

// rcode builds a non-authoritative answer with the given RCODE and no records.
func rcode(code uint16) func() packet.Packet {
	return func() packet.Packet { return rcodePacket(code) }
}

func soaPacket(owner string, mname string, rname string) packet.Packet {
	return tctest.Response(tctest.Answers(tctest.SOARR(owner,
		tctest.MName(mname), tctest.RName(rname))))
}

// Referrals carry the delegation in the authority section with AA clear.
func referralPacket(zoneName string, nsName string, nsAddr net.IP) packet.Packet {
	return referralPacketMulti(zoneName, []nsEntry{{name: nsName, addr: nsAddr}})
}

type nsEntry struct {
	name string
	addr net.IP
}

func referralPacketMulti(zoneName string, entries []nsEntry) packet.Packet {
	opts := []tctest.MsgOpt{tctest.NotAuthoritative()}
	for _, entry := range entries {
		opts = append(opts,
			tctest.Authority(tctest.NSRR(zoneName, entry.name)),
			tctest.Additional(tctest.ARR(entry.name, entry.addr.String())))
	}
	return tctest.Response(opts...)
}

func aPacket(owner string, addr net.IP) packet.Packet {
	return tctest.Response(tctest.Answers(tctest.ARR(owner, addr.String())))
}

func nsPacket(owner string, nsname string) packet.Packet {
	return tctest.Response(tctest.Answers(tctest.NSRR(owner, nsname)))
}

func nsPacketMulti(owner string, nsnames ...string) packet.Packet {
	return tctest.Response(tctest.Answers(tctest.NSRRs(owner, nsnames...)...))
}

func rcodePacket(rcode uint16) packet.Packet {
	return tctest.Response(tctest.NotAuthoritative(), tctest.Rcode(rcode))
}

func emptyAnswerPacket() packet.Packet {
	return tctest.Response()
}

func nxdomainAAPacket() packet.Packet {
	return tctest.Response(tctest.NXDOMAIN())
}

func dnameAnswerPacket(owner string, target string) packet.Packet {
	return tctest.Response(tctest.Answers(tctest.DNAMERR(owner, target)))
}
