package basic

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/asnlookup"
	"codeberg.org/pawal/gonemaster/engine/cachefile"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/recursor/recursortest"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
)

//go:embed testdata/bahnhof-nxdomain-contradiction.cache.json
var bahnhofNXDomainContradictionCache []byte

func TestBasic01Root(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	entries := basic01Entries(t, ctx, ".", tctest.Recursor(t, nil))

	if len(entries) < 2 {
		t.Fatalf("expected start and end markers, got %v", tctest.Tags(entries))
	}
	if entries[0].Tag != "TEST_CASE_START" {
		t.Fatalf("expected TEST_CASE_START, got %q", entries[0].Tag)
	}
	if entries[len(entries)-1].Tag != "TEST_CASE_END" {
		t.Fatalf("expected TEST_CASE_END, got %q", entries[len(entries)-1].Tag)
	}
	tctest.RequireTags(t, entries, "B01_CHILD_FOUND", "B01_ROOT_HAS_NO_PARENT")
}

func TestBasic01Undelegated(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	z := tctest.ZoneWithAddrs(t, "example", map[string][]string{
		"ns1.example": {"192.0.2.1"},
	})

	entries, err := Basic01(ctx, z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}
	tctest.RequireTags(t, entries, "B01_CHILD_FOUND", "B01_PARENT_DISREGARDED")
}

func TestBasic01ParentFoundTypedArgs(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := tctest.Recursor(t, map[string]map[string][]string{
		".": {
			"b.root": {"192.0.2.2"},
			"a.root": {"192.0.2.1"},
		},
	})
	// The NS answer lists b.root first; the typed args are sorted.
	root := rootZone(replies{"example SOA": referral("example", ns1Example)}.handler(), "b.root", "a.root")
	tctest.NSOn(t, ctx, r, "a.root", "192.0.2.1", root)
	tctest.NSOn(t, ctx, r, "b.root", "192.0.2.2", root)

	entries := basic01Entries(t, ctx, "example", r)

	entry := tctest.RequireTag(t, entries, "B01_PARENT_FOUND")
	var got []string
	for _, server := range tctest.Servers(t, entry.Args["servers"]) {
		got = append(got, fmt.Sprintf("%v/%v", server["ns"], server["address"]))
	}
	if want := []string{"a.root/192.0.2.1", "b.root/192.0.2.2"}; !slices.Equal(got, want) {
		t.Fatalf("servers in emitted order = %v, want %v", got, want)
	}
	if got, want := tctest.Strings(t, entry.Args, "addresses"), []string{"192.0.2.1", "192.0.2.2"}; !slices.Equal(got, want) {
		t.Fatalf("addresses = %v, want %v", got, want)
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
}

// A CNAMEError from the lookup of a glueless NS name surfaces as a CNAME_* tag.
func TestBasic01EmitsCNAMETagOnNSLookup(t *testing.T) {
	cases := []struct {
		name     string
		seedErr  *recursor.CNAMEError
		wantTag  string
		wantArgs map[string]string
	}{
		{
			name:    "unresolved/loop emits CNAME_TARGET_UNRESOLVED",
			seedErr: &recursor.CNAMEError{Reason: recursor.CNAMEUnresolved, Name: "ns.outside.test", Target: "loop.outside.test", Detail: "loop"},
			wantTag: "CNAME_TARGET_UNRESOLVED",
			wantArgs: map[string]string{
				"query_name":   "ns.outside.test",
				"cname_target": "loop.outside.test",
			},
		},
		{
			name:     "too-many emits CNAME_TOO_MANY_RECORDS",
			seedErr:  &recursor.CNAMEError{Reason: recursor.CNAMETooMany, Name: "ns.outside.test"},
			wantTag:  "CNAME_TOO_MANY_RECORDS",
			wantArgs: map[string]string{"query_name": "ns.outside.test"},
		},
		{
			name:     "chain-too-long emits CNAME_CHAIN_TOO_LONG",
			seedErr:  &recursor.CNAMEError{Reason: recursor.CNAMEChainTooLong, Name: "ns.outside.test", Target: "deep.outside.test"},
			wantTag:  "CNAME_CHAIN_TOO_LONG",
			wantArgs: map[string]string{"query_name": "ns.outside.test"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)

			r := tctest.Recursor(t, map[string]map[string][]string{
				".": {"a.root": {"192.0.2.1"}},
			})
			r.SetNegativeCacheTTL(60 * time.Second)
			recursortest.SeedCNAMEError(r, tc.seedErr, "ns.outside.test", []string{"A", "AAAA"})
			tctest.NSOn(t, ctx, r, "a.root", "192.0.2.1", rootZone(replies{}.handler(), "a.root", "ns.outside.test"))

			entries := basic01Entries(t, ctx, "example", r)

			entry := tctest.RequireTag(t, entries, tc.wantTag)
			for key, want := range tc.wantArgs {
				tctest.RequireArg(t, entry, key, want)
			}
		})
	}
}

