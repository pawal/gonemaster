package connectivity

import (
	"context"
	"net/netip"
	"sort"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/asnlookup"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestConnectivity01IPv6Disabled(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origMethod := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod })

	ns4 := newNameserver(t, "ns1.example", "192.0.2.1", nil)
	ns6 := newNameserver(t, "ns2.example", "2001:db8::1", nil)
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns4, ns6}, nil
	}

	profile.Effective().Net.IPv6 = false

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Connectivity01(context.Background(), &z)
	if err != nil {
		t.Fatalf("connectivity01: %v", err)
	}
	if !hasEntryTag(entries, "CN01_IPV6_DISABLED") {
		t.Fatalf("expected CN01_IPV6_DISABLED")
	}
	entry := findEntry(entries, "CN01_IPV6_DISABLED")
	if entry == nil {
		t.Fatalf("expected CN01_IPV6_DISABLED entry")
	}
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("did not expect legacy ns_list key in args")
	}
	names := serverNamesFromArgs(t, entry.Args)
	if len(names) != 1 || names[0] != "ns2.example" {
		t.Fatalf("expected typed servers with ns2.example, got %v", names)
	}
}

func TestConnectivityLoopNoResponse(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	ns := newNameserver(t, "ns1.example", "192.0.2.1", nil)
	results := []*logger.Entry{}

	if err := connectivityLoop(context.Background(), "Connectivity01", dnsname.New("example"), []nameserver.Nameserver{ns}, &results); err != nil {
		t.Fatalf("connectivity loop: %v", err)
	}
	if !hasEntryTag(results, "CN01_NO_RESPONSE_UDP") {
		t.Fatalf("expected CN01_NO_RESPONSE_UDP")
	}
}

func TestConnectivityLoopWrongSOAOwner(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	ns := newNameserver(t, "ns1.example", "192.0.2.1", func(qname string, qtype string) packet.Packet {
		switch strings.ToUpper(qtype) {
		case "SOA":
			return soaPacket("wrong.example", "ns1.example", "hostmaster.example")
		case "NS":
			return nsPacket("example", "ns1.example")
		default:
			return packet.Packet{}
		}
	})

	results := []*logger.Entry{}
	if err := connectivityLoop(context.Background(), "Connectivity01", dnsname.New("example"), []nameserver.Nameserver{ns}, &results); err != nil {
		t.Fatalf("connectivity loop: %v", err)
	}
	if !hasEntryTag(results, "CN01_WRONG_SOA_RECORD_UDP") {
		t.Fatalf("expected CN01_WRONG_SOA_RECORD_UDP")
	}
}

