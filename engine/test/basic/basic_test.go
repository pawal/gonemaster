package basic

import (
	"context"
	_ "embed"
	"encoding/json"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/asnlookup"
	"codeberg.org/pawal/gonemaster/engine/cachefile"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/recursor/recursortest"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

//go:embed testdata/bahnhof-nxdomain-contradiction.cache.json
var bahnhofNXDomainContradictionCache []byte

func TestBasic01Root(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	z := zone.Zone{Name: dnsname.New(".")}
	entries, err := Basic01(ctx, &z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected log entries")
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

	rootHook := func(owner string) tctest.RawHandler {
		return func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			name := strings.ToLower(qname)
			kind := strings.ToUpper(qtype)
			switch {
			case name == "." && kind == "SOA":
				return soaPacket(".", owner, "hostmaster.root"), nil
			case name == "." && kind == "NS":
				return nsPacketMulti(".", "b.root", "a.root"), nil
			case name == "example" && kind == "SOA":
				return referralPacketMulti("example", []nsEntry{
					{name: "ns1.example", addr: net.IPv4(192, 0, 2, 53)},
				}), nil
			default:
				return packet.Packet{}, nil
			}
		}
	}

	tctest.NSRaw(t, ctx, r, "a.root", "192.0.2.1", rootHook("a.root"))

	tctest.NSRaw(t, ctx, r, "b.root", "192.0.2.2", rootHook("b.root"))

	z := tctest.Zone(t, "example", r)

	entries, err := Basic01(ctx, z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "B01_PARENT_FOUND")
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("expected two typed servers for B01_PARENT_FOUND, got %#v", entry.Args["servers"])
	}
	if servers[0]["ns"] != "a.root" || servers[0]["address"] != "192.0.2.1" {
		t.Fatalf("unexpected first typed server payload: %#v", servers[0])
	}
	if servers[1]["ns"] != "b.root" || servers[1]["address"] != "192.0.2.2" {
		t.Fatalf("unexpected second typed server payload: %#v", servers[1])
	}
	addresses, ok := entry.Args["addresses"].([]string)
	if !ok || len(addresses) != 2 {
		t.Fatalf("expected two typed addresses for B01_PARENT_FOUND, got %#v", entry.Args["addresses"])
	}
	if addresses[0] != "192.0.2.1" || addresses[1] != "192.0.2.2" {
		t.Fatalf("expected deterministic address order, got %v", addresses)
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
}

// TestBasic01EmitsCNAMETagOnNSLookup exercises the cnamelog integration:
// when Basic01 walks the parent zone and tries to resolve an
// out-of-bailiwick NS hostname whose A/AAAA recursion produces a typed
// *recursor.CNAMEError, the matching CNAME_* tag must appear in the
// emitted entries instead of disappearing silently.
//
// Setup: pre-populate the recursor cache with the CNAMEError for
// "ns.outside.test" A and AAAA via recursortest.SeedCNAMEError. The root server's NS
// response for "." carries an extra NS "ns.outside.test" without glue,
// so Basic01 calls rec.Recurse(ctx, "ns.outside.test", ...) which hits
// the cache and surfaces the typed error to cnamelog.Log.
func TestBasic01EmitsCNAMETagOnNSLookup(t *testing.T) {
	cases := []struct {
		name       string
		seedErr    *recursor.CNAMEError
		wantTag    string
		wantArgs   map[string]any
		extraArgKs []string
	}{
		{
			name:    "unresolved/loop emits CNAME_TARGET_UNRESOLVED",
			seedErr: &recursor.CNAMEError{Reason: recursor.CNAMEUnresolved, Name: "ns.outside.test", Target: "loop.outside.test", Detail: "loop"},
			wantTag: "CNAME_TARGET_UNRESOLVED",
			wantArgs: map[string]any{
				"query_name":   "ns.outside.test",
				"cname_target": "loop.outside.test",
			},
		},
		{
			name:    "too-many emits CNAME_TOO_MANY_RECORDS",
			seedErr: &recursor.CNAMEError{Reason: recursor.CNAMETooMany, Name: "ns.outside.test"},
			wantTag: "CNAME_TOO_MANY_RECORDS",
			wantArgs: map[string]any{
				"query_name": "ns.outside.test",
			},
		},
		{
			name:    "chain-too-long emits CNAME_CHAIN_TOO_LONG",
			seedErr: &recursor.CNAMEError{Reason: recursor.CNAMEChainTooLong, Name: "ns.outside.test", Target: "deep.outside.test"},
			wantTag: "CNAME_CHAIN_TOO_LONG",
			wantArgs: map[string]any{
				"query_name": "ns.outside.test",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)

			r := tctest.Recursor(t, map[string]map[string][]string{
				".": {
					"a.root": {"192.0.2.1"},
				},
			})
			r.SetNegativeCacheTTL(60 * time.Second)
			recursortest.SeedCNAMEError(r, tc.seedErr, "ns.outside.test", []string{"A", "AAAA"})

			rootHook := func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
				name := strings.ToLower(qname)
				kind := strings.ToUpper(qtype)
				switch {
				case name == "." && kind == "SOA":
					return soaPacket(".", "a.root", "hostmaster.root"), nil
				case name == "." && kind == "NS":
					return nsPacketMulti(".", "a.root", "ns.outside.test"), nil
				}
				return packet.Packet{}, nil
			}

			tctest.NSRaw(t, ctx, r, "a.root", "192.0.2.1", rootHook)

			z := tctest.Zone(t, "example", r)

			entries, err := Basic01(ctx, z)
			if err != nil {
				t.Fatalf("basic01: %v", err)
			}
			entry := tctest.RequireTag(t, entries, tc.wantTag)
			for k, want := range tc.wantArgs {
				if got := entry.Args[k]; got != want {
					t.Fatalf("%s arg %q: got %#v, want %#v", tc.wantTag, k, got, want)
				}
			}
		})
	}
}

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
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one typed server for B02_AUTH_RESPONSE_SOA, got %#v", entry.Args["servers"])
	}
	if servers[0]["ns"] != "a.root" || servers[0]["address"] != "192.0.2.1" {
		t.Fatalf("unexpected typed server payload: %#v", servers[0])
	}
	addresses, ok := entry.Args["addresses"].([]string)
	if !ok || len(addresses) != 1 || addresses[0] != "192.0.2.1" {
		t.Fatalf("unexpected typed addresses payload: %#v", entry.Args["addresses"])
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("legacy key ns_list should not be present: %#v", entry.Args)
	}
}