// The only root answers the SOA query for example in each shape Basic01 classifies.
func TestBasic01ParentAnswerAtChild(t *testing.T) {
	tests := []struct {
		name      string
		root      replies
		wantChild bool
	}{
		{"referral", replies{"example SOA": referral("example", ns1Example)}, true},
		{"authoritative SOA", replies{"example SOA": soa("example", "a.root")}, true},
		{"NXDOMAIN", replies{"example SOA": nxdomainAAPacket}, false},
		{"NODATA without a DNAME", replies{"example SOA": emptyAnswerPacket, "example DNAME": emptyAnswerPacket}, false},
		{"NODATA and no answer to the DNAME query", replies{"example SOA": emptyAnswerPacket}, false},
		{"CNAME", replies{"example SOA": cname("example", "other.example")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)

			r := serveRoots(t, ctx, func(string) tctest.Handler { return tt.root.handler() }, "a.root")

			entries := basic01Entries(t, ctx, "example", r)

			tctest.RequireArg(t, tctest.RequireTag(t, entries, "B01_PARENT_FOUND"), "domain", ".")
			tctest.RequireNoTag(t, entries, "B01_SERVER_ZONE_ERROR", "B01_INCONSISTENT_DELEGATION", "B01_CHILD_IS_ALIAS", "B01_PARENT_UNDETERMINED")
			if tt.wantChild {
				tctest.RequireArg(t, tctest.RequireTag(t, entries, "B01_CHILD_FOUND"), "domain", "example")
				tctest.RequireNoTag(t, entries, "B01_NO_CHILD")
				return
			}
			tctest.RequireNoTag(t, entries, "B01_CHILD_FOUND")
			noChild := tctest.RequireTag(t, entries, "B01_NO_CHILD")
			tctest.RequireArg(t, noChild, "domain_child", "example")
			tctest.RequireArg(t, noChild, "domain_super", ".")
		})
	}
}

// a.root delegates example; b.root answers for the name without a delegation.
func TestBasic01InconsistentDelegation(t *testing.T) {
	tests := []struct {
		name  string
		bRoot replies
	}{
		{"NXDOMAIN", replies{"example SOA": nxdomainAAPacket}},
		{"NODATA", replies{"example SOA": emptyAnswerPacket, "example DNAME": emptyAnswerPacket}},
		{"CNAME", replies{"example SOA": cname("example", "other.example")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)

			r := serveRoots(t, ctx, func(root string) tctest.Handler {
				if root == "a.root" {
					return replies{"example SOA": referral("example", ns1Example)}.handler()
				}
				return tt.bRoot.handler()
			}, "a.root", "b.root")

			entries := basic01Entries(t, ctx, "example", r)

			tctest.RequireTags(t, entries, "B01_PARENT_FOUND", "B01_CHILD_FOUND")
			tctest.RequireNoTag(t, entries, "B01_NO_CHILD", "B01_PARENT_UNDETERMINED")
			tctest.RequireCount(t, entries, "B01_INCONSISTENT_DELEGATION", 1)
			inconsistent := tctest.RequireTag(t, entries, "B01_INCONSISTENT_DELEGATION")
			tctest.RequireArg(t, inconsistent, "domain_child", "example")
			tctest.RequireArg(t, inconsistent, "domain_parent", ".")
			if got, want := tctest.ServerEndpoints(t, inconsistent.Args), []string{"b.root/192.0.2.2"}; !slices.Equal(got, want) {
				t.Fatalf("servers = %v, want %v", got, want)
			}
		})
	}
}

// Both roots deny example.
func TestBasic01NoChildFromEveryRoot(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := serveRoots(t, ctx, func(string) tctest.Handler {
		return replies{"example SOA": nxdomainAAPacket}.handler()
	}, "a.root", "b.root")

	entries := basic01Entries(t, ctx, "example", r)

	parent := tctest.RequireTag(t, entries, "B01_PARENT_FOUND")
	if got, want := tctest.ServerEndpoints(t, parent.Args), []string{"a.root/192.0.2.1", "b.root/192.0.2.2"}; !slices.Equal(got, want) {
		t.Fatalf("servers = %v, want %v", got, want)
	}
	tctest.RequireNoTag(t, entries, "B01_CHILD_FOUND", "B01_INCONSISTENT_DELEGATION")
	noChild := tctest.RequireTag(t, entries, "B01_NO_CHILD")
	tctest.RequireArg(t, noChild, "domain_child", "example")
	tctest.RequireArg(t, noChild, "domain_super", ".")
}

// The root refers example without glue, so the server address comes from a lookup.
func TestBasic01GluelessReferral(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := serveRoots(t, ctx, func(string) tctest.Handler {
		return replies{
			"example SOA":   func() packet.Packet { return gluelessReferralPacket("example", "ns1.example") },
			"ns1.example A": func() packet.Packet { return aPacket("ns1.example", net.IPv4(192, 0, 2, 53)) },
		}.handler()
	}, "a.root")
	tctest.NSOn(t, ctx, r, "ns1.example", "192.0.2.53", apex("example", replies{
		"child.example SOA": referral("child.example", nsEntry{name: "ns.child.example", addr: net.IPv4(192, 0, 2, 55)}),
	}.handler(), "ns1.example"))

	entries := basic01Entries(t, ctx, "child.example", r)

	tctest.RequireTags(t, entries, "B01_CHILD_FOUND")
	tctest.RequireNoTag(t, entries, "B01_NO_CHILD", "B01_SERVER_ZONE_ERROR")
	parent := tctest.RequireTag(t, entries, "B01_PARENT_FOUND")
	tctest.RequireArg(t, parent, "domain", "example")
	if got, want := tctest.ServerEndpoints(t, parent.Args), []string{"ns1.example/192.0.2.53"}; !slices.Equal(got, want) {
		t.Fatalf("servers = %v, want %v", got, want)
	}
}

