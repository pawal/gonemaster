package zone

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"

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
	for _, entry := range entries {
		if entry == nil || entry.Tag != "NO_RESPONSE" {
			continue
		}
		if ns, ok := entry.Args["ns"].(string); ok {
			order = append(order, ns)
		}
	}
	if len(order) != 2 {
		t.Fatalf("expected 2 no-response entries, got %v", order)
	}
	if order[0] != "ns1.example/192.0.2.1" || order[1] != "ns2.example/192.0.2.2" {
		t.Fatalf("expected deterministic log order, got %v", order)
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
			msg.SetQuestion(dns.Fqdn("example"), dns.TypeMX)
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
	msg.SetQuestion(dns.Fqdn(owner), dns.TypeSOA)
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.SOA{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeSOA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns:      dns.Fqdn("ns1.example"),
			Mbox:    dns.Fqdn("hostmaster.example"),
			Serial:  serial,
			Refresh: refresh,
			Retry:   retry,
			Expire:  expire,
			Minttl:  minimum,
		},
	}
	return packet.Packet{Msg: msg}
}

func txtPacket(name string, value string) packet.Packet {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(name), dns.TypeTXT)
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.TXT{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(name),
				Rrtype: dns.TypeTXT,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Txt: []string{value},
		},
	}
	return packet.Packet{Msg: msg}
}