func TestBasic02ParallelQueries(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	prof.Resolver.Defaults.Parallel = 2

	r := tctest.Recursor(t, map[string]map[string][]string{
		".": {
			"a.root": {"192.0.2.1"},
			"b.root": {"192.0.2.2"},
		},
	})

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string, block bool) tctest.RawHandler {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			name := strings.ToLower(qname)
			kind := strings.ToUpper(qtype)
			switch {
			case name == "." && kind == "NS":
				return nsPacketMulti(".", "a.root", "b.root"), nil
			case name == "." && kind == "SOA":
				select {
				case started <- id:
				default:
				}
				if block {
					select {
					case <-release:
					case <-ctx.Done():
						return packet.Packet{}, ctx.Err()
					}
				}
				return soaPacket(".", id, "hostmaster.root"), nil
			default:
				return packet.Packet{}, nil
			}
		}
	}

	tctest.NSRaw(t, baseCtx, r, "a.root", "192.0.2.1", hook("a.root", true))

	tctest.NSRaw(t, baseCtx, r, "b.root", "192.0.2.2", hook("b.root", false))

	z := tctest.Zone(t, ".", r)

	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var basicErr error
	go func() {
		entries, basicErr = Basic02(ctx, z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case id := <-started:
			got[id] = true
		case <-deadline:
			t.Fatalf("expected parallel SOA queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if basicErr != nil {
			t.Fatalf("basic02: %v", basicErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("basic02 did not finish")
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
	servers, ok := entry.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 2 {
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
}

func TestBasic02UnexpectedRcode(t *testing.T) {
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
			return rcodePacket(dns.RcodeServerFailure)
		default:
			return packet.Packet{}
		}
	})

	z := tctest.Zone(t, ".", r)

	entries, err := Basic02(ctx, z)
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}
	tctest.RequireTags(t, entries, "B02_NO_WORKING_NS", "B02_UNEXPECTED_RCODE")
}

func TestBasic02NoIPAddress(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := tctest.Recursor(t, map[string]map[string][]string{
		".": {
			"a.root": {"192.0.2.1"},
		},
	})

	tctest.NSOn(t, ctx, r, "a.root", "192.0.2.1", func(q tctest.Query) packet.Packet {
		name := strings.ToLower(q.Name)
		kind := strings.ToUpper(q.Type)
		if name == "." && kind == "NS" {
			return nsPacket(".", "b.root")
		}
		return packet.Packet{}
	})

	z := tctest.Zone(t, ".", r)

	entries, err := Basic02(ctx, z)
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "B02_NS_NO_IP_ADDR")
	if ns, _ := entry.Args["ns"].(string); ns != "b.root" {
		t.Fatalf("expected ns=b.root in B02_NS_NO_IP_ADDR, got %#v", entry.Args["ns"])
	}
	if _, ok := entry.Args["nsname"]; ok {
		t.Fatalf("did not expect legacy nsname key in args")
	}
}

func TestBasic03HasARecords(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

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
			return aPacket("www.example", net.IPv4(192, 0, 2, 99))
		default:
			return packet.Packet{}
		}
	})

	z := tctest.Zone(t, "example", r)

	entries, err := Basic03(ctx, z)
	if err != nil {
		t.Fatalf("basic03: %v", err)
	}
	tctest.RequireTags(t, entries, "HAS_A_RECORDS")
	tctest.RequireNoTag(t, entries, "A_QUERY_NO_RESPONSES")
}