// Two servers of example disagree on whether sub.example is a zone of its own.
func TestBasic01ParentUndetermined(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := serveRoots(t, ctx, func(string) tctest.Handler {
		return replies{"example SOA": referral("example", ns1Example, ns2Example)}.handler()
	}, "a.root")
	child := referral("child.sub.example", nsEntry{name: "ns.child.example", addr: net.IPv4(192, 0, 2, 55)})
	tctest.NSOn(t, ctx, r, "ns1.example", "192.0.2.53", apex("example", replies{
		"sub.example SOA":       emptyAnswerPacket,
		"child.sub.example SOA": child,
	}.handler(), "ns1.example", "ns2.example"))
	tctest.NSOn(t, ctx, r, "ns2.example", "192.0.2.54", apex("example", apex("sub.example", replies{
		"child.sub.example SOA": child,
	}.handler(), "ns2.example"), "ns1.example", "ns2.example"))

	entries := basic01Entries(t, ctx, "child.sub.example", r)

	tctest.RequireTags(t, entries, "B01_CHILD_FOUND")
	tctest.RequireNoTag(t, entries, "B01_NO_CHILD", "B01_PARENT_NOT_FOUND")
	parents := tctest.ArgValues(entries, "B01_PARENT_FOUND", "domain")
	slices.Sort(parents)
	if want := []string{"example", "sub.example"}; !slices.Equal(parents, want) {
		t.Fatalf("parents = %v, want %v", parents, want)
	}
	undetermined := tctest.RequireTag(t, entries, "B01_PARENT_UNDETERMINED")
	if got, want := tctest.ServerEndpoints(t, undetermined.Args), []string{"ns1.example/192.0.2.53", "ns2.example/192.0.2.54"}; !slices.Equal(got, want) {
		t.Fatalf("servers = %v, want %v", got, want)
	}
}

// The only root fails a query for its own zone, so no parent is established.
func TestBasic01RootZoneError(t *testing.T) {
	tests := []struct {
		name          string
		root          replies
		wantQueryType string
		wantReason    string
		wantRcode     string
	}{
		{"SOA SERVFAIL", replies{". SOA": rcode(dns.RcodeServerFailure)}, "SOA", "rcode", "SERVFAIL"},
		{"SOA NXDOMAIN", replies{". SOA": nxdomainAAPacket}, "SOA", "rcode", "NXDOMAIN"},
		{"SOA without AA", replies{". SOA": rcode(dns.RcodeSuccess)}, "SOA", "not_authoritative", "NOERROR"},
		{"no SOA record", replies{". SOA": emptyAnswerPacket}, "SOA", "soa_records", "NOERROR"},
		{"second SOA record of another owner", replies{". SOA": func() packet.Packet {
			return tctest.Response(tctest.Answers(tctest.SOARR("."), tctest.SOARR("example")))
		}}, "SOA", "soa_records", "NOERROR"},
		{"NS SERVFAIL", replies{". SOA": soa(".", "a.root"), ". NS": rcode(dns.RcodeServerFailure)}, "NS", "rcode", "SERVFAIL"},
		{"NS of another owner", replies{". SOA": soa(".", "a.root"), ". NS": func() packet.Packet { return nsPacketMulti("example", "a.root") }}, "NS", "ns_records", "NOERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)

			r := tctest.Recursor(t, map[string]map[string][]string{
				".": {"a.root": {"192.0.2.1"}},
			})
			tctest.NSOn(t, ctx, r, "a.root", "192.0.2.1", tt.root.handler())

			entries := basic01Entries(t, ctx, "example", r)

			tctest.RequireTags(t, entries, "B01_PARENT_NOT_FOUND")
			tctest.RequireNoTag(t, entries, "B01_PARENT_FOUND", "B01_CHILD_FOUND")
			tctest.RequireCount(t, entries, "B01_SERVER_ZONE_ERROR", 1)
			zoneError := tctest.RequireTag(t, entries, "B01_SERVER_ZONE_ERROR")
			tctest.RequireArg(t, zoneError, "ns", "a.root")
			tctest.RequireArg(t, zoneError, "address", "192.0.2.1")
			tctest.RequireArg(t, zoneError, "query_name", ".")
			tctest.RequireArg(t, zoneError, "query_type", tt.wantQueryType)
			tctest.RequireArg(t, zoneError, "reason", tt.wantReason)
			tctest.RequireArg(t, zoneError, "rcode", tt.wantRcode)
			noChild := tctest.RequireTag(t, entries, "B01_NO_CHILD")
			tctest.RequireArg(t, noChild, "domain_child", "example")
			tctest.RequireArg(t, noChild, "domain_super", ".")
		})
	}
}