func TestConnectivity03SameASNSet(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origMethod := authoritativeNS
	origLookup := lookupASN
	t.Cleanup(func() {
		authoritativeNS = origMethod
		lookupASN = origLookup
	})

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", nil)
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", nil)
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}

	prefix, err := netip.ParsePrefix("192.0.2.0/24")
	if err != nil {
		t.Fatalf("parse prefix: %v", err)
	}
	lookupASN = func(_ context.Context, _ asnlookup.Resolver, _ netip.Addr) (asnlookup.Result, error) {
		return asnlookup.Result{
			ASNs:   []int{64500, 64501},
			Prefix: &prefix,
			Raw:    "64500 64501 | 192.0.2.0/24 | test",
			Code:   asnlookup.CodeFound,
		}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	entries, err := Connectivity03(context.Background(), &z)
	if err != nil {
		t.Fatalf("connectivity03: %v", err)
	}
	if !hasEntryTag(entries, "IPV4_SAME_ASN") {
		t.Fatalf("expected IPV4_SAME_ASN")
	}
	sameASN := findEntry(entries, "IPV4_SAME_ASN")
	if sameASN == nil {
		t.Fatalf("expected IPV4_SAME_ASN entry")
	}
	if _, ok := sameASN.Args["asn_list"]; ok {
		t.Fatalf("did not expect legacy asn_list key in args")
	}
	asns := intSliceFromArgs(t, sameASN.Args, "asns")
	if len(asns) != 2 || asns[0] != 64500 || asns[1] != 64501 {
		t.Fatalf("expected asns [64500 64501], got %v", asns)
	}
	announce := findEntry(entries, "ASN_INFOS_ANNOUNCE_BY")
	if announce == nil {
		t.Fatalf("expected ASN_INFOS_ANNOUNCE_BY entry")
	}
	if _, ok := announce.Args["asn"]; ok {
		t.Fatalf("did not expect legacy asn key in ASN_INFOS_ANNOUNCE_BY args")
	}
	announceASNs := intSliceFromArgs(t, announce.Args, "asns")
	if len(announceASNs) != 2 || announceASNs[0] != 64500 || announceASNs[1] != 64501 {
		t.Fatalf("expected announce asns [64500 64501], got %v", announceASNs)
	}
	announceIn := findEntry(entries, "ASN_INFOS_ANNOUNCE_IN")
	if announceIn == nil {
		t.Fatalf("expected ASN_INFOS_ANNOUNCE_IN entry")
	}
	if _, ok := announceIn.Args["prefix"]; ok {
		t.Fatalf("did not expect legacy prefix key in ASN_INFOS_ANNOUNCE_IN args")
	}
	prefixes := stringSliceFromArgs(t, announceIn.Args, "prefixes")
	if len(prefixes) != 1 || prefixes[0] != "192.0.2.0/24" {
		t.Fatalf("expected prefixes [192.0.2.0/24], got %v", prefixes)
	}
}