func TestBasic03NoARecords(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

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
			return emptyAnswerPacket()
		default:
			return packet.Packet{}
		}
	})

	z := tctest.Zone(t, "example", r)

	entries, err := Basic03(ctx, z)
	if err != nil {
		t.Fatalf("basic03: %v", err)
	}
	tctest.RequireTags(t, entries, "NO_A_RECORDS")
	tctest.RequireNoTag(t, entries, "HAS_A_RECORDS")
}

func TestBasic03NoResponses(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

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
		if name == "example" && kind == "SOA" {
			return soaPacket("example", "ns1.example", "hostmaster.example")
		}
		return packet.Packet{}
	})

	z := tctest.Zone(t, "example", r)

	entries, err := Basic03(ctx, z)
	if err != nil {
		t.Fatalf("basic03: %v", err)
	}
	tctest.RequireTags(t, entries, "A_QUERY_NO_RESPONSES")
}

func TestBasic03ParallelQueries(t *testing.T) {
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

	tctest.NSRaw(t, baseCtx, r, "a.root", "192.0.2.1", func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && (kind == "SOA" || kind == "NS"):
			return referralPacketMulti("example", []nsEntry{
				{name: "ns1.example", addr: net.IPv4(192, 0, 2, 53)},
				{name: "ns2.example", addr: net.IPv4(192, 0, 2, 54)},
			}), nil
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), nil
		default:
			return packet.Packet{}, nil
		}
	})

	started := make(chan string, 2)
	release := make(chan struct{})
	nsHook := func(id string, addr net.IP, block bool) tctest.RawHandler {
		return func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			name := strings.ToLower(qname)
			kind := strings.ToUpper(qtype)
			if name == "www.example" && kind == "A" {
				select {
				case started <- id:
				default:
				}
				if block {
					select {
					case <-release:
					case <-ctx.Done():
						return packet.Packet{}, ctx.Err()
					}
				}
				return aPacket("www.example", addr), nil
			}
			if name == "example" && kind == "SOA" {
				return soaPacket("example", "ns1.example", "hostmaster.example"), nil
			}
			return packet.Packet{}, nil
		}
	}

	tctest.NSRaw(t, baseCtx, r, "ns1.example", "192.0.2.53", nsHook("ns1.example", net.IPv4(192, 0, 2, 53), true))

	tctest.NSRaw(t, baseCtx, r, "ns2.example", "192.0.2.54", nsHook("ns2.example", net.IPv4(192, 0, 2, 54), false))

	z := tctest.Zone(t, "example", r)

	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var basicErr error
	go func() {
		entries, basicErr = Basic03(ctx, z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case id := <-started:
			got[id] = true
		case <-deadline:
			t.Fatalf("expected parallel A queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if basicErr != nil {
			t.Fatalf("basic03: %v", basicErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("basic03 did not finish")
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

		tctest.NSRaw(t, ctx, r, "a.root", "192.0.2.1", func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			name := strings.ToLower(qname)
			kind := strings.ToUpper(qtype)
			switch {
			case name == "example" && (kind == "SOA" || kind == "NS"):
				return referralPacketMulti("example", []nsEntry{
					{name: "ns1.example", addr: net.IPv4(192, 0, 2, 53)},
					{name: "ns2.example", addr: net.IPv4(192, 0, 2, 54)},
				}), nil
			case name == "." && kind == "SOA":
				return soaPacket(".", "a.root", "hostmaster.root"), nil
			default:
				return packet.Packet{}, nil
			}
		})

		nsHook := func(owner string, addr net.IP) tctest.RawHandler {
			return func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
				name := strings.ToLower(qname)
				kind := strings.ToUpper(qtype)
				switch {
				case name == "example" && kind == "SOA":
					return soaPacket("example", owner, "hostmaster.example"), nil
				case name == "www.example" && kind == "A":
					return aPacket("www.example", addr), nil
				default:
					return packet.Packet{}, nil
				}
			}
		}

		tctest.NSRaw(t, ctx, r, "ns1.example", "192.0.2.53", nsHook("ns1.example", net.IPv4(192, 0, 2, 53)))

		tctest.NSRaw(t, ctx, r, "ns2.example", "192.0.2.54", nsHook("ns2.example", net.IPv4(192, 0, 2, 54)))

		z := tctest.Zone(t, "example", r)

		entries, err := Basic03(ctx, z)
		if err != nil {
			t.Fatalf("basic03: %v", err)
		}
		tctest.RequireTags(t, entries, "HAS_A_RECORDS")
		return entries
	}

	sequentialEntries := runBasic03(1)
	parallelEntries := runBasic03(2)

	sequentialNormalized := tctest.Normalize(sequentialEntries)
	parallelNormalized := tctest.Normalize(parallelEntries)

	if len(sequentialNormalized) != len(parallelNormalized) {
		t.Fatalf("entry count changed with parallelism: sequential=%v parallel=%v", sequentialNormalized, parallelNormalized)
	}
	for i := range sequentialNormalized {
		if sequentialNormalized[i] != parallelNormalized[i] {
			t.Fatalf("entry[%d] changed with parallelism: %q != %q", i, sequentialNormalized[i], parallelNormalized[i])
		}
	}
}