// The parent's only server proves example, then gives an unusable answer for mid.example above the child.
func TestBasic01IntermediateNameError(t *testing.T) {
	tests := []struct {
		name          string
		mid           replies
		wantQueryType string
		wantReason    string
		wantRcode     string
	}{
		{"SERVFAIL", replies{"mid.example SOA": rcode(dns.RcodeServerFailure)}, "SOA", "rcode", "SERVFAIL"},
		{"REFUSED", replies{"mid.example SOA": rcode(dns.RcodeRefused)}, "SOA", "rcode", "REFUSED"},
		{"NXDOMAIN without AA", replies{"mid.example SOA": rcode(dns.RcodeNameError)}, "SOA", "not_authoritative", "NXDOMAIN"},
		{"NOERROR without AA", replies{"mid.example SOA": rcode(dns.RcodeSuccess)}, "SOA", "not_authoritative", "NOERROR"},
		{"referral upwards", replies{"mid.example SOA": referral("example", ns1Example)}, "SOA", "referral", "NOERROR"},
		{"no answer", replies{}, "SOA", "no_response", ""},
		{"SOA answered and NS fails", replies{"mid.example SOA": soa("mid.example", "ns1.example"), "mid.example NS": rcode(dns.RcodeServerFailure)}, "NS", "rcode", "SERVFAIL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)

			r := serveRoots(t, ctx, func(string) tctest.Handler {
				return replies{"example SOA": referral("example", ns1Example)}.handler()
			}, "a.root")
			server := replies{
				"child.mid.example SOA": referral("child.mid.example", nsEntry{name: "ns.child.example", addr: net.IPv4(192, 0, 2, 55)}),
			}
			maps.Copy(server, tt.mid)
			tctest.NSOn(t, ctx, r, "ns1.example", "192.0.2.53", apex("example", server.handler(), "ns1.example"))

			entries := basic01Entries(t, ctx, "child.mid.example", r)

			tctest.RequireTags(t, entries, "B01_PARENT_NOT_FOUND")
			tctest.RequireNoTag(t, entries, "B01_PARENT_FOUND", "B01_CHILD_FOUND", "B01_PARENT_NXDOMAIN_HIDES_DELEGATION")
			tctest.RequireCount(t, entries, "B01_SERVER_ZONE_ERROR", 1)
			zoneError := tctest.RequireTag(t, entries, "B01_SERVER_ZONE_ERROR")
			tctest.RequireArg(t, zoneError, "ns", "ns1.example")
			tctest.RequireArg(t, zoneError, "query_name", "mid.example")
			tctest.RequireArg(t, zoneError, "query_type", tt.wantQueryType)
			tctest.RequireArg(t, zoneError, "reason", tt.wantReason)
			if got, _ := zoneError.Args["rcode"].(string); got != tt.wantRcode {
				t.Fatalf("rcode = %q, want %q", got, tt.wantRcode)
			}
			noChild := tctest.RequireTag(t, entries, "B01_NO_CHILD")
			tctest.RequireArg(t, noChild, "domain_child", "child.mid.example")
			tctest.RequireArg(t, noChild, "domain_super", "example")
		})
	}
}

// A walk that gets no answer for the child suggests the deepest zone it reached, proved or only referred to.
func TestBasic01NoChildSuggestsDeepestZoneReached(t *testing.T) {
	tests := []struct {
		name      string
		example   tctest.Handler
		zoneError string
		wantSuper string
	}{
		{
			name: "deepest zone proved",
			example: apex("example", apex("sub.example", replies{
				"mid.sub.example SOA": rcode(dns.RcodeServerFailure),
			}.handler(), "ns1.example"), "ns1.example"),
			zoneError: "mid.sub.example",
			wantSuper: "sub.example",
		},
		{
			name:      "deepest zone referred, its server lame",
			example:   replies{"example SOA": rcode(dns.RcodeRefused)}.handler(),
			zoneError: "example",
			wantSuper: "example",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)

			r := serveRoots(t, ctx, func(string) tctest.Handler {
				return replies{"example SOA": referral("example", ns1Example)}.handler()
			}, "a.root")
			tctest.NSOn(t, ctx, r, "ns1.example", "192.0.2.53", tt.example)

			entries := basic01Entries(t, ctx, "child.mid.sub.example", r)

			tctest.RequireTags(t, entries, "B01_PARENT_NOT_FOUND")
			tctest.RequireNoTag(t, entries, "B01_PARENT_FOUND", "B01_CHILD_FOUND")
			if got := tctest.ArgValues(entries, "B01_SERVER_ZONE_ERROR", "query_name"); !slices.Equal(got, []string{tt.zoneError}) {
				t.Fatalf("zone error query names = %v, want [%s]", got, tt.zoneError)
			}
			noChild := tctest.RequireTag(t, entries, "B01_NO_CHILD")
			tctest.RequireArg(t, noChild, "domain_child", "child.mid.sub.example")
			tctest.RequireArg(t, noChild, "domain_super", tt.wantSuper)
		})
	}
}

