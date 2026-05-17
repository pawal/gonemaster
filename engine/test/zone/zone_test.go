package zone

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	methodsv2 "codeberg.org/pawal/gonemaster/engine/methodsv2"
	ens "codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/util"
	zonepkg "codeberg.org/pawal/gonemaster/engine/zone"
)

func TestZone02RefreshBelowMinimum(t *testing.T) {
	setupTest(t)

	origMethod5 := method5
	t.Cleanup(func() { method5 = origMethod5 })

	profile.Effective().TestCasesVars.Zone02.SOARefreshMinimumValue = 1000

	ns := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, _ string, _ *ens.QueryOptions) packet.Packet {
		return soaPacket("example", 1, 500, 50, 7200, 60)
	})
	method5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone02(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone02: %v", err)
	}
	if !hasEntryTag(entries, "REFRESH_MINIMUM_VALUE_LOWER") {
		t.Fatalf("expected REFRESH_MINIMUM_VALUE_LOWER")
	}
}

func TestZone05ExpireLowerThanRefreshAndMinimum(t *testing.T) {
	setupTest(t)

	origMethod5 := method5
	t.Cleanup(func() { method5 = origMethod5 })

	profile.Effective().TestCasesVars.Zone05.SOAExpireMinimumValue = 2000

	ns := newNameserver(t, "ns1.example", "192.0.2.2", func(_ string, _ string, _ *ens.QueryOptions) packet.Packet {
		return soaPacket("example", 1, 1500, 100, 1000, 60)
	})
	method5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone05(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone05: %v", err)
	}
	if !hasEntryTag(entries, "EXPIRE_MINIMUM_VALUE_LOWER") {
		t.Fatalf("expected EXPIRE_MINIMUM_VALUE_LOWER")
	}
	if !hasEntryTag(entries, "EXPIRE_LOWER_THAN_REFRESH") {
		t.Fatalf("expected EXPIRE_LOWER_THAN_REFRESH")
	}
}