func TestBasic01NoChild(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := tctest.Recursor(t, map[string]map[string][]string{
		".": {
			"a.root": {"192.0.2.1"},
			"b.root": {"192.0.2.2"},
		},
	})

	rootHook := func(owner string) tctest.Handler {
		return func(q tctest.Query) packet.Packet {
			name := strings.ToLower(q.Name)
			kind := strings.ToUpper(q.Type)
			switch {
			case name == "." && kind == "SOA":
				return soaPacket(".", owner, "hostmaster.root")
			case name == "." && kind == "NS":
				return nsPacketMulti(".", "a.root", "b.root")
			case name == "example" && kind == "SOA":
				return nxdomainAAPacket()
			default:
				return packet.Packet{}
			}
		}
	}

	tctest.NSOn(t, ctx, r, "a.root", "192.0.2.1", rootHook("a.root"))

	tctest.NSOn(t, ctx, r, "b.root", "192.0.2.2", rootHook("b.root"))

	z := tctest.Zone(t, "example", r)

	entries, err := Basic01(ctx, z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}

	tctest.RequireTags(t, entries, "B01_PARENT_FOUND", "B01_NO_CHILD")
	tctest.RequireNoTag(t, entries, "B01_CHILD_FOUND", "B01_INCONSISTENT_DELEGATION")

	noChild := tctest.RequireTag(t, entries, "B01_NO_CHILD")
	if noChild.Args["domain_child"] != "example" {
		t.Fatalf("expected domain_child=example, got %#v", noChild.Args["domain_child"])
	}
	if noChild.Args["domain_super"] != "." {
		t.Fatalf("expected domain_super=., got %#v", noChild.Args["domain_super"])
	}
}

func TestBasic01InconsistentDelegation(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := tctest.Recursor(t, map[string]map[string][]string{
		".": {
			"a.root": {"192.0.2.1"},
			"b.root": {"192.0.2.2"},
		},
	})

	commonRoot := func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, bool) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), true
		case name == "." && kind == "NS":
			return nsPacketMulti(".", "a.root", "b.root"), true
		}
		return packet.Packet{}, false
	}

	tctest.NSRaw(t, ctx, r, "a.root", "192.0.2.1", func(ctx context.Context, qname string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		if pkt, ok := commonRoot(ctx, qname, qtype, qclass, opts); ok {
			return pkt, nil
		}
		if strings.EqualFold(qname, "example") && strings.EqualFold(qtype, "SOA") {
			return referralPacketMulti("example", []nsEntry{
				{name: "ns1.example", addr: net.IPv4(192, 0, 2, 53)},
			}), nil
		}
		return packet.Packet{}, nil
	})

	tctest.NSRaw(t, ctx, r, "b.root", "192.0.2.2", func(ctx context.Context, qname string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		if pkt, ok := commonRoot(ctx, qname, qtype, qclass, opts); ok {
			return pkt, nil
		}
		if strings.EqualFold(qname, "example") && strings.EqualFold(qtype, "SOA") {
			return emptyAnswerPacket(), nil
		}
		if strings.EqualFold(qname, "example") && strings.EqualFold(qtype, "DNAME") {
			return emptyAnswerPacket(), nil
		}
		return packet.Packet{}, nil
	})

	z := tctest.Zone(t, "example", r)

	entries, err := Basic01(ctx, z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}

	tctest.RequireTags(t, entries, "B01_PARENT_FOUND", "B01_CHILD_FOUND", "B01_INCONSISTENT_DELEGATION")
	tctest.RequireNoTag(t, entries, "B01_NO_CHILD")

	inconsistent := tctest.RequireTag(t, entries, "B01_INCONSISTENT_DELEGATION")
	if inconsistent.Args["domain_child"] != "example" {
		t.Fatalf("expected domain_child=example, got %#v", inconsistent.Args["domain_child"])
	}
	if inconsistent.Args["domain_parent"] != "." {
		t.Fatalf("expected domain_parent=., got %#v", inconsistent.Args["domain_parent"])
	}
	servers, ok := inconsistent.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one typed server for B01_INCONSISTENT_DELEGATION, got %#v", inconsistent.Args["servers"])
	}
	if servers[0]["ns"] != "b.root" || servers[0]["address"] != "192.0.2.2" {
		t.Fatalf("expected only b.root in B01_INCONSISTENT_DELEGATION, got %#v", servers[0])
	}
}