// ns2.example is named only in the NS answer of example, not in the referral to it.
func TestBasic01ProbesServersOfTheZoneNSAnswer(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := serveRoots(t, ctx, func(string) tctest.Handler {
		return replies{"example SOA": referral("example", ns1Example)}.handler()
	}, "a.root")
	apexNS := func() packet.Packet {
		return tctest.Response(tctest.Answers(tctest.NSRRs("example", "ns1.example", "ns2.example")...),
			tctest.Additional(tctest.ARR("ns1.example", "192.0.2.53"), tctest.ARR("ns2.example", "192.0.2.54")))
	}
	tctest.NSOn(t, ctx, r, "ns1.example", "192.0.2.53", replies{
		"example SOA":       soa("example", "ns1.example"),
		"example NS":        apexNS,
		"child.example SOA": referral("child.example", nsEntry{name: "ns.child.example", addr: net.IPv4(192, 0, 2, 55)}),
	}.handler())
	var asked atomic.Int32
	tctest.NSOn(t, ctx, r, "ns2.example", "192.0.2.54", func(q tctest.Query) packet.Packet {
		asked.Add(1)
		return replies{
			"example SOA":       soa("example", "ns1.example"),
			"example NS":        apexNS,
			"child.example SOA": nxdomainAAPacket,
		}.handler()(q)
	})

	entries := basic01Entries(t, ctx, "child.example", r)

	if got := asked.Load(); got != 3 {
		t.Fatalf("ns2.example was asked %d times, want 3", got)
	}
	parent := tctest.RequireTag(t, entries, "B01_PARENT_FOUND")
	if got, want := tctest.ServerEndpoints(t, parent.Args), []string{"ns1.example/192.0.2.53", "ns2.example/192.0.2.54"}; !slices.Equal(got, want) {
		t.Fatalf("parent servers = %v, want %v", got, want)
	}
	tctest.RequireTags(t, entries, "B01_CHILD_FOUND")
	inconsistent := tctest.RequireTag(t, entries, "B01_INCONSISTENT_DELEGATION")
	if got, want := tctest.ServerEndpoints(t, inconsistent.Args), []string{"ns2.example/192.0.2.54"}; !slices.Equal(got, want) {
		t.Fatalf("inconsistent servers = %v, want %v", got, want)
	}
}

// A probe referral queues mid.sub.example before sub.example, whose server names a second server for it.
func TestBasic01WalksShallowestZoneFirst(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	nsA := nsEntry{name: "ns-a.example", addr: net.IPv4(192, 0, 2, 60)}
	nsB := nsEntry{name: "ns-b.example", addr: net.IPv4(192, 0, 2, 61)}
	nsSub := nsEntry{name: "ns.sub.example", addr: net.IPv4(192, 0, 2, 62)}
	r := serveRoots(t, ctx, func(string) tctest.Handler {
		return replies{"example SOA": referral("example", ns1Example, ns2Example)}.handler()
	}, "a.root")
	tctest.NSOn(t, ctx, r, "ns1.example", "192.0.2.53", apex("example", replies{
		"sub.example SOA":           nxdomainAAPacket,
		"child.mid.sub.example SOA": referral("mid.sub.example", nsA),
	}.handler(), "ns1.example", "ns2.example"))
	tctest.NSOn(t, ctx, r, "ns2.example", "192.0.2.54", apex("example", replies{
		"sub.example SOA": referral("sub.example", nsSub),
	}.handler(), "ns1.example", "ns2.example"))
	tctest.NSOn(t, ctx, r, nsSub.name, nsSub.addr.String(), apex("sub.example", replies{
		"mid.sub.example SOA": referral("mid.sub.example", nsA, nsB),
	}.handler(), nsSub.name))
	child := referral("child.mid.sub.example", nsEntry{name: "ns.child.example", addr: net.IPv4(192, 0, 2, 55)})
	tctest.NSOn(t, ctx, r, nsA.name, nsA.addr.String(), apex("mid.sub.example", replies{"child.mid.sub.example SOA": child}.handler(), nsA.name, nsB.name))
	tctest.NSOn(t, ctx, r, nsB.name, nsB.addr.String(), apex("mid.sub.example", replies{"child.mid.sub.example SOA": nxdomainAAPacket}.handler(), nsA.name, nsB.name))

	entries := basic01Entries(t, ctx, "child.mid.sub.example", r)

	tctest.RequireArg(t, tctest.RequireTag(t, entries, "B01_PARENT_FOUND"), "domain", "mid.sub.example")
	inconsistent := tctest.RequireTag(t, entries, "B01_INCONSISTENT_DELEGATION")
	if got, want := tctest.ServerEndpoints(t, inconsistent.Args), []string{"ns-b.example/192.0.2.61"}; !slices.Equal(got, want) {
		t.Fatalf("inconsistent servers = %v, want %v", got, want)
	}
	tctest.RequireNoTag(t, entries, "LOOP_PROTECTION", "B01_PARENT_UNDETERMINED")
}

// A referral without the AA bit holding a CNAME for the child is an answer without a delegation.
func TestBasic01CNAMEWithReferral(t *testing.T) {
	cnameReferral := func(extra ...dns.RR) func() packet.Packet {
		return func() packet.Packet {
			answers := append([]dns.RR{tctest.CNAMERR("example", "x.other")}, extra...)
			return tctest.Response(tctest.NotAuthoritative(), tctest.Answers(answers...), tctest.Authority(tctest.NSRR("other", "ns.other")))
		}
	}
	tests := []struct {
		name       string
		bRoot      func() packet.Packet
		wantAnswer bool
	}{
		{"CNAME only", cnameReferral(), true},
		{"CNAME with its RRSIG", cnameReferral(tctest.RRSIGRR("example", dns.TypeCNAME)), true},
		{"CNAME beside an A record", cnameReferral(tctest.ARR("example", "192.0.2.9")), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)

			r := serveRoots(t, ctx, func(root string) tctest.Handler {
				if root == "a.root" {
					return replies{"example SOA": referral("example", ns1Example)}.handler()
				}
				return replies{"example SOA": tt.bRoot}.handler()
			}, "a.root", "b.root")

			entries := basic01Entries(t, ctx, "example", r)

			tctest.RequireTags(t, entries, "B01_CHILD_FOUND")
			if !tt.wantAnswer {
				tctest.RequireNoTag(t, entries, "B01_INCONSISTENT_DELEGATION")
				zoneError := tctest.RequireTag(t, entries, "B01_SERVER_ZONE_ERROR")
				tctest.RequireArg(t, zoneError, "ns", "b.root")
				tctest.RequireArg(t, zoneError, "reason", "not_authoritative")
				return
			}
			tctest.RequireNoTag(t, entries, "B01_SERVER_ZONE_ERROR")
			inconsistent := tctest.RequireTag(t, entries, "B01_INCONSISTENT_DELEGATION")
			if got, want := tctest.ServerEndpoints(t, inconsistent.Args), []string{"b.root/192.0.2.2"}; !slices.Equal(got, want) {
				t.Fatalf("servers = %v, want %v", got, want)
			}
		})
	}
}