func TestZone10ParallelQueries(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	profile.Effective().Resolver.Defaults.Parallel = 2

	started := make(chan string, 2)
	release := make(chan struct{})

	hook := func(id string) func(context.Context, string, string, string, *ens.QueryOptions) (packet.Packet, error) {
		return func(ctx context.Context, _ string, qtype string, _ string, _ *ens.QueryOptions) (packet.Packet, error) {
			if qtype == "SOA" {
				select {
				case started <- id:
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
	}

	ns1, err := ens.New("ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := ens.New("ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var zoneErr error
	go func() {
		entries, zoneErr = Zone10(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-deadline:
			t.Fatalf("expected parallel SOA queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if zoneErr != nil {
			t.Fatalf("zone10: %v", zoneErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("zone10 did not finish")
	}

	var order []string
	var addresses []string
	for _, entry := range entries {
		if entry == nil || entry.Tag != "NO_RESPONSE" {
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

func TestZone10WrongSOAUsesQueryName(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype != "SOA" {
			return packet.Packet{}
		}
		msg := new(dns.Msg)
		msg.Authoritative = true
		msg.Rcode = dns.RcodeSuccess
		soa := &dns.SOA{Hdr: dns.Header{Name: "wrong.example.", Class: dns.ClassINET, TTL: 300}}
		soa.Ns = "ns1.example."
		soa.Mbox = "hostmaster.example."
		soa.Serial = 1
		soa.Refresh = 1
		soa.Retry = 1
		soa.Expire = 1
		soa.Minttl = 1
		msg.Answer = []dns.RR{soa}
		return packet.Packet{Msg: msg}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone10(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone10: %v", err)
	}
	if !hasEntryTag(entries, "WRONG_SOA") {
		t.Fatalf("expected WRONG_SOA")
	}
	var entry *logger.Entry
	for _, e := range entries {
		if e != nil && e.Tag == "WRONG_SOA" {
			entry = e
			break
		}
	}
	if entry == nil {
		t.Fatalf("missing WRONG_SOA")
	}
	if got, ok := entry.Args["query_name"].(string); !ok || got != "example.com." {
		t.Fatalf("expected query_name=example.com., got %#v", entry.Args["query_name"])
	}
	if _, ok := entry.Args["name"]; ok {
		t.Fatalf("legacy key name should not be present: %#v", entry.Args)
	}
}

func TestZone09MXQueryDisablesFallback(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	profile.Effective().Resolver.Defaults.Parallel = 1

	var mu sync.Mutex
	var fallbackValues []bool
	var useVCValues []bool
	mxCalls := 0

	ns := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, opts *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return soaPacket("example", 1, 1, 1, 1, 1)
		case "MX":
			mu.Lock()
			mxCalls++
			call := mxCalls
			fallback := false
			if opts != nil && opts.Fallback != nil {
				fallback = *opts.Fallback
			}
			fallbackValues = append(fallbackValues, fallback)
			if opts != nil && opts.UseVC != nil {
				useVCValues = append(useVCValues, *opts.UseVC)
			} else {
				useVCValues = append(useVCValues, false)
			}
			mu.Unlock()

			msg := new(dns.Msg)
			dnsutil.SetQuestion(msg, dnsutil.Fqdn("example"), dns.TypeMX)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			if call == 1 {
				msg.Truncated = true
			}
			return packet.Packet{Msg: msg}
		default:
			return packet.Packet{}
		}
	})

	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	if _, err := Zone09(context.Background(), &z); err != nil {
		t.Fatalf("zone09: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(fallbackValues) == 0 {
		t.Fatalf("expected MX query to be issued")
	}
	for _, fallback := range fallbackValues {
		if fallback {
			t.Fatalf("expected MX query fallback to be disabled")
		}
	}
	if len(useVCValues) < 2 || useVCValues[0] || !useVCValues[1] {
		t.Fatalf("expected MX query to retry with UseVC after truncation, got %v", useVCValues)
	}
}

func TestZone09MXDataUsesTypedMailTargets(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return soaPacket("example.com", 1, 1, 1, 1, 1)
		case "MX":
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			mx := &dns.MX{Hdr: dns.Header{Name: "example.com.", Class: dns.ClassINET, TTL: 300}}
			mx.Mx = "mail.example."
			mx.Preference = 10
			msg.Answer = []dns.RR{mx}
			return packet.Packet{Msg: msg}
		default:
			return packet.Packet{}
		}
	})

	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone09(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone09: %v", err)
	}

	var mxData *logger.Entry
	for _, entry := range entries {
		if entry != nil && entry.Tag == "Z09_MX_DATA" {
			mxData = entry
			break
		}
	}
	if mxData == nil {
		t.Fatalf("expected Z09_MX_DATA")
	}
	targets, ok := mxData.Args["mail_targets"].([]string)
	if !ok || len(targets) != 1 || targets[0] != "mail.example" {
		t.Fatalf("expected typed mail_targets [mail.example], got %#v", mxData.Args["mail_targets"])
	}
	addresses, ok := mxData.Args["addresses"].([]string)
	if !ok || len(addresses) != 1 || addresses[0] != "192.0.2.1" {
		t.Fatalf("expected typed addresses [192.0.2.1], got %#v", mxData.Args["addresses"])
	}
	if _, ok := mxData.Args["mailtarget_list"]; ok {
		t.Fatalf("legacy key mailtarget_list should not be present: %#v", mxData.Args)
	}
}

func TestZone11SpfSyntaxError(t *testing.T) {
	setupTest(t)

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
	})

	z, err := zonepkg.New("example.com")
	if err != nil {
		t.Fatalf("zone11: %v", err)
	}

	newNameserver(t, "ns1.example.com", "192.0.2.10", func(qname string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype != "TXT" {
			return packet.Packet{}
		}
		return txtPacket(qname, "v=spf1 amx-all")
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zonepkg.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{{
			Name:       dnsname.New("ns1.example.com"),
			Address:    netip.MustParseAddr("192.0.2.10"),
			HasAddress: true,
		}}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zonepkg.Zone) ([]methodsv2.NSItem, error) {
		return nil, nil
	}

	entries, err := Zone11(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone11: %v", err)
	}
	if !hasEntryTag(entries, "Z11_SPF_SYNTAX_ERROR") {
		t.Fatalf("expected Z11_SPF_SYNTAX_ERROR")
	}
}

func TestZone11NoSpfNonMailDomain(t *testing.T) {
	setupTest(t)

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
	})

	z, err := zonepkg.New("se")
	if err != nil {
		t.Fatalf("zone11: %v", err)
	}

	newNameserver(t, "ns1.se", "192.0.2.10", func(qname string, qtype string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, dnsutil.Fqdn(qname), dns.TypeTXT)
		msg.Authoritative = true
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}
	})

	getDelNSNamesAndIPs = func(_ context.Context, _ *zonepkg.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{{
			Name:       dnsname.New("ns1.se"),
			Address:    netip.MustParseAddr("192.0.2.10"),
			HasAddress: true,
		}}, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zonepkg.Zone) ([]methodsv2.NSItem, error) {
		return nil, nil
	}

	entries, err := Zone11(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone11: %v", err)
	}
	var found *logger.Entry
	for _, entry := range entries {
		if entry != nil && entry.Tag == "Z11_NO_SPF_NON_MAIL_DOMAIN" {
			found = entry
			break
		}
	}
	if found == nil {
		t.Fatalf("expected Z11_NO_SPF_NON_MAIL_DOMAIN")
	}
	if domain, ok := found.Args["domain"].(string); !ok || domain != "se" {
		t.Fatalf("expected domain=\"se\", got %v", found.Args["domain"])
	}
}