// TestBasic01ParentNXDomainHidesDelegation models a parent NS that returns
// NXDOMAIN+AA at an intermediate empty non-terminal but a proper referral at
// the child name (an RFC 8020 violation). Basic01 must emit
// B01_PARENT_NXDOMAIN_HIDES_DELEGATION and still recognize the child as
// delegated (B01_CHILD_FOUND) so downstream test cases run.
func TestBasic01ParentNXDomainHidesDelegation(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := tctest.Recursor(t, map[string]map[string][]string{
		".": {
			"a.root": {"192.0.2.1"},
		},
	})

	tctest.NSRaw(t, ctx, r, "a.root", "192.0.2.1", func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), nil
		case name == "." && kind == "NS":
			return nsPacketMulti(".", "a.root"), nil
		case name == "example" && kind == "SOA":
			return referralPacketMulti("example", []nsEntry{
				{name: "ns1.example", addr: net.IPv4(192, 0, 2, 53)},
			}), nil
		}
		return packet.Packet{}, nil
	})

	tctest.NSRaw(t, ctx, r, "ns1.example", "192.0.2.53", func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && kind == "SOA":
			return soaPacket("example", "ns1.example", "hostmaster.example"), nil
		case name == "example" && kind == "NS":
			return nsPacketMulti("example", "ns1.example"), nil
		case name == "mid.example" && kind == "SOA":
			return nxdomainAAPacket(), nil
		case name == "child.mid.example" && kind == "SOA":
			return referralPacketMulti("child.mid.example", []nsEntry{
				{name: "ns.child.example", addr: net.IPv4(192, 0, 2, 54)},
			}), nil
		}
		return packet.Packet{}, nil
	})

	z := tctest.Zone(t, "child.mid.example", r)

	entries, err := Basic01(ctx, z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}

	tctest.RequireTags(t, entries, "B01_PARENT_FOUND", "B01_CHILD_FOUND", "B01_PARENT_NXDOMAIN_HIDES_DELEGATION")
	tctest.RequireNoTag(t, entries, "B01_NO_CHILD", "B01_INCONSISTENT_DELEGATION")

	entry := tctest.RequireTag(t, entries, "B01_PARENT_NXDOMAIN_HIDES_DELEGATION")
	if entry.Args["ns"] != "ns1.example" {
		t.Fatalf("expected ns=ns1.example, got %#v", entry.Args["ns"])
	}
	if entry.Args["address"] != "192.0.2.53" {
		t.Fatalf("expected address=192.0.2.53, got %#v", entry.Args["address"])
	}
	if entry.Args["query_name"] != "mid.example" {
		t.Fatalf("expected query_name=mid.example, got %#v", entry.Args["query_name"])
	}
	if entry.Args["domain_child"] != "child.mid.example" {
		t.Fatalf("expected domain_child=child.mid.example, got %#v", entry.Args["domain_child"])
	}
}

// TestBasic01ParentNXDomainNoDelegation models a parent NS that returns
// NXDOMAIN+AA at every probed name including the child. The contradiction
// probe must NOT fire, and Basic01 must emit the plain B01_NO_CHILD.
func TestBasic01ParentNXDomainNoDelegation(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := tctest.Recursor(t, map[string]map[string][]string{
		".": {
			"a.root": {"192.0.2.1"},
		},
	})

	tctest.NSRaw(t, ctx, r, "a.root", "192.0.2.1", func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), nil
		case name == "." && kind == "NS":
			return nsPacketMulti(".", "a.root"), nil
		case name == "example" && kind == "SOA":
			return referralPacketMulti("example", []nsEntry{
				{name: "ns1.example", addr: net.IPv4(192, 0, 2, 53)},
			}), nil
		}
		return packet.Packet{}, nil
	})

	tctest.NSOn(t, ctx, r, "ns1.example", "192.0.2.53", func(q tctest.Query) packet.Packet {
		name := strings.ToLower(q.Name)
		kind := strings.ToUpper(q.Type)
		switch {
		case name == "example" && kind == "SOA":
			return soaPacket("example", "ns1.example", "hostmaster.example")
		case name == "example" && kind == "NS":
			return nsPacketMulti("example", "ns1.example")
		case (name == "mid.example" || name == "child.mid.example") && kind == "SOA":
			return nxdomainAAPacket()
		}
		return packet.Packet{}
	})

	z := tctest.Zone(t, "child.mid.example", r)

	entries, err := Basic01(ctx, z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}

	tctest.RequireTags(t, entries, "B01_NO_CHILD")
	tctest.RequireNoTag(t, entries, "B01_CHILD_FOUND", "B01_PARENT_NXDOMAIN_HIDES_DELEGATION")

	noChild := tctest.RequireTag(t, entries, "B01_NO_CHILD")
	if noChild.Args["domain_child"] != "child.mid.example" {
		t.Fatalf("expected domain_child=child.mid.example, got %#v", noChild.Args["domain_child"])
	}
	if noChild.Args["domain_super"] != "mid.example" {
		t.Fatalf("expected domain_super=mid.example, got %#v", noChild.Args["domain_super"])
	}
}