func TestConnectivityLoopParallelQueries(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	profile.Effective().Resolver.Defaults.Parallel = 2

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if strings.EqualFold(qtype, "SOA") {
			select {
			case started <- qname:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
		}
		return packet.Packet{}, nil
	}

	ns1, err := nameserver.New("ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook)

	ns2, err := nameserver.New("ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var results []*logger.Entry
	var loopErr error
	go func() {
		loopErr = connectivityLoop(ctx, "Connectivity01", dnsname.New("example"), []nameserver.Nameserver{ns1, ns2}, &results)
		close(done)
	}()

	count := 0
	deadline := time.After(1 * time.Second)
	for count < 2 {
		select {
		case <-started:
			count++
		case <-deadline:
			t.Fatalf("expected parallel SOA queries to start, got %d", count)
		}
	}

	close(release)

	select {
	case <-done:
		if loopErr != nil {
			t.Fatalf("connectivity loop: %v", loopErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("connectivity loop did not finish")
	}

	var order []string
	var addresses []string
	for _, entry := range results {
		if entry == nil || entry.Tag != "CN01_NO_RESPONSE_UDP" {
			continue
		}
		if _, ok := entry.Args["arg_schema"]; ok {
			t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
		}
		if ns, ok := entry.Args["ns"].(string); ok {
			if strings.Contains(ns, "/") {
				t.Fatalf("expected nameserver-only ns argument, got %q", ns)
			}
			order = append(order, ns)
		}
		if address, ok := entry.Args["address"].(string); ok {
			addresses = append(addresses, address)
		}
	}
	if len(order) != 2 {
		t.Fatalf("expected 2 no-response entries, got %v", order)
	}
	if order[0] != "ns1.example" || order[1] != "ns2.example" {
		t.Fatalf("expected deterministic nameserver order, got %v", order)
	}
	if len(addresses) != 2 {
		t.Fatalf("expected 2 no-response addresses, got %v", addresses)
	}
	if addresses[0] != "192.0.2.1" || addresses[1] != "192.0.2.2" {
		t.Fatalf("expected deterministic address order, got %v", addresses)
	}
}

func TestConnectivity03ParallelASNLookups(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origMethod := authoritativeNS
	origLookup := lookupASN
	t.Cleanup(func() {
		authoritativeNS = origMethod
		lookupASN = origLookup
	})

	profile.Effective().Resolver.Defaults.Parallel = 2

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", nil)
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", nil)
	authoritativeNS = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}

	started := make(chan string, 2)
	release := make(chan struct{})
	prefix, err := netip.ParsePrefix("192.0.2.0/24")
	if err != nil {
		t.Fatalf("parse prefix: %v", err)
	}
	lookupASN = func(ctx context.Context, _ asnlookup.Resolver, ip netip.Addr) (asnlookup.Result, error) {
		select {
		case started <- ip.String():
		default:
		}
		select {
		case <-release:
		case <-ctx.Done():
			return asnlookup.Result{}, ctx.Err()
		}
		return asnlookup.Result{
			ASNs:   []int{64500},
			Prefix: &prefix,
			Code:   asnlookup.CodeFound,
		}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var connErr error
	go func() {
		entries, connErr = Connectivity03(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel ASN lookups to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if connErr != nil {
			t.Fatalf("connectivity03: %v", connErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("connectivity03 did not finish")
	}

	var order []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "ASN_INFOS_ANNOUNCE_BY" {
			continue
		}
		if ip, ok := entry.Args["address"].(string); ok {
			order = append(order, ip)
		}
	}
	if len(order) != 2 {
		t.Fatalf("expected 2 announce-by entries, got %v", order)
	}
	if order[0] != "192.0.2.1" || order[1] != "192.0.2.2" {
		t.Fatalf("expected deterministic log order, got %v", order)
	}
}

func TestConnectivity04SinglePrefix(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := delegationNameservers
	origZone := zoneNameservers
	origLookup := lookupASN
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
		lookupASN = origLookup
	})

	items := []nsdiscovery.NSItem{
		{
			Name:       dnsname.New("ns1.example"),
			Address:    netip.MustParseAddr("192.0.2.1"),
			HasAddress: true,
		},
		{
			Name:       dnsname.New("ns2.example"),
			Address:    netip.MustParseAddr("192.0.2.2"),
			HasAddress: true,
		},
	}
	delegationNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return items, nil
	}
	zoneNameservers = func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	}

	prefix, err := netip.ParsePrefix("192.0.2.0/24")
	if err != nil {
		t.Fatalf("parse prefix: %v", err)
	}
	lookupASN = func(_ context.Context, _ asnlookup.Resolver, _ netip.Addr) (asnlookup.Result, error) {
		return asnlookup.Result{
			Prefix: &prefix,
			Raw:    "64500 | 192.0.2.0/24 | test",
			Code:   asnlookup.CodeFound,
		}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	entries, err := Connectivity04(context.Background(), &z)
	if err != nil {
		t.Fatalf("connectivity04: %v", err)
	}
	if !hasEntryTag(entries, "CN04_IPV4_SAME_PREFIX") {
		t.Fatalf("expected CN04_IPV4_SAME_PREFIX")
	}
	samePrefix := findEntry(entries, "CN04_IPV4_SAME_PREFIX")
	if samePrefix == nil {
		t.Fatalf("expected CN04_IPV4_SAME_PREFIX entry")
	}
	if _, ok := samePrefix.Args["ns_list"]; ok {
		t.Fatalf("did not expect legacy ns_list key in args")
	}
	if _, ok := samePrefix.Args["ip_prefix"]; ok {
		t.Fatalf("did not expect legacy ip_prefix key in args")
	}
	names := serverNamesFromArgs(t, samePrefix.Args)
	if len(names) != 2 || names[0] != "ns1.example" || names[1] != "ns2.example" {
		t.Fatalf("expected typed servers [ns1.example ns2.example], got %v", names)
	}
	prefixes := stringSliceFromArgs(t, samePrefix.Args, "prefixes")
	if len(prefixes) != 1 || prefixes[0] != "192.0.2.0/24" {
		t.Fatalf("expected prefixes [192.0.2.0/24], got %v", prefixes)
	}
	announceIn := findEntry(entries, "CN04_ASN_INFOS_ANNOUNCE_IN")
	if announceIn == nil {
		t.Fatalf("expected CN04_ASN_INFOS_ANNOUNCE_IN entry")
	}
	if _, ok := announceIn.Args["prefix"]; ok {
		t.Fatalf("did not expect legacy prefix key in CN04_ASN_INFOS_ANNOUNCE_IN args")
	}
	announcePrefixes := stringSliceFromArgs(t, announceIn.Args, "prefixes")
	if len(announcePrefixes) != 1 || announcePrefixes[0] != "192.0.2.0/24" {
		t.Fatalf("expected announce prefixes [192.0.2.0/24], got %v", announcePrefixes)
	}
	if !hasEntryTag(entries, "CN04_IPV4_SINGLE_PREFIX") {
		t.Fatalf("expected CN04_IPV4_SINGLE_PREFIX")
	}
}

func intSliceFromArgs(t *testing.T, args map[string]any, key string) []int {
	t.Helper()
	raw, ok := args[key]
	if !ok {
		t.Fatalf("expected %s key in args", key)
	}
	switch items := raw.(type) {
	case []int:
		out := append([]int{}, items...)
		sort.Ints(out)
		return out
	case []any:
		out := make([]int, 0, len(items))
		for _, item := range items {
			switch v := item.(type) {
			case int:
				out = append(out, v)
			case float64:
				out = append(out, int(v))
			default:
				t.Fatalf("unexpected %s element type: %T", key, item)
			}
		}
		sort.Ints(out)
		return out
	default:
		t.Fatalf("unexpected %s type: %T", key, raw)
	}
	return nil
}

func stringSliceFromArgs(t *testing.T, args map[string]any, key string) []string {
	t.Helper()
	raw, ok := args[key]
	if !ok {
		t.Fatalf("expected %s key in args", key)
	}
	switch items := raw.(type) {
	case []string:
		return append([]string{}, items...)
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			value, ok := item.(string)
			if !ok {
				t.Fatalf("unexpected %s element type: %T", key, item)
			}
			out = append(out, value)
		}
		return out
	default:
		t.Fatalf("unexpected %s type: %T", key, raw)
	}
	return nil
}

func newNameserver(t *testing.T, name string, ip string, handler func(qname string, qtype string) packet.Packet) nameserver.Nameserver {
	t.Helper()

	ns, err := nameserver.New(name, ip, nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if handler == nil {
		handler = func(_ string, _ string) packet.Packet {
			return packet.Packet{}
		}
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return handler(qname, qtype), nil
	})
	return ns
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

func findEntry(entries []*logger.Entry, tag string) *logger.Entry {
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

func serverNamesFromArgs(t *testing.T, args map[string]any) []string {
	t.Helper()
	raw, ok := args["servers"]
	if !ok {
		t.Fatalf("expected servers key in args")
	}

	var names []string
	switch items := raw.(type) {
	case []map[string]any:
		for _, item := range items {
			if ns, ok := item["ns"].(string); ok && ns != "" {
				names = append(names, ns)
			}
		}
	case []any:
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if ns, ok := m["ns"].(string); ok && ns != "" {
				names = append(names, ns)
			}
		}
	default:
		t.Fatalf("unexpected servers type: %T", raw)
	}

	sort.Strings(names)
	return names
}

func soaPacket(owner string, mname string, rname string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	soaRR := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	soaRR.Ns = dnsutil.Fqdn(mname)
	soaRR.Mbox = dnsutil.Fqdn(rname)
	soaRR.Serial = 2024010101
	soaRR.Refresh = 3600
	soaRR.Retry = 600
	soaRR.Expire = 86400
	soaRR.Minttl = 60
	msg.Answer = []dns.RR{soaRR}
	return packet.Packet{Msg: msg}
}

func nsPacket(owner string, nsname string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	nsRR := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	nsRR.Ns = dnsutil.Fqdn(nsname)
	msg.Answer = []dns.RR{nsRR}
	return packet.Packet{Msg: msg}
}