func setupTest(t *testing.T) {
	t.Helper()

	ens.EmptyCache()
	t.Cleanup(ens.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })
}

func newNameserver(t *testing.T, name string, ip string, handler func(qname string, qtype string, opts *ens.QueryOptions) packet.Packet) ens.Nameserver {
	t.Helper()

	ns, err := ens.New(name, ip, nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if handler == nil {
		handler = func(_ string, _ string, _ *ens.QueryOptions) packet.Packet {
			return packet.Packet{}
		}
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, opts *ens.QueryOptions) (packet.Packet, error) {
		return handler(qname, qtype, opts), nil
	})
	return ns
}

func soaPacket(owner string, serial uint32, refresh uint32, retry uint32, expire uint32, minimum uint32) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(owner), dns.TypeSOA)
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	soaRR := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	soaRR.Ns = dnsutil.Fqdn("ns1.example")
	soaRR.Mbox = dnsutil.Fqdn("hostmaster.example")
	soaRR.Serial = serial
	soaRR.Refresh = refresh
	soaRR.Retry = retry
	soaRR.Expire = expire
	soaRR.Minttl = minimum
	msg.Answer = []dns.RR{soaRR}
	return packet.Packet{Msg: msg}
}

func txtPacket(name string, value string) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(name), dns.TypeTXT)
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	txtRR := &dns.TXT{Hdr: dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 60}}
	txtRR.Txt = []string{value}
	msg.Answer = []dns.RR{txtRR}
	return packet.Packet{Msg: msg}
}

func csyncPacket(name string, serial uint32, flags uint16, types []uint16) packet.Packet {
	msg := new(dns.Msg)
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	csync := &dns.CSYNC{}
	csync.Hdr = dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 300}
	csync.CSYNC.Serial = serial
	csync.CSYNC.Flags = flags
	csync.CSYNC.TypeBitMap = types
	msg.Answer = []dns.RR{csync}
	return packet.Packet{Msg: msg}
}

func TestZone12CSYNCFound(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	const serial uint32 = 2024010101
	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "CSYNC":
			return csyncPacket("example", serial, 0x0001, []uint16{dns.TypeNS, dns.TypeA, dns.TypeAAAA})
		case "SOA":
			return soaPacket("example", serial, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_CSYNC_FOUND") {
		t.Fatalf("expected Z12_CSYNC_FOUND")
	}
}

func TestZone12NoCSYNC(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "CSYNC":
			// Authoritative NOERROR with no CSYNC in answer.
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}
		case "SOA":
			return soaPacket("example", 2024010101, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_NO_CSYNC") {
		t.Fatalf("expected Z12_NO_CSYNC")
	}
}

func TestZone12SerialMismatch(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "CSYNC":
			// CSYNC carries serial 100, but SOA has serial 200.
			return csyncPacket("example", 100, 0, []uint16{dns.TypeNS})
		case "SOA":
			return soaPacket("example", 200, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_SERIAL_MISMATCH") {
		t.Fatalf("expected Z12_SERIAL_MISMATCH")
	}
}

