package basic

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

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
	for _, entry := range entries {
		if entry == nil || entry.Tag != "IPV4_ENABLED" {
			continue
		}
		if ns, ok := entry.Args["ns"].(string); ok {
			enabled = append(enabled, ns)
		}
	}
	if len(enabled) < 2 {
		t.Fatalf("expected IPV4_ENABLED entries for both nameservers, got %v", enabled)
	}
	if enabled[0] != "a.root/192.0.2.1" || enabled[1] != "b.root/192.0.2.2" {
		t.Fatalf("expected deterministic log order, got %v", enabled)
	}
	if !hasEntryTag(entries, "B02_AUTH_RESPONSE_SOA") {
		t.Fatalf("expected B02_AUTH_RESPONSE_SOA")
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
	for _, entry := range entries {
		if entry == nil || entry.Tag != "IPV4_ENABLED" {
			continue
		}
		if ns, ok := entry.Args["ns"].(string); ok {
			enabled = append(enabled, ns)
		}
	}
	if len(enabled) < 2 {
		t.Fatalf("expected IPV4_ENABLED entries for both nameservers, got %v", enabled)
	}
	if enabled[0] != "ns1.example/192.0.2.53" || enabled[1] != "ns2.example/192.0.2.54" {
		t.Fatalf("expected deterministic log order, got %v", enabled)
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
	msg.Answer = []dns.RR{
		&dns.SOA{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeSOA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns:      dns.Fqdn(mname),
			Mbox:    dns.Fqdn(rname),
			Serial:  1,
			Refresh: 3600,
			Retry:   600,
			Expire:  86400,
			Minttl:  60,
		},
	}
	return packet.Packet{Msg: msg}
}

func referralPacket(zoneName string, nsName string, nsAddr net.IP) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Ns = []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(zoneName),
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns: dns.Fqdn(nsName),
		},
	}
	msg.Extra = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(nsName),
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			A: nsAddr,
		},
	}
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
		msg.Ns = append(msg.Ns, &dns.NS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(zoneName),
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns: dns.Fqdn(entry.name),
		})
		msg.Extra = append(msg.Extra, &dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(entry.name),
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			A: entry.addr,
		})
	}
	return packet.Packet{Msg: msg}
}

func aPacket(owner string, addr net.IP) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			A: addr,
		},
	}
	return packet.Packet{Msg: msg}
}

func nsPacket(owner string, nsname string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	msg.Answer = []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns: dns.Fqdn(nsname),
		},
	}
	return packet.Packet{Msg: msg}
}

func nsPacketMulti(owner string, nsnames ...string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	for _, nsname := range nsnames {
		msg.Answer = append(msg.Answer, &dns.NS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns: dns.Fqdn(nsname),
		})
	}
	return packet.Packet{Msg: msg}
}

func rcodePacket(rcode int) packet.Packet {
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
