package basic

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

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
	if !hasEntryTag(entries, "B01_CHILD_FOUND") {
		t.Fatalf("expected B01_CHILD_FOUND")
	}
	if !hasEntryTag(entries, "B01_ROOT_HAS_NO_PARENT") {
		t.Fatalf("expected B01_ROOT_HAS_NO_PARENT")
	}
}

func TestBasic01Undelegated(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic01(ctx, &z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}
	if !hasEntryTag(entries, "B01_CHILD_FOUND") {
		t.Fatalf("expected B01_CHILD_FOUND")
	}
	if !hasEntryTag(entries, "B01_PARENT_DISREGARDED") {
		t.Fatalf("expected B01_PARENT_DISREGARDED")
	}
}

func TestBasic01ParentFoundTypedArgs(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"b.root": {"192.0.2.2"},
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	rootHook := func(owner string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
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

	aroot, err := nameserver.NewWithContext(ctx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	aroot.SetQueryHook(rootHook("a.root"))

	broot, err := nameserver.NewWithContext(ctx, "b.root", "192.0.2.2", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	broot.SetQueryHook(rootHook("b.root"))

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic01(ctx, &z)
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}
	if !hasEntryTag(entries, "B01_PARENT_FOUND") {
		t.Fatalf("expected B01_PARENT_FOUND")
	}
	entry := firstEntryByTag(entries, "B01_PARENT_FOUND")
	if entry == nil {
		t.Fatalf("missing B01_PARENT_FOUND entry")
	}
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

func TestBasic02NoDelegation(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic02(ctx, &z)
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}
	if !hasEntryTag(entries, "B02_NO_DELEGATION") {
		t.Fatalf("expected B02_NO_DELEGATION")
	}
	if entries[0].Tag != "TEST_CASE_START" || entries[len(entries)-1].Tag != "TEST_CASE_END" {
		t.Fatalf("expected test case start/end markers")
	}
}

func TestBasic02AuthResponseSOA(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	root, err := nameserver.NewWithContext(ctx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "." && kind == "NS":
			return nsPacket(".", "a.root"), nil
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), nil
		default:
			return packet.Packet{}, nil
		}
	})

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic02(ctx, &z)
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}
	if !hasEntryTag(entries, "B02_AUTH_RESPONSE_SOA") {
		t.Fatalf("expected B02_AUTH_RESPONSE_SOA")
	}
	entry := firstEntryByTag(entries, "B02_AUTH_RESPONSE_SOA")
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
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	baseCtx, prof, _ := testhelpers.Context(t)
	prof.Resolver.Defaults.Parallel = 2

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
		"b.root": {"192.0.2.2"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string, block bool) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
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

	aroot, err := nameserver.NewWithContext(baseCtx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	aroot.SetQueryHook(hook("a.root", true))

	broot, err := nameserver.NewWithContext(baseCtx, "b.root", "192.0.2.2", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	broot.SetQueryHook(hook("b.root", false))

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var basicErr error
	go func() {
		entries, basicErr = Basic02(ctx, &z)
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
	for _, entry := range entries {
		if entry == nil || entry.Tag != "IPV4_ENABLED" {
			continue
		}
		if _, ok := entry.Args["arg_schema"]; ok {
			t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
		}
		if ns, ok := entry.Args["ns"].(string); ok {
			if strings.Contains(ns, "/") {
				t.Fatalf("expected nameserver-only ns argument, got %q", ns)
			}
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
	if !hasEntryTag(entries, "B02_AUTH_RESPONSE_SOA") {
		t.Fatalf("expected B02_AUTH_RESPONSE_SOA")
	}
	entry := firstEntryByTag(entries, "B02_AUTH_RESPONSE_SOA")
	if entry == nil {
		t.Fatalf("missing B02_AUTH_RESPONSE_SOA entry")
	}
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
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	root, err := nameserver.NewWithContext(ctx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "." && kind == "NS":
			return nsPacket(".", "a.root"), nil
		case name == "." && kind == "SOA":
			return rcodePacket(dns.RcodeServerFailure), nil
		default:
			return packet.Packet{}, nil
		}
	})

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic02(ctx, &z)
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}
	if !hasEntryTag(entries, "B02_NO_WORKING_NS") {
		t.Fatalf("expected B02_NO_WORKING_NS")
	}
	if !hasEntryTag(entries, "B02_UNEXPECTED_RCODE") {
		t.Fatalf("expected B02_UNEXPECTED_RCODE")
	}
}

func TestBasic02NoIPAddress(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	root, err := nameserver.NewWithContext(ctx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		if name == "." && kind == "NS" {
			return nsPacket(".", "b.root"), nil
		}
		return packet.Packet{}, nil
	})

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic02(ctx, &z)
	if err != nil {
		t.Fatalf("basic02: %v", err)
	}
	if !hasEntryTag(entries, "B02_NS_NO_IP_ADDR") {
		t.Fatalf("expected B02_NS_NO_IP_ADDR")
	}
	entry := firstEntryByTag(entries, "B02_NS_NO_IP_ADDR")
	if entry == nil {
		t.Fatalf("expected B02_NS_NO_IP_ADDR entry")
	}
	if ns, _ := entry.Args["ns"].(string); ns != "b.root" {
		t.Fatalf("expected ns=b.root in B02_NS_NO_IP_ADDR, got %#v", entry.Args["ns"])
	}
	if _, ok := entry.Args["nsname"]; ok {
		t.Fatalf("did not expect legacy nsname key in args")
	}
}