// TestBasic01MixedNXDomainContradiction models two parent nameservers where
// one returns a clean referral at the child and the other returns NXDOMAIN at
// an intermediate ENT plus a referral at the child. Both contribute to
// delegationFound, B01_PARENT_NXDOMAIN_HIDES_DELEGATION fires only for the
// broken NS, and no B01_INCONSISTENT_DELEGATION is emitted because the
// broken NS is no longer in aaNXDomain.
func TestBasic01MixedNXDomainContradiction(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := tctest.Recursor(t, map[string]map[string][]string{
		".": {
			"a.root": {"192.0.2.1"},
		},
	})

	tctest.NSRaw(t, ctx, r, "a.root", "192.0.2.1", func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), nil
		case name == "." && kind == "NS":
			return nsPacketMulti(".", "a.root"), nil
		case name == "example" && kind == "SOA":
			return referralPacketMulti("example", []nsEntry{
				{name: "ns1.example", addr: net.IPv4(192, 0, 2, 53)},
				{name: "ns2.example", addr: net.IPv4(192, 0, 2, 54)},
			}), nil
		}
		return packet.Packet{}, nil
	})

	// ns1.example: well-behaved, returns NODATA at mid.example and a
	// referral at child.mid.example.
	tctest.NSRaw(t, ctx, r, "ns1.example", "192.0.2.53", func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && kind == "SOA":
			return soaPacket("example", "ns1.example", "hostmaster.example"), nil
		case name == "example" && kind == "NS":
			return nsPacketMulti("example", "ns1.example", "ns2.example"), nil
		case name == "mid.example" && kind == "SOA":
			return emptyAnswerPacket(), nil
		case name == "child.mid.example" && kind == "SOA":
			return referralPacketMulti("child.mid.example", []nsEntry{
				{name: "ns.child.example", addr: net.IPv4(192, 0, 2, 55)},
			}), nil
		}
		return packet.Packet{}, nil
	})

	// ns2.example: NXDOMAIN at mid.example, referral at child.mid.example
	// (the RFC 8020 contradiction).
	tctest.NSRaw(t, ctx, r, "ns2.example", "192.0.2.54", func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && kind == "SOA":
			return soaPacket("example", "ns1.example", "hostmaster.example"), nil
		case name == "example" && kind == "NS":
			return nsPacketMulti("example", "ns1.example", "ns2.example"), nil
		case name == "mid.example" && kind == "SOA":
			return nxdomainAAPacket(), nil
		case name == "child.mid.example" && kind == "SOA":
			return referralPacketMulti("child.mid.example", []nsEntry{
				{name: "ns.child.example", addr: net.IPv4(192, 0, 2, 55)},
			}), nil
		}
		return packet.Packet{}, nil
	})

	z := tctest.Zone(t, "child.mid.example", r)

	entries, err := Basic01(ctx, z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}

	tctest.RequireTags(t, entries, "B01_CHILD_FOUND", "B01_PARENT_NXDOMAIN_HIDES_DELEGATION")
	tctest.RequireNoTag(t, entries, "B01_INCONSISTENT_DELEGATION", "B01_NO_CHILD")

	count := 0
	for _, entry := range entries {
		if entry != nil && entry.Tag == "B01_PARENT_NXDOMAIN_HIDES_DELEGATION" {
			count++
			if entry.Args["ns"] != "ns2.example" {
				t.Fatalf("expected ns=ns2.example, got %#v", entry.Args["ns"])
			}
			if entry.Args["address"] != "192.0.2.54" {
				t.Fatalf("expected address=192.0.2.54, got %#v", entry.Args["address"])
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one B01_PARENT_NXDOMAIN_HIDES_DELEGATION entry, got %d", count)
	}
}

// TestBasic01ParentNXDomainHidesDelegationFromRecordedCache replays a real
// recorded DNS trace against the live zone 0.d.b.9.1.b.9.0.1.0.0.2.ip6.arpa
// (whose parent at Bahnhof returns authoritative NXDOMAIN for the
// intermediate ENT "b.9.1.b.9.0.1.0.0.2.ip6.arpa" while still delegating the
// child to flashdance.cx). The fixture was captured with
// `gonemaster --testcase basic01 --save bahnhof.cache 0.d.b.9.1.b.9.0.1.0.0.2.ip6.arpa`
// then trimmed (recursor entries and IPv6 nameserver endpoints dropped) and
// is replayed offline (no_network=true, IPv6 disabled in the profile) so
// the test never touches the network.
func TestBasic01ParentNXDomainHidesDelegationFromRecordedCache(t *testing.T) {

	ctx, prof, _ := testhelpers.Context(t)
	prof.NoNetwork = true
	// Fixture only captures IPv4 endpoints to keep the file small. Force
	// the profile to match so cache misses on IPv6 endpoints don't matter.
	prof.Net.IPv6 = false

	nsCache := nameserver.NewCacheStore()
	rec, err := recursor.New()
	if err != nil {
		t.Fatalf("recursor.New: %v", err)
	}
	asnCache := asnlookup.NewCache()

	var file cachefile.File
	if err := json.Unmarshal(bahnhofNXDomainContradictionCache, &file); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if err := cachefile.Import(file, nsCache, rec, asnCache); err != nil {
		t.Fatalf("import fixture: %v", err)
	}

	ctx = nameserver.WithCache(ctx, nsCache)

	z := tctest.Zone(t, "0.d.b.9.1.b.9.0.1.0.0.2.ip6.arpa", rec)

	entries, err := Basic01(ctx, z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}

	tctest.RequireTags(t, entries, "B01_PARENT_FOUND", "B01_PARENT_NXDOMAIN_HIDES_DELEGATION", "B01_CHILD_FOUND")
	tctest.RequireNoTag(t, entries, "B01_NO_CHILD")

	entry := tctest.RequireTag(t, entries, "B01_PARENT_NXDOMAIN_HIDES_DELEGATION")
	if entry.Args["domain_child"] != "0.d.b.9.1.b.9.0.1.0.0.2.ip6.arpa" {
		t.Fatalf("expected domain_child=0.d.b.9.1.b.9.0.1.0.0.2.ip6.arpa, got %#v", entry.Args["domain_child"])
	}
	if entry.Args["query_name"] != "b.9.1.b.9.0.1.0.0.2.ip6.arpa" {
		t.Fatalf("expected query_name=b.9.1.b.9.0.1.0.0.2.ip6.arpa, got %#v", entry.Args["query_name"])
	}
	nsArg, _ := entry.Args["ns"].(string)
	if !strings.HasSuffix(strings.ToLower(nsArg), "bahnhof.net") {
		t.Fatalf("expected ns endpoint at bahnhof.net, got %#v", entry.Args["ns"])
	}
}

func TestBasic01ChildAlias(t *testing.T) {
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
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root")
		case name == "." && kind == "NS":
			return nsPacket(".", "a.root")
		case name == "example" && kind == "SOA":
			return emptyAnswerPacket()
		case name == "example" && kind == "DNAME":
			return dnameAnswerPacket("example", "sister.example")
		}
		return packet.Packet{}
	})

	z := tctest.Zone(t, "example", r)

	entries, err := Basic01(ctx, z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}

	tctest.RequireTags(t, entries, "B01_PARENT_FOUND")
	tctest.RequireNoTag(t, entries, "B01_NO_CHILD")
	tctest.RequireTags(t, entries, "B01_CHILD_IS_ALIAS")
	tctest.RequireNoTag(t, entries, "B01_CHILD_FOUND", "B01_INCONSISTENT_ALIAS")

	alias := tctest.RequireTag(t, entries, "B01_CHILD_IS_ALIAS")
	if alias.Args["domain_child"] != "example" {
		t.Fatalf("expected domain_child=example, got %#v", alias.Args["domain_child"])
	}
	if alias.Args["domain_target"] != "sister.example" {
		t.Fatalf("expected domain_target=sister.example, got %#v", alias.Args["domain_target"])
	}
}