// A walk past its visit bound logs LOOP_PROTECTION and reports what it gathered.
func TestBasic01WalkVisitBound(t *testing.T) {
	if got := walkVisitLimit(2); got != 6 {
		t.Fatalf("walkVisitLimit(2) = %d, want 6", got)
	}
	tctest.Stub(t, &walkVisitLimit, func(int) int { return 1 })
	ctx, _, _ := testhelpers.Context(t)

	r := serveRoots(t, ctx, func(string) tctest.Handler {
		return replies{"example SOA": referral("example", ns1Example)}.handler()
	}, "a.root")
	var asked atomic.Int32
	tctest.NSOn(t, ctx, r, "ns1.example", "192.0.2.53", func(q tctest.Query) packet.Packet {
		asked.Add(1)
		return apex("example", replies{
			"child.example SOA": referral("child.example", nsEntry{name: "ns.child.example", addr: net.IPv4(192, 0, 2, 55)}),
		}.handler(), "ns1.example")(q)
	})

	entries := basic01Entries(t, ctx, "child.example", r)

	guard := tctest.RequireTag(t, entries, "LOOP_PROTECTION")
	tctest.RequireArg(t, guard, "caller", "basic.Basic01")
	tctest.RequireArg(t, guard, "child_zone_name", "child.example")
	tctest.RequireArg(t, guard, "zone_name", "example")
	tctest.RequireArg(t, guard, "intermediate_query_name", "example")
	if got := asked.Load(); got != 0 {
		t.Fatalf("ns1.example was asked %d times, want 0", got)
	}
	tctest.RequireTags(t, entries, "B01_PARENT_NOT_FOUND")
	tctest.RequireNoTag(t, entries, "B01_CHILD_FOUND")
	tctest.RequireArg(t, tctest.RequireTag(t, entries, "B01_NO_CHILD"), "domain_super", "example")
	if last := entries[len(entries)-1].Tag; last != "TEST_CASE_END" {
		t.Fatalf("last entry = %s, want TEST_CASE_END", last)
	}
}

// A server of example answers NXDOMAIN with AA for mid.example, above child.mid.example (RFC 8020).
func TestBasic01IntermediateNXDomain(t *testing.T) {
	child := referral("child.mid.example", nsEntry{name: "ns.child.example", addr: net.IPv4(192, 0, 2, 55)})
	type server struct {
		ns    nsEntry
		mid   func() packet.Packet
		child func() packet.Packet
	}
	tests := []struct {
		name          string
		servers       []server
		wantTags      []string
		wantNoTags    []string
		wantHides     []string
		wantNoChildOf string
	}{
		{
			name:       "the child is still delegated",
			servers:    []server{{ns1Example, nxdomainAAPacket, child}},
			wantTags:   []string{"B01_PARENT_FOUND", "B01_CHILD_FOUND"},
			wantNoTags: []string{"B01_NO_CHILD", "B01_INCONSISTENT_DELEGATION"},
			wantHides:  []string{"ns1.example/192.0.2.53"},
		},
		{
			name:          "the child is denied as well",
			servers:       []server{{ns1Example, nxdomainAAPacket, nxdomainAAPacket}},
			wantTags:      []string{"B01_PARENT_FOUND"},
			wantNoTags:    []string{"B01_CHILD_FOUND"},
			wantNoChildOf: "example",
		},
		{
			name: "one of two servers denies the name",
			servers: []server{
				{ns1Example, emptyAnswerPacket, child},
				{ns2Example, nxdomainAAPacket, child},
			},
			wantTags:   []string{"B01_PARENT_FOUND", "B01_CHILD_FOUND"},
			wantNoTags: []string{"B01_NO_CHILD", "B01_INCONSISTENT_DELEGATION"},
			wantHides:  []string{"ns2.example/192.0.2.54"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)

			var delegation []nsEntry
			var names []string
			for _, s := range tt.servers {
				delegation = append(delegation, s.ns)
				names = append(names, s.ns.name)
			}
			r := serveRoots(t, ctx, func(string) tctest.Handler {
				return replies{"example SOA": referral("example", delegation...)}.handler()
			}, "a.root")
			for _, s := range tt.servers {
				tctest.NSOn(t, ctx, r, s.ns.name, s.ns.addr.String(), apex("example", replies{
					"mid.example SOA":       s.mid,
					"child.mid.example SOA": s.child,
				}.handler(), names...))
			}

			entries := basic01Entries(t, ctx, "child.mid.example", r)

			tctest.RequireTags(t, entries, tt.wantTags...)
			tctest.RequireNoTag(t, entries, tt.wantNoTags...)

			var hides []string
			for _, entry := range tctest.All(entries, "B01_PARENT_NXDOMAIN_HIDES_DELEGATION") {
				tctest.RequireArg(t, entry, "query_name", "mid.example")
				tctest.RequireArg(t, entry, "domain_child", "child.mid.example")
				hides = append(hides, entry.Args["ns"].(string)+"/"+entry.Args["address"].(string))
			}
			if !slices.Equal(hides, tt.wantHides) {
				t.Fatalf("hiding servers = %v, want %v", hides, tt.wantHides)
			}

			if tt.wantNoChildOf == "" {
				return
			}
			noChild := tctest.RequireTag(t, entries, "B01_NO_CHILD")
			tctest.RequireArg(t, noChild, "domain_child", "child.mid.example")
			tctest.RequireArg(t, noChild, "domain_super", tt.wantNoChildOf)
		})
	}
}