func TestZone12SerialMismatchSoaMinimumNewerCSYNC(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "CSYNC":
			// soaminimum set, but CSYNC serial is newer than current SOA serial.
			return csyncPacket("example", 300, csyncFlagSoaMinimum, []uint16{dns.TypeNS})
		case "SOA":
			return soaPacket("example", 200, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_SERIAL_MISMATCH") {
		t.Fatalf("expected Z12_SERIAL_MISMATCH")
	}
}

func TestZone12SerialMismatchSoaMinimumOlderCSYNCNotMismatch(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "CSYNC":
			// soaminimum set and SOA serial has advanced, which is valid.
			return csyncPacket("example", 100, csyncFlagSoaMinimum, []uint16{dns.TypeNS})
		case "SOA":
			return soaPacket("example", 200, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if hasEntryTag(entries, "Z12_SERIAL_MISMATCH") {
		t.Fatalf("did not expect Z12_SERIAL_MISMATCH")
	}
}

func TestZone12MultipleCSYNC(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype != "CSYNC" {
			return packet.Packet{}
		}
		msg := new(dns.Msg)
		msg.Authoritative = true
		msg.Rcode = dns.RcodeSuccess
		for i := range 2 {
			csync := &dns.CSYNC{}
			csync.Hdr = dns.Header{Name: "example.", Class: dns.ClassINET, TTL: 300}
			csync.CSYNC.Serial = uint32(2024010100 + i)
			csync.CSYNC.TypeBitMap = []uint16{dns.TypeNS}
			msg.Answer = append(msg.Answer, csync)
		}
		return packet.Packet{Msg: msg}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_MULTIPLE_CSYNC") {
		t.Fatalf("expected Z12_MULTIPLE_CSYNC")
	}
}

func TestZone12InconsistentCSYNC(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	// ns1 and ns2 return CSYNC records with different serials.
	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "CSYNC" {
			return csyncPacket("example", 2024010101, 0, []uint16{dns.TypeNS})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "CSYNC" {
			return csyncPacket("example", 2024010199, 0, []uint16{dns.TypeNS})
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_INCONSISTENT_CSYNC") {
		t.Fatalf("expected Z12_INCONSISTENT_CSYNC")
	}
}

func TestZone12MixedPresence(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	// ns1 returns CSYNC, ns2 returns authoritative NOERROR without CSYNC.
	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "CSYNC" {
			return csyncPacket("example", 2024010101, 0, []uint16{dns.TypeNS})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "CSYNC" {
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_MIXED_PRESENCE") {
		t.Fatalf("expected Z12_MIXED_PRESENCE")
	}
}

// --- Zone13 tests ---

// spfTxtPacket creates an authoritative TXT response with an SPF record.
func spfTxtPacket(name string, spf string) packet.Packet {
	return txtPacket(name, spf)
}

// recurseTxtPacket creates a non-authoritative TXT response (for recursive lookups).
func recurseTxtPacket(name string, value string) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(name), dns.TypeTXT)
	msg.Rcode = dns.RcodeSuccess
	txtRR := &dns.TXT{Hdr: dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 60}}
	txtRR.Txt = []string{value}
	msg.Answer = []dns.RR{txtRR}
	return packet.Packet{Msg: msg}
}

func setupZone13(t *testing.T) {
	t.Helper()
	setupTest(t)
	origQueryAuth := queryAuth
	origRecurse := recurse
	t.Cleanup(func() {
		queryAuth = origQueryAuth
		recurse = origRecurse
	})
}

func TestZone13LookupCountOK_NoLookups(t *testing.T) {
	setupZone13(t)

	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 ip4:192.0.2.0/24 ip6:2001:db8::/32 -all"), nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_SPF_LOOKUP_COUNT_OK") {
		t.Fatalf("expected Z13_SPF_LOOKUP_COUNT_OK")
	}
	for _, e := range entries {
		if e != nil && e.Tag == "Z13_SPF_LOOKUP_COUNT_OK" {
			if count, ok := e.Args["count"].(int); !ok || count != 0 {
				t.Fatalf("expected count=0, got %v", e.Args["count"])
			}
		}
	}
}