func TestBasic01InconsistentAlias(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := tctest.Recursor(t, map[string]map[string][]string{
		".": {
			"a.root": {"192.0.2.1"},
			"b.root": {"192.0.2.2"},
		},
	})

	rootHook := func(target string) tctest.Handler {
		return func(q tctest.Query) packet.Packet {
			name := strings.ToLower(q.Name)
			kind := strings.ToUpper(q.Type)
			switch {
			case name == "." && kind == "SOA":
				return soaPacket(".", "a.root", "hostmaster.root")
			case name == "." && kind == "NS":
				return nsPacketMulti(".", "a.root", "b.root")
			case name == "example" && kind == "SOA":
				return emptyAnswerPacket()
			case name == "example" && kind == "DNAME":
				return dnameAnswerPacket("example", target)
			}
			return packet.Packet{}
		}
	}

	tctest.NSOn(t, ctx, r, "a.root", "192.0.2.1", rootHook("sister.example"))

	tctest.NSOn(t, ctx, r, "b.root", "192.0.2.2", rootHook("brother.example"))

	z := tctest.Zone(t, "example", r)

	entries, err := Basic01(ctx, z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}

	tctest.RequireTags(t, entries, "B01_PARENT_FOUND")
	tctest.RequireNoTag(t, entries, "B01_NO_CHILD")
	tctest.RequireTags(t, entries, "B01_CHILD_IS_ALIAS", "B01_INCONSISTENT_ALIAS")

	targets := map[string]bool{}
	for _, entry := range entries {
		if entry == nil || entry.Tag != "B01_CHILD_IS_ALIAS" {
			continue
		}
		if target, ok := entry.Args["domain_target"].(string); ok {
			targets[target] = true
		}
	}
	if !targets["sister.example"] || !targets["brother.example"] {
		t.Fatalf("expected both sister.example and brother.example targets, got %v", targets)
	}

	inconsistentAlias := tctest.RequireTag(t, entries, "B01_INCONSISTENT_ALIAS")
	if inconsistentAlias.Args["domain"] != "example" {
		t.Fatalf("expected domain=example, got %#v", inconsistentAlias.Args["domain"])
	}
}