// An ancestor denies a name above the parent's delegation and still refers to the parent.
func TestBasic01AncestorNXDomainHidesParentDelegation(t *testing.T) {
	tests := []struct {
		name            string
		zone            string
		parentDenies    string
		wantQueryNames  []string
		wantNS          []string
		wantAncestorLog []string
		wantParentLog   []string
	}{
		{
			name:           "ancestor denies",
			zone:           "child.parent.mid.example",
			wantQueryNames: []string{"mid.example"},
			wantNS:         []string{"ns1.example"},
			wantAncestorLog: []string{
				"example SOA", "example NS", "mid.example SOA", "child.parent.mid.example SOA",
			},
			wantParentLog: []string{
				"parent.mid.example SOA", "parent.mid.example NS", "child.parent.mid.example SOA",
			},
		},
		{
			name:           "ancestor and parent deny",
			zone:           "child.im.parent.mid.example",
			parentDenies:   "im.parent.mid.example",
			wantQueryNames: []string{"mid.example", "im.parent.mid.example"},
			wantNS:         []string{"ns1.example", "ns1.parent.mid.example"},
			wantAncestorLog: []string{
				"example SOA", "example NS", "mid.example SOA", "child.im.parent.mid.example SOA",
			},
			wantParentLog: []string{
				"parent.mid.example SOA", "parent.mid.example NS", "im.parent.mid.example SOA", "child.im.parent.mid.example SOA",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)

			var mu sync.Mutex
			logs := map[string][]string{}
			// recording logs the SOA and NS queries a server receives.
			recording := func(server string, next tctest.Handler) tctest.Handler {
				return func(q tctest.Query) packet.Packet {
					if key := queryKey(q); strings.HasSuffix(key, " SOA") || strings.HasSuffix(key, " NS") {
						mu.Lock()
						logs[server] = append(logs[server], key)
						mu.Unlock()
					}
					return next(q)
				}
			}

			r := serveRoots(t, ctx, func(string) tctest.Handler {
				return recording("root", replies{"example SOA": referral("example", ns1Example)}.handler())
			}, "a.root")
			tctest.NSOn(t, ctx, r, "ns1.example", "192.0.2.53", recording("ancestor", apex("example", replies{
				"mid.example SOA": nxdomainAAPacket,
				tt.zone + " SOA":  referral("parent.mid.example", nsEntry{name: "ns1.parent.mid.example", addr: net.IPv4(192, 0, 2, 54)}),
			}.handler(), "ns1.example")))
			parent := replies{
				tt.zone + " SOA": referral(tt.zone, nsEntry{name: "ns.child.example", addr: net.IPv4(192, 0, 2, 55)}),
			}
			if tt.parentDenies != "" {
				parent[tt.parentDenies+" SOA"] = nxdomainAAPacket
			}
			tctest.NSOn(t, ctx, r, "ns1.parent.mid.example", "192.0.2.54", recording("parent", apex("parent.mid.example", parent.handler(), "ns1.parent.mid.example")))

			entries := basic01Entries(t, ctx, tt.zone, r)

			tctest.RequireTags(t, entries, "B01_CHILD_FOUND")
			tctest.RequireNoTag(t, entries, "B01_NO_CHILD", "B01_INCONSISTENT_DELEGATION", "B01_PARENT_UNDETERMINED", "B01_SERVER_ZONE_ERROR")
			tctest.RequireArg(t, tctest.RequireTag(t, entries, "B01_PARENT_FOUND"), "domain", "parent.mid.example")

			if got := tctest.ArgValues(entries, "B01_PARENT_NXDOMAIN_HIDES_DELEGATION", "query_name"); !slices.Equal(got, tt.wantQueryNames) {
				t.Errorf("query_name = %v, want %v", got, tt.wantQueryNames)
			}
			if got := tctest.ArgValues(entries, "B01_PARENT_NXDOMAIN_HIDES_DELEGATION", "ns"); !slices.Equal(got, tt.wantNS) {
				t.Errorf("ns = %v, want %v", got, tt.wantNS)
			}
			// The root SOA and NS queries are answered below the recorder.
			if want := []string{"example SOA"}; !slices.Equal(logs["root"], want) {
				t.Errorf("root queries = %v, want %v", logs["root"], want)
			}
			if !slices.Equal(logs["ancestor"], tt.wantAncestorLog) {
				t.Errorf("ancestor queries = %v, want %v", logs["ancestor"], tt.wantAncestorLog)
			}
			if !slices.Equal(logs["parent"], tt.wantParentLog) {
				t.Errorf("parent queries = %v, want %v", logs["parent"], tt.wantParentLog)
			}
		})
	}
}