func TestZone13LookupCountOK_Boundary(t *testing.T) {
	setupZone13(t)

	// SPF with exactly 10 mechanisms that require DNS: a, mx, include (with 7 mechanisms inside)
	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 a mx include:other.example.com -all"), nil
	}
	recurse = func(_ context.Context, _ *zonepkg.Zone, name string, _ string) (packet.Packet, error) {
		if strings.HasPrefix(name, "other.example.com") {
			// 7 more lookups inside: a mx exists:x1 exists:x2 exists:x3 exists:x4 exists:x5
			return recurseTxtPacket("other.example.com", "v=spf1 a mx exists:x1.example.com exists:x2.example.com exists:x3.example.com exists:x4.example.com exists:x5.example.com -all"), nil
		}
		return packet.Packet{}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_SPF_LOOKUP_COUNT_OK") {
		t.Fatalf("expected Z13_SPF_LOOKUP_COUNT_OK, got tags: %v", entryTags(entries))
	}
	for _, e := range entries {
		if e != nil && e.Tag == "Z13_SPF_LOOKUP_COUNT_OK" {
			if count, ok := e.Args["count"].(int); !ok || count != 10 {
				t.Fatalf("expected count=10, got %v", e.Args["count"])
			}
		}
	}
}

func TestZone13LookupCountExceeded(t *testing.T) {
	setupZone13(t)

	// SPF with 11 mechanisms: a mx ptr exists:x1 ... exists:x8
	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 a mx ptr exists:x1.example.com exists:x2.example.com exists:x3.example.com exists:x4.example.com exists:x5.example.com exists:x6.example.com exists:x7.example.com exists:x8.example.com -all"), nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_SPF_LOOKUP_COUNT_EXCEEDED") {
		t.Fatalf("expected Z13_SPF_LOOKUP_COUNT_EXCEEDED, got tags: %v", entryTags(entries))
	}
	// Also should get ptr deprecated
	if !hasEntryTag(entries, "Z13_SPF_PTR_DEPRECATED") {
		t.Fatalf("expected Z13_SPF_PTR_DEPRECATED")
	}
}

func TestZone13IncludeLoop(t *testing.T) {
	setupZone13(t)

	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 include:loop.example.com -all"), nil
	}
	recurse = func(_ context.Context, _ *zonepkg.Zone, name string, _ string) (packet.Packet, error) {
		if strings.HasPrefix(name, "loop.example.com") {
			return recurseTxtPacket("loop.example.com", "v=spf1 include:loop.example.com -all"), nil
		}
		return packet.Packet{}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_SPF_LOOKUP_LOOP") {
		t.Fatalf("expected Z13_SPF_LOOKUP_LOOP, got tags: %v", entryTags(entries))
	}
}

func TestZone13RecursiveError(t *testing.T) {
	setupZone13(t)

	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 include:missing.example.com -all"), nil
	}
	recurse = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return packet.Packet{}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_SPF_RECURSIVE_ERROR") {
		t.Fatalf("expected Z13_SPF_RECURSIVE_ERROR, got tags: %v", entryTags(entries))
	}
}

func TestZone13PtrDeprecated(t *testing.T) {
	setupZone13(t)

	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 ptr -all"), nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_SPF_PTR_DEPRECATED") {
		t.Fatalf("expected Z13_SPF_PTR_DEPRECATED")
	}
	if !hasEntryTag(entries, "Z13_SPF_LOOKUP_COUNT_OK") {
		t.Fatalf("expected Z13_SPF_LOOKUP_COUNT_OK")
	}
}

func TestZone13NoSpfFound(t *testing.T) {
	setupZone13(t)

	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, "example.com.", dns.TypeTXT)
		msg.Authoritative = true
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_NO_SPF_FOUND") {
		t.Fatalf("expected Z13_NO_SPF_FOUND, got tags: %v", entryTags(entries))
	}
}

func TestZone13CustomLimit(t *testing.T) {
	setupZone13(t)

	profile.Effective().TestCasesVars.Zone13.SPFLookupLimit = 5

	// SPF with 6 mechanisms: a mx exists:x1 exists:x2 exists:x3 exists:x4
	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 a mx exists:x1.example.com exists:x2.example.com exists:x3.example.com exists:x4.example.com -all"), nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_SPF_LOOKUP_COUNT_EXCEEDED") {
		t.Fatalf("expected Z13_SPF_LOOKUP_COUNT_EXCEEDED with limit=5, got tags: %v", entryTags(entries))
	}
	for _, e := range entries {
		if e != nil && e.Tag == "Z13_SPF_LOOKUP_COUNT_EXCEEDED" {
			if limit, ok := e.Args["limit"].(int); !ok || limit != 5 {
				t.Fatalf("expected limit=5, got %v", e.Args["limit"])
			}
		}
	}
}