func TestBasic03HasARecords(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	root, err := nameserver.NewWithContext(ctx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && (kind == "SOA" || kind == "NS"):
			return referralPacket("example", "ns1.example", net.IPv4(192, 0, 2, 53)), nil
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), nil
		default:
			return packet.Packet{}, nil
		}
	})

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.53", r.Client())
	if err != nil {
		t.Fatalf("new ns1: %v", err)
	}
	ns1.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && kind == "SOA":
			return soaPacket("example", "ns1.example", "hostmaster.example"), nil
		case name == "www.example" && kind == "A":
			return aPacket("www.example", net.IPv4(192, 0, 2, 99)), nil
		default:
			return packet.Packet{}, nil
		}
	})

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic03(ctx, &z)
	if err != nil {
		t.Fatalf("basic03: %v", err)
	}
	if !hasEntryTag(entries, "HAS_A_RECORDS") {
		t.Fatalf("expected HAS_A_RECORDS")
	}
	if hasEntryTag(entries, "A_QUERY_NO_RESPONSES") {
		t.Fatalf("unexpected A_QUERY_NO_RESPONSES")
	}
}

func TestBasic03NoARecords(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	root, err := nameserver.NewWithContext(ctx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && (kind == "SOA" || kind == "NS"):
			return referralPacket("example", "ns1.example", net.IPv4(192, 0, 2, 53)), nil
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), nil
		default:
			return packet.Packet{}, nil
		}
	})

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.53", r.Client())
	if err != nil {
		t.Fatalf("new ns1: %v", err)
	}
	ns1.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && kind == "SOA":
			return soaPacket("example", "ns1.example", "hostmaster.example"), nil
		case name == "www.example" && kind == "A":
			return emptyAnswerPacket(), nil
		default:
			return packet.Packet{}, nil
		}
	})

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic03(ctx, &z)
	if err != nil {
		t.Fatalf("basic03: %v", err)
	}
	if !hasEntryTag(entries, "NO_A_RECORDS") {
		t.Fatalf("expected NO_A_RECORDS")
	}
	if hasEntryTag(entries, "HAS_A_RECORDS") {
		t.Fatalf("unexpected HAS_A_RECORDS")
	}
}

func TestBasic03NoResponses(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	root, err := nameserver.NewWithContext(ctx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "example" && (kind == "SOA" || kind == "NS"):
			return referralPacket("example", "ns1.example", net.IPv4(192, 0, 2, 53)), nil
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.root"), nil
		default:
			return packet.Packet{}, nil
		}
	})

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.53", r.Client())
	if err != nil {
		t.Fatalf("new ns1: %v", err)
	}
	ns1.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		if name == "example" && kind == "SOA" {
			return soaPacket("example", "ns1.example", "hostmaster.example"), nil
		}
		return packet.Packet{}, nil
	})

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	entries, err := Basic03(ctx, &z)
	if err != nil {
		t.Fatalf("basic03: %v", err)
	}
	if !hasEntryTag(entries, "A_QUERY_NO_RESPONSES") {
		t.Fatalf("expected A_QUERY_NO_RESPONSES")
	}
}