// Replays a trimmed `--save` capture of 0.d.b.9.1.b.9.0.1.0.0.2.ip6.arpa, IPv4 only, with the network off.
func TestBasic01ParentNXDomainHidesDelegationFromRecordedCache(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.NoNetwork = true
	prof.Net.IPv6 = false

	nsCache := nameserver.NewCacheStore()
	rec, err := recursor.New()
	if err != nil {
		t.Fatalf("recursor.New: %v", err)
	}
	var file cachefile.File
	if err := json.Unmarshal(bahnhofNXDomainContradictionCache, &file); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if err := cachefile.Import(file, nsCache, rec, asnlookup.NewCache()); err != nil {
		t.Fatalf("import fixture: %v", err)
	}
	ctx = nameserver.WithCache(ctx, nsCache)

	entries := basic01Entries(t, ctx, "0.d.b.9.1.b.9.0.1.0.0.2.ip6.arpa", rec)

	tctest.RequireTags(t, entries, "B01_PARENT_FOUND", "B01_CHILD_FOUND")
	tctest.RequireNoTag(t, entries, "B01_NO_CHILD")
	entry := tctest.RequireTag(t, entries, "B01_PARENT_NXDOMAIN_HIDES_DELEGATION")
	tctest.RequireArg(t, entry, "domain_child", "0.d.b.9.1.b.9.0.1.0.0.2.ip6.arpa")
	tctest.RequireArg(t, entry, "query_name", "b.9.1.b.9.0.1.0.0.2.ip6.arpa")
	nsArg, _ := entry.Args["ns"].(string)
	if !strings.HasSuffix(strings.ToLower(nsArg), ".bahnhof.net") {
		t.Fatalf("expected ns endpoint at bahnhof.net, got %#v", entry.Args["ns"])
	}
}

// Each root answers the DNAME query for example with its own target.
func TestBasic01ChildAlias(t *testing.T) {
	tests := []struct {
		name             string
		targets          map[string]string
		wantTargets      []string
		wantInconsistent int
	}{
		{
			name:        "one target",
			targets:     map[string]string{"a.root": "sister.example"},
			wantTargets: []string{"sister.example"},
		},
		{
			name:             "two targets",
			targets:          map[string]string{"a.root": "sister.example", "b.root": "brother.example"},
			wantTargets:      []string{"brother.example", "sister.example"},
			wantInconsistent: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)

			r := serveRoots(t, ctx, func(root string) tctest.Handler {
				return replies{
					"example SOA":   emptyAnswerPacket,
					"example DNAME": func() packet.Packet { return dnameAnswerPacket("example", tt.targets[root]) },
				}.handler()
			}, slices.Sorted(maps.Keys(tt.targets))...)

			entries := basic01Entries(t, ctx, "example", r)

			tctest.RequireTags(t, entries, "B01_PARENT_FOUND")
			tctest.RequireNoTag(t, entries, "B01_NO_CHILD", "B01_CHILD_FOUND")
			for _, entry := range tctest.All(entries, "B01_CHILD_IS_ALIAS") {
				tctest.RequireArg(t, entry, "domain_child", "example")
			}
			targets := tctest.ArgValues(entries, "B01_CHILD_IS_ALIAS", "domain_target")
			slices.Sort(targets)
			if !slices.Equal(targets, tt.wantTargets) {
				t.Fatalf("targets = %v, want %v", targets, tt.wantTargets)
			}
			tctest.RequireCount(t, entries, "B01_INCONSISTENT_ALIAS", tt.wantInconsistent)
			if tt.wantInconsistent == 1 {
				tctest.RequireArg(t, tctest.RequireTag(t, entries, "B01_INCONSISTENT_ALIAS"), "domain", "example")
			}
		})
	}
}

// A name published only as a DNAME in the parent ends the run without a CRITICAL entry.
func TestBasic01DNAMEAliasNoCritical(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := serveRoots(t, ctx, func(string) tctest.Handler {
		return replies{
			"example SOA":   emptyAnswerPacket,
			"example DNAME": func() packet.Packet { return dnameAnswerPacket("example", "target.example") },
		}.handler()
	}, "a.root")
	z := tctest.Zone(t, "example", r)

	entries, err := All(ctx, z)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	tctest.RequireTags(t, entries, "B01_CHILD_IS_ALIAS")
	tctest.RequireNoTag(t, entries, "B01_NO_CHILD", "B02_NO_DELEGATION")
	if !IsDNAMEAlias(entries) {
		t.Fatalf("IsDNAMEAlias = false, want true")
	}
	if CanContinue(ctx, z, entries) {
		t.Fatalf("CanContinue = true, want false")
	}
	for _, e := range entries {
		if e != nil && strings.ToUpper(e.Level()) == "CRITICAL" {
			t.Fatalf("unexpected CRITICAL entry: %s", e.Tag)
		}
	}
}