func TestZone13MacroInInclude(t *testing.T) {
	setupZone13(t)

	// Real-world pattern from ibm.com: the include target uses RFC 7208 §7 macros
	// that can only be expanded at SMTP time, so the audit cannot follow it.
	const macroTarget = "%{ir}.%{v}.%{d}.spf.has.pphosted.com"
	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 include:"+macroTarget+" -all"), nil
	}
	recurse = func(_ context.Context, _ *zonepkg.Zone, name string, _ string) (packet.Packet, error) {
		t.Fatalf("recurse should not be called for macro-laden target, got %q", name)
		return packet.Packet{}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_SPF_MACRO_TARGET") {
		t.Fatalf("expected Z13_SPF_MACRO_TARGET, got tags: %v", entryTags(entries))
	}
	if hasEntryTag(entries, "Z13_SPF_RECURSIVE_ERROR") {
		t.Fatalf("did not expect Z13_SPF_RECURSIVE_ERROR for a macro-laden target")
	}
	if !hasEntryTag(entries, "Z13_SPF_LOOKUP_COUNT_OK") {
		t.Fatalf("expected Z13_SPF_LOOKUP_COUNT_OK, got tags: %v", entryTags(entries))
	}
	for _, e := range entries {
		if e == nil {
			continue
		}
		switch e.Tag {
		case "Z13_SPF_MACRO_TARGET":
			if got, _ := e.Args["target"].(string); got != macroTarget {
				t.Fatalf("expected target=%q, got %v", macroTarget, e.Args["target"])
			}
			if got, _ := e.Args["domain"].(string); got != "example.com" {
				t.Fatalf("expected domain=example.com, got %v", e.Args["domain"])
			}
		case "Z13_SPF_LOOKUP_COUNT_OK":
			// The include itself still counts +1 even though we cannot follow it.
			if count, ok := e.Args["count"].(int); !ok || count != 1 {
				t.Fatalf("expected count=1 (include +1, no recursion), got %v", e.Args["count"])
			}
		}
	}
}

func TestZone13MacroInRedirect(t *testing.T) {
	setupZone13(t)

	const macroTarget = "%{l1r+}.%{d}._spf.example.net"
	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 redirect="+macroTarget), nil
	}
	recurse = func(_ context.Context, _ *zonepkg.Zone, name string, _ string) (packet.Packet, error) {
		t.Fatalf("recurse should not be called for macro-laden redirect, got %q", name)
		return packet.Packet{}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_SPF_MACRO_TARGET") {
		t.Fatalf("expected Z13_SPF_MACRO_TARGET, got tags: %v", entryTags(entries))
	}
	if hasEntryTag(entries, "Z13_SPF_RECURSIVE_ERROR") {
		t.Fatalf("did not expect Z13_SPF_RECURSIVE_ERROR for a macro-laden redirect")
	}
	for _, e := range entries {
		if e != nil && e.Tag == "Z13_SPF_MACRO_TARGET" {
			if got, _ := e.Args["target"].(string); got != macroTarget {
				t.Fatalf("expected target=%q, got %v", macroTarget, e.Args["target"])
			}
		}
	}
}

func TestZone13MacroAndResolvableIncludeCoexist(t *testing.T) {
	setupZone13(t)

	// One macro-laden include (counted, not followed) plus one normal include
	// (counted and followed for nested lookups). Expected count: 1 (macro) + 1 (real include) + 2 (a, mx inside) = 4.
	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 include:%{ir}.spf.pphosted.com include:real.example.org -all"), nil
	}
	recurse = func(_ context.Context, _ *zonepkg.Zone, name string, _ string) (packet.Packet, error) {
		if strings.Contains(name, "%") {
			t.Fatalf("recurse must not be called for macro target, got %q", name)
		}
		if strings.HasPrefix(name, "real.example.org") {
			return recurseTxtPacket("real.example.org", "v=spf1 a mx -all"), nil
		}
		return packet.Packet{}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_SPF_MACRO_TARGET") {
		t.Fatalf("expected Z13_SPF_MACRO_TARGET, got tags: %v", entryTags(entries))
	}
	if !hasEntryTag(entries, "Z13_SPF_LOOKUP_COUNT_OK") {
		t.Fatalf("expected Z13_SPF_LOOKUP_COUNT_OK, got tags: %v", entryTags(entries))
	}
	for _, e := range entries {
		if e != nil && e.Tag == "Z13_SPF_LOOKUP_COUNT_OK" {
			if count, ok := e.Args["count"].(int); !ok || count != 4 {
				t.Fatalf("expected count=4 (macro+1, include+1, a+1, mx+1), got %v", e.Args["count"])
			}
		}
	}
}