// TestBasic01DNAMEAliasNoCritical verifies that a domain published only as a
// DNAME alias in the parent - with no NS delegation and no SOA of its own -
// does not produce a CRITICAL entry and does not emit B01_NO_CHILD or
// B02_NO_DELEGATION. The regression target is xn--mori-qsa.nz (maori.nz),
// which is a DNAME alias with a valid DNAME target but no zone of its own.
func TestBasic01DNAMEAliasNoCritical(t *testing.T) {
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
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root")
		case name == "." && kind == "NS":
			return nsPacket(".", "a.root")
		case name == "example" && kind == "SOA":
			return emptyAnswerPacket()
		case name == "example" && kind == "DNAME":
			return dnameAnswerPacket("example", "target.example")
		}
		return packet.Packet{}
	})

	z := tctest.Zone(t, "example", r)

	entries, err := All(ctx, z)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	tctest.RequireTags(t, entries, "B01_CHILD_IS_ALIAS")
	tctest.RequireNoTag(t, entries, "B01_NO_CHILD", "B02_NO_DELEGATION")
	if !IsDNAMEAlias(entries) {
		t.Fatalf("expected IsDNAMEAlias to return true")
	}
	if CanContinue(ctx, z, entries) {
		t.Fatalf("expected CanContinue to return false for DNAME alias (no further tests needed)")
	}
	for _, e := range entries {
		if e != nil && strings.ToUpper(e.Level()) == "CRITICAL" {
			t.Fatalf("unexpected CRITICAL entry: %s", e.Tag)
		}
	}
}

func soaPacket(owner string, mname string, rname string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	soaRR := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	soaRR.Ns = dnsutil.Fqdn(mname)
	soaRR.Mbox = dnsutil.Fqdn(rname)
	soaRR.Serial = 1
	soaRR.Refresh = 3600
	soaRR.Retry = 600
	soaRR.Expire = 86400
	soaRR.Minttl = 60
	msg.Answer = []dns.RR{soaRR}
	return packet.Packet{Msg: msg}
}

func referralPacket(zoneName string, nsName string, nsAddr net.IP) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	nsRR := &dns.NS{}
	nsRR.Hdr = dns.Header{Name: dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 60}
	nsRR.Ns = dnsutil.Fqdn(nsName)
	msg.Ns = []dns.RR{nsRR}
	aRR := &dns.A{}
	aRR.Hdr = dns.Header{Name: dnsutil.Fqdn(nsName), Class: dns.ClassINET, TTL: 60}
	aRR.Addr = netip.AddrFrom4([4]byte(nsAddr.To4()))
	msg.Extra = []dns.RR{aRR}
	return packet.Packet{Msg: msg}
}

type nsEntry struct {
	name string
	addr net.IP
}

func referralPacketMulti(zoneName string, entries []nsEntry) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	for _, entry := range entries {
		nsRR := &dns.NS{}
		nsRR.Hdr = dns.Header{Name: dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 60}
		nsRR.Ns = dnsutil.Fqdn(entry.name)
		msg.Ns = append(msg.Ns, nsRR)
		aRR := &dns.A{}
		aRR.Hdr = dns.Header{Name: dnsutil.Fqdn(entry.name), Class: dns.ClassINET, TTL: 60}
		aRR.Addr = netip.AddrFrom4([4]byte(entry.addr.To4()))
		msg.Extra = append(msg.Extra, aRR)
	}
	return packet.Packet{Msg: msg}
}

func aPacket(owner string, addr net.IP) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	aRR := &dns.A{}
	aRR.Hdr = dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}
	aRR.Addr = netip.AddrFrom4([4]byte(addr.To4()))
	msg.Answer = []dns.RR{aRR}
	return packet.Packet{Msg: msg}
}

func nsPacket(owner string, nsname string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	nsRR := &dns.NS{}
	nsRR.Hdr = dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}
	nsRR.Ns = dnsutil.Fqdn(nsname)
	msg.Answer = []dns.RR{nsRR}
	return packet.Packet{Msg: msg}
}

func nsPacketMulti(owner string, nsnames ...string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	for _, nsname := range nsnames {
		nsRR := &dns.NS{}
		nsRR.Hdr = dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}
		nsRR.Ns = dnsutil.Fqdn(nsname)
		msg.Answer = append(msg.Answer, nsRR)
	}
	return packet.Packet{Msg: msg}
}

func rcodePacket(rcode uint16) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = rcode
	return packet.Packet{Msg: msg}
}

func emptyAnswerPacket() packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	return packet.Packet{Msg: msg}
}

func nxdomainAAPacket() packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeNameError
	msg.Authoritative = true
	return packet.Packet{Msg: msg}
}

func dnameAnswerPacket(owner string, target string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	dnameRR := &dns.DNAME{}
	dnameRR.Hdr = dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}
	dnameRR.Target = dnsutil.Fqdn(target)
	msg.Answer = []dns.RR{dnameRR}
	return packet.Packet{Msg: msg}
}