func TestBasic03ParallelQueries(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	baseCtx, prof, _ := testhelpers.Context(t)
	prof.Resolver.Defaults.Parallel = 2

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.53"},
		"ns2.example": {"192.0.2.54"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	root, err := nameserver.NewWithContext(baseCtx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
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
	nsHook := func(id string, addr net.IP, block bool) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
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

	ns1, err := nameserver.NewWithContext(baseCtx, "ns1.example", "192.0.2.53", r.Client())
	if err != nil {
		t.Fatalf("new ns1: %v", err)
	}
	ns1.SetQueryHook(nsHook("ns1.example", net.IPv4(192, 0, 2, 53), true))

	ns2, err := nameserver.NewWithContext(baseCtx, "ns2.example", "192.0.2.54", r.Client())
	if err != nil {
		t.Fatalf("new ns2: %v", err)
	}
	ns2.SetQueryHook(nsHook("ns2.example", net.IPv4(192, 0, 2, 54), false))

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var basicErr error
	go func() {
		entries, basicErr = Basic03(ctx, &z)
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
	for _, entry := range entries {
		if entry == nil || entry.Tag != "IPV4_ENABLED" {
			continue
		}
		if _, ok := entry.Args["arg_schema"]; ok {
			t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
		}
		if ns, ok := entry.Args["ns"].(string); ok {
			if strings.Contains(ns, "/") {
				t.Fatalf("expected nameserver-only ns argument, got %q", ns)
			}
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
	if !hasEntryTag(entries, "HAS_A_RECORDS") {
		t.Fatalf("expected HAS_A_RECORDS")
	}
}

func TestBasic03ParallelOutputStable(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	runBasic03 := func(parallel int) []*logger.Entry {
		ctx, prof, _ := testhelpers.Context(t)
		prof.Resolver.Defaults.Parallel = parallel

		r := &recursor.Recursor{}
		if err := r.AddFakeAddresses(".", map[string][]string{
			"a.root": {"192.0.2.1"},
		}); err != nil {
			t.Fatalf("add root hints: %v", err)
		}
		if err := r.AddFakeAddresses("example", map[string][]string{
			"ns1.example": {"192.0.2.53"},
			"ns2.example": {"192.0.2.54"},
		}); err != nil {
			t.Fatalf("add fake addresses: %v", err)
		}

		root, err := nameserver.NewWithContext(ctx, "a.root", "192.0.2.1", r.Client())
		if err != nil {
			t.Fatalf("new root nameserver: %v", err)
		}
		root.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
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

		nsHook := func(owner string, addr net.IP) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
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

		ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.53", r.Client())
		if err != nil {
			t.Fatalf("new ns1: %v", err)
		}
		ns1.SetQueryHook(nsHook("ns1.example", net.IPv4(192, 0, 2, 53)))

		ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.54", r.Client())
		if err != nil {
			t.Fatalf("new ns2: %v", err)
		}
		ns2.SetQueryHook(nsHook("ns2.example", net.IPv4(192, 0, 2, 54)))

		z, err := zone.NewWithRecursor("example", r)
		if err != nil {
			t.Fatalf("new zone: %v", err)
		}

		entries, err := Basic03(ctx, &z)
		if err != nil {
			t.Fatalf("basic03: %v", err)
		}
		if !hasEntryTag(entries, "HAS_A_RECORDS") {
			t.Fatalf("expected HAS_A_RECORDS")
		}
		return entries
	}

	sequentialEntries := runBasic03(1)
	parallelEntries := runBasic03(2)

	sequentialNormalized := normalizeEntriesForComparison(sequentialEntries)
	parallelNormalized := normalizeEntriesForComparison(parallelEntries)

	if len(sequentialNormalized) != len(parallelNormalized) {
		t.Fatalf("entry count changed with parallelism: sequential=%v parallel=%v", sequentialNormalized, parallelNormalized)
	}
	for i := range sequentialNormalized {
		if sequentialNormalized[i] != parallelNormalized[i] {
			t.Fatalf("entry[%d] changed with parallelism: %q != %q", i, sequentialNormalized[i], parallelNormalized[i])
		}
	}
}

func hasEntryTag(entries []*logger.Entry, tag string) bool {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Tag == tag {
			return true
		}
	}
	return false
}

func firstEntryByTag(entries []*logger.Entry, tag string) *logger.Entry {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Tag == tag {
			return entry
		}
	}
	return nil
}

func normalizeEntriesForComparison(entries []*logger.Entry) []string {
	normalized := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		item := entry.Module + ":" + entry.Testcase + ":" + entry.Tag
		if args := entry.ArgString(); args != "" {
			item += " " + args
		}
		normalized = append(normalized, item)
	}
	return normalized
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