// --- Zone14 tests ---

type zonemdRecord struct {
	serial uint32
	scheme uint8
	hash   uint8
	digest string
}

func zonemdPacket(name string, records []zonemdRecord) packet.Packet {
	msg := new(dns.Msg)
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	for _, r := range records {
		zm := &dns.ZONEMD{}
		zm.Hdr = dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 300}
		zm.ZONEMD.Serial = r.serial
		zm.ZONEMD.Scheme = r.scheme
		zm.ZONEMD.Hash = r.hash
		zm.ZONEMD.Digest = r.digest
		msg.Answer = append(msg.Answer, zm)
	}
	return packet.Packet{Msg: msg}
}

func TestZone14ZONEMDFound(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	const serial uint32 = 2024010101
	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "ZONEMD":
			return zonemdPacket("example", []zonemdRecord{{serial: serial, scheme: 1, hash: 1, digest: "aabbcc"}})
		case "SOA":
			return soaPacket("example", serial, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_ZONEMD_FOUND") {
		t.Fatalf("expected Z14_ZONEMD_FOUND")
	}
	if hasEntryTag(entries, "Z14_SERIAL_MISMATCH") {
		t.Fatalf("did not expect Z14_SERIAL_MISMATCH when serials match")
	}
}

func TestZone14NoZONEMD(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_NO_ZONEMD") {
		t.Fatalf("expected Z14_NO_ZONEMD")
	}
}

func TestZone14SerialMismatch(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "ZONEMD":
			return zonemdPacket("example", []zonemdRecord{{serial: 100, scheme: 1, hash: 1, digest: "aabbcc"}})
		case "SOA":
			return soaPacket("example", 200, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_SERIAL_MISMATCH") {
		t.Fatalf("expected Z14_SERIAL_MISMATCH")
	}
}

func TestZone14DuplicateSchemeHash(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{
				{serial: 2024010101, scheme: 1, hash: 1, digest: "aabbcc"},
				{serial: 2024010101, scheme: 1, hash: 1, digest: "ddeeff"},
			})
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_DUPLICATE_SCHEME_HASH") {
		t.Fatalf("expected Z14_DUPLICATE_SCHEME_HASH")
	}
}

func TestZone14MultipleZONEMD(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{
				{serial: 2024010101, scheme: 1, hash: 1, digest: "aabbcc"},
				{serial: 2024010101, scheme: 1, hash: 2, digest: "ddeeff"},
			})
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	var foundCount int
	for _, e := range entries {
		if e != nil && e.Tag == "Z14_ZONEMD_FOUND" {
			foundCount++
		}
	}
	if foundCount != 2 {
		t.Fatalf("expected 2 Z14_ZONEMD_FOUND, got %d", foundCount)
	}
	if hasEntryTag(entries, "Z14_DUPLICATE_SCHEME_HASH") {
		t.Fatalf("did not expect Z14_DUPLICATE_SCHEME_HASH for distinct (scheme, hash) pairs")
	}
}

func TestZone14InconsistentZONEMD(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 2024010101, scheme: 1, hash: 1, digest: "aabbcc"}})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 2024010101, scheme: 1, hash: 1, digest: "112233"}})
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_INCONSISTENT_ZONEMD") {
		t.Fatalf("expected Z14_INCONSISTENT_ZONEMD")
	}
}

func TestZone14MixedPresence(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 2024010101, scheme: 1, hash: 1, digest: "aabbcc"}})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_MIXED_PRESENCE") {
		t.Fatalf("expected Z14_MIXED_PRESENCE")
	}
}

func TestZone14ConsolidatedFound(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	rec := zonemdRecord{serial: 2024010101, scheme: 1, hash: 1, digest: "aabbcc"}
	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{rec})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{rec})
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	var foundEntries []*logger.Entry
	for _, e := range entries {
		if e != nil && e.Tag == "Z14_ZONEMD_FOUND" {
			foundEntries = append(foundEntries, e)
		}
	}
	if len(foundEntries) != 1 {
		t.Fatalf("expected exactly 1 consolidated Z14_ZONEMD_FOUND, got %d", len(foundEntries))
	}
	if hasEntryTag(entries, "Z14_INCONSISTENT_ZONEMD") {
		t.Fatalf("did not expect Z14_INCONSISTENT_ZONEMD when content is identical")
	}
}

func TestZone14NonAuthoritativeSkipped(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			// NOERROR but AA not set.
			msg := new(dns.Msg)
			msg.Authoritative = false
			msg.Rcode = dns.RcodeSuccess
			zm := &dns.ZONEMD{}
			zm.Hdr = dns.Header{Name: "example.", Class: dns.ClassINET, TTL: 300}
			zm.ZONEMD.Serial = 2024010101
			zm.ZONEMD.Scheme = 1
			zm.ZONEMD.Hash = 1
			zm.ZONEMD.Digest = "aabbcc"
			msg.Answer = []dns.RR{zm}
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	for _, e := range entries {
		if e != nil && strings.HasPrefix(e.Tag, "Z14_") {
			t.Fatalf("expected no Z14_* tags for non-authoritative response, got %s", e.Tag)
		}
	}
}

func TestZone14UnsupportedHash(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 2024010101, scheme: 1, hash: 99, digest: "aabbcc"}})
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_UNSUPPORTED_HASH") {
		t.Fatalf("expected Z14_UNSUPPORTED_HASH for hash=99")
	}
	if !hasEntryTag(entries, "Z14_ZONEMD_FOUND") {
		t.Fatalf("expected Z14_ZONEMD_FOUND alongside Z14_UNSUPPORTED_HASH")
	}
}

func TestZone14UnsupportedHashConsolidated(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	// Two ZONEMD records with the same unsupported hash but different schemes.
	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{
				{serial: 2024010101, scheme: 1, hash: 99, digest: "aabbcc"},
				{serial: 2024010101, scheme: 2, hash: 99, digest: "ddeeff"},
			})
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	var unsupportedCount int
	for _, e := range entries {
		if e != nil && e.Tag == "Z14_UNSUPPORTED_HASH" {
			unsupportedCount++
		}
	}
	if unsupportedCount != 1 {
		t.Fatalf("expected exactly 1 Z14_UNSUPPORTED_HASH for same hash value, got %d", unsupportedCount)
	}
}

func TestZone14SOAUnavailable(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 100, scheme: 1, hash: 1, digest: "aabbcc"}})
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_ZONEMD_FOUND") {
		t.Fatalf("expected Z14_ZONEMD_FOUND")
	}
	if hasEntryTag(entries, "Z14_SERIAL_MISMATCH") {
		t.Fatalf("did not expect Z14_SERIAL_MISMATCH when SOA is unavailable")
	}
}

func TestZone14MixedPresenceAndInconsistent(t *testing.T) {
	setupTest(t)

	origMethod4and5 := method4and5
	t.Cleanup(func() { method4and5 = origMethod4and5 })

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 2024010101, scheme: 1, hash: 1, digest: "aabbcc"}})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 2024010101, scheme: 1, hash: 1, digest: "112233"}})
		}
		return packet.Packet{}
	})
	ns3 := newNameserver(t, "ns3.example", "192.0.2.3", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	method4and5 = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2, ns3}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(context.Background(), &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_MIXED_PRESENCE") {
		t.Fatalf("expected Z14_MIXED_PRESENCE")
	}
	if !hasEntryTag(entries, "Z14_INCONSISTENT_ZONEMD") {
		t.Fatalf("expected Z14_INCONSISTENT_ZONEMD")
	}
}

func entryTags(entries []*logger.Entry) []string {
	var tags []string
	for _, e := range entries {
		if e != nil {
			tags = append(tags, e.Tag)
		}
	}
	return tags
}
