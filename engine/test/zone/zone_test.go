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
	ens "codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/util"
	zonepkg "codeberg.org/pawal/gonemaster/engine/zone"
)

func TestZone02RefreshBelowMinimum(t *testing.T) {
	ctx := setupTest(t)

	origMethod5 := apexNameservers
	t.Cleanup(func() { apexNameservers = origMethod5 })

	profile.Effective().TestCasesVars.Zone02.SOARefreshMinimumValue = 1000

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, _ string, _ *ens.QueryOptions) packet.Packet {
		return soaPacket("example", 1, 500, 50, 7200, 60)
	})
	apexNameservers = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone02(ctx, &z)
	if err != nil {
		t.Fatalf("zone02: %v", err)
	}
	if !hasEntryTag(entries, "REFRESH_MINIMUM_VALUE_LOWER") {
		t.Fatalf("expected REFRESH_MINIMUM_VALUE_LOWER")
	}
}

func TestZone05ExpireLowerThanRefreshAndMinimum(t *testing.T) {
	ctx := setupTest(t)

	origMethod5 := apexNameservers
	t.Cleanup(func() { apexNameservers = origMethod5 })

	profile.Effective().TestCasesVars.Zone05.SOAExpireMinimumValue = 2000

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.2", func(_ string, _ string, _ *ens.QueryOptions) packet.Packet {
		return soaPacket("example", 1, 1500, 100, 1000, 60)
	})
	apexNameservers = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone05(ctx, &z)
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
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

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

	ns1, err := ens.NewWithContext(ctx, "ns1.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns1.SetQueryHook(hook("ns1"))

	ns2, err := ens.NewWithContext(ctx, "ns2.example", "192.0.2.2", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns2.SetQueryHook(hook("ns2"))

	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
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
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone10(ctx, &z)
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

func TestZone10ApexCNAME(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return soaPacket("example", 1, 1, 1, 1, 1)
		case "CNAME":
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			cname := &dns.CNAME{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET, TTL: 300}}
			cname.Target = "other.example."
			msg.Answer = []dns.RR{cname}
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone10(ctx, &z)
	if err != nil {
		t.Fatalf("zone10: %v", err)
	}
	if !hasEntryTag(entries, "SOA_AND_CNAME") {
		t.Fatalf("expected SOA_AND_CNAME, got %v", entryTags(entries))
	}
	if hasEntryTag(entries, "APEX_DNAME") {
		t.Fatalf("unexpected APEX_DNAME")
	}
}

func TestZone10ApexDNAME(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return soaPacket("example", 1, 1, 1, 1, 1)
		case "DNAME":
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			dname := &dns.DNAME{}
			dname.Hdr = dns.Header{Name: "example.", Class: dns.ClassINET, TTL: 300}
			dname.Target = "other.example."
			msg.Answer = []dns.RR{dname}
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone10(ctx, &z)
	if err != nil {
		t.Fatalf("zone10: %v", err)
	}
	if !hasEntryTag(entries, "APEX_DNAME") {
		t.Fatalf("expected APEX_DNAME, got %v", entryTags(entries))
	}
	if hasEntryTag(entries, "SOA_AND_CNAME") {
		t.Fatalf("unexpected SOA_AND_CNAME")
	}
}

func TestZone10ApexDNAMEAndCNAME(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return soaPacket("example", 1, 1, 1, 1, 1)
		case "CNAME":
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			cname := &dns.CNAME{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET, TTL: 300}}
			cname.Target = "other.example."
			msg.Answer = []dns.RR{cname}
			return packet.Packet{Msg: msg}
		case "DNAME":
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			dname := &dns.DNAME{}
			dname.Hdr = dns.Header{Name: "example.", Class: dns.ClassINET, TTL: 300}
			dname.Target = "other.example."
			msg.Answer = []dns.RR{dname}
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone10(ctx, &z)
	if err != nil {
		t.Fatalf("zone10: %v", err)
	}
	if !hasEntryTag(entries, "SOA_AND_CNAME") {
		t.Fatalf("expected SOA_AND_CNAME for DNAME+CNAME collision, got %v", entryTags(entries))
	}
	if !hasEntryTag(entries, "APEX_DNAME") {
		t.Fatalf("expected APEX_DNAME for DNAME+CNAME collision, got %v", entryTags(entries))
	}
}

func TestZone10CleanApex(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "SOA" {
			return soaPacket("example", 1, 1, 1, 1, 1)
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone10(ctx, &z)
	if err != nil {
		t.Fatalf("zone10: %v", err)
	}
	if hasEntryTag(entries, "SOA_AND_CNAME") {
		t.Fatalf("unexpected SOA_AND_CNAME on clean apex")
	}
	if hasEntryTag(entries, "APEX_DNAME") {
		t.Fatalf("unexpected APEX_DNAME on clean apex")
	}
	if !hasEntryTag(entries, "ONE_SOA") {
		t.Fatalf("expected ONE_SOA on clean apex")
	}
}

func TestZone09MXQueryDisablesFallback(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	profile.Effective().Resolver.Defaults.Parallel = 1

	var mu sync.Mutex
	var fallbackValues []bool
	var useVCValues []bool
	mxCalls := 0

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, opts *ens.QueryOptions) packet.Packet {
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

	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	if _, err := Zone09(ctx, &z); err != nil {
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
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
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

	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone09(ctx, &z)
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
	// Z09_MX_DATA now reports name servers by host name and IP (servers),
	// not IP addresses alone, so the addresses key must be gone.
	if _, ok := mxData.Args["addresses"]; ok {
		t.Fatalf("Z09_MX_DATA should not emit addresses anymore: %#v", mxData.Args)
	}
	servers, ok := mxData.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one typed server, got %#v", mxData.Args["servers"])
	}
	if addr, _ := servers[0]["address"].(string); addr != "192.0.2.1" {
		t.Fatalf("expected server address 192.0.2.1, got %#v", servers[0])
	}
	if ns, _ := servers[0]["ns"].(string); ns == "" {
		t.Fatalf("expected server host name to be present, got %#v", servers[0])
	}
	if _, ok := mxData.Args["mailtarget_list"]; ok {
		t.Fatalf("legacy key mailtarget_list should not be present: %#v", mxData.Args)
	}
}

// mxRR is a single MX record (preference + mail target) for mxPacket.
type mxRR struct {
	pref   uint16
	target string
}

// mxPacket builds an authoritative MX answer for owner at the given TTL.
func mxPacket(owner string, ttl uint32, rrs ...mxRR) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(owner), dns.TypeMX)
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	for _, r := range rrs {
		mx := &dns.MX{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: ttl}}
		mx.Mx = dnsutil.Fqdn(r.target)
		mx.Preference = r.pref
		msg.Answer = append(msg.Answer, mx)
	}
	return packet.Packet{Msg: msg}
}

// noMXPacket builds an authoritative NOERROR response with no MX records.
func noMXPacket(owner string) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(owner), dns.TypeMX)
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	return packet.Packet{Msg: msg}
}

// TestEncodeMXRRSetRDATA verifies the MX consistency key: it is built from
// RDATA (preference + mail target) only, is independent of TTL and record
// order, is case-insensitive on the target, and distinguishes genuine RDATA
// differences (preference or target).
func TestEncodeMXRRSetRDATA(t *testing.T) {
	mk := func(ttl uint32, pref uint16, target string) dns.RR {
		mx := &dns.MX{Hdr: dns.Header{Name: "example.com.", Class: dns.ClassINET, TTL: ttl}}
		mx.Mx = target
		mx.Preference = pref
		return mx
	}

	base := []dns.RR{mk(3600, 10, "mx1.example."), mk(3600, 20, "mx2.example.")}

	// TTL is not part of the key.
	ttlDiff := []dns.RR{mk(300, 10, "mx1.example."), mk(300, 20, "mx2.example.")}
	if encodeMXRRSetRDATA(base) != encodeMXRRSetRDATA(ttlDiff) {
		t.Errorf("TTL difference must not change the key")
	}

	// Record order is not part of the key.
	reordered := []dns.RR{mk(3600, 20, "mx2.example."), mk(3600, 10, "mx1.example.")}
	if encodeMXRRSetRDATA(base) != encodeMXRRSetRDATA(reordered) {
		t.Errorf("record order must not change the key")
	}

	// Target comparison is case-insensitive.
	mixedCase := []dns.RR{mk(3600, 10, "MX1.Example."), mk(3600, 20, "mx2.EXAMPLE.")}
	if encodeMXRRSetRDATA(base) != encodeMXRRSetRDATA(mixedCase) {
		t.Errorf("target case must not change the key")
	}

	// A different preference is a real difference.
	prefDiff := []dns.RR{mk(3600, 15, "mx1.example."), mk(3600, 20, "mx2.example.")}
	if encodeMXRRSetRDATA(base) == encodeMXRRSetRDATA(prefDiff) {
		t.Errorf("preference difference must change the key")
	}

	// A different mail target is a real difference.
	targetDiff := []dns.RR{mk(3600, 10, "mx9.example."), mk(3600, 20, "mx2.example.")}
	if encodeMXRRSetRDATA(base) == encodeMXRRSetRDATA(targetDiff) {
		t.Errorf("target difference must change the key")
	}
}

// TestZone09MXConsistentDespiteTTLDifference verifies that two name servers
// serving identical MX RDATA (preference + target) but different TTLs are
// treated as consistent: no Z09_INCONSISTENT_MX_DATA, one Z09_MX_DATA. TTL is
// not RRset data and must not drive the consistency comparison.
func TestZone09MXConsistentDespiteTTLDifference(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	handler := func(ttl uint32) func(string, string, *ens.QueryOptions) packet.Packet {
		return func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
			switch qtype {
			case "SOA":
				return soaPacket("example.com", 1, 1, 1, 1, 1)
			case "MX":
				return mxPacket("example.com", ttl, mxRR{10, "mail.example."})
			default:
				return packet.Packet{}
			}
		}
	}
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", handler(300))
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", handler(3600))
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone09(ctx, &z)
	if err != nil {
		t.Fatalf("zone09: %v", err)
	}

	var mxData *logger.Entry
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Tag == "Z09_INCONSISTENT_MX_DATA" {
			t.Fatalf("TTL-only difference must not be reported as inconsistent MX data")
		}
		if entry.Tag == "Z09_MX_DATA" {
			mxData = entry
		}
	}
	if mxData == nil {
		t.Fatalf("expected a single consistent Z09_MX_DATA")
	}
	servers, ok := mxData.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("expected both name servers in Z09_MX_DATA, got %#v", mxData.Args["servers"])
	}
}

// TestZone09MXInconsistentDataPerVariant verifies that differing MX RDATA
// yields one self-contained Z09_INCONSISTENT_MX_DATA per RDATA variant, each
// carrying its own servers (host name + IP) and mail targets, and no
// Z09_MX_DATA.
func TestZone09MXInconsistentDataPerVariant(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	handler := func(target string) func(string, string, *ens.QueryOptions) packet.Packet {
		return func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
			switch qtype {
			case "SOA":
				return soaPacket("example.com", 1, 1, 1, 1, 1)
			case "MX":
				return mxPacket("example.com", 300, mxRR{10, target})
			default:
				return packet.Packet{}
			}
		}
	}
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", handler("mail1.example."))
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", handler("mail2.example."))
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone09(ctx, &z)
	if err != nil {
		t.Fatalf("zone09: %v", err)
	}

	// Map each variant's mail target to the addresses that returned it.
	byTarget := map[string][]string{}
	inconsistent := 0
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Tag == "Z09_MX_DATA" {
			t.Fatalf("inconsistent branch must not emit Z09_MX_DATA")
		}
		if entry.Tag != "Z09_INCONSISTENT_MX_DATA" {
			continue
		}
		inconsistent++
		targets, ok := entry.Args["mail_targets"].([]string)
		if !ok || len(targets) != 1 {
			t.Fatalf("expected one mail target per variant, got %#v", entry.Args["mail_targets"])
		}
		servers, ok := entry.Args["servers"].([]map[string]any)
		if !ok || len(servers) != 1 {
			t.Fatalf("expected one server per variant, got %#v", entry.Args["servers"])
		}
		if ns, _ := servers[0]["ns"].(string); ns == "" {
			t.Fatalf("expected server host name in Z09_INCONSISTENT_MX_DATA, got %#v", servers[0])
		}
		addr, _ := servers[0]["address"].(string)
		byTarget[targets[0]] = append(byTarget[targets[0]], addr)
	}

	if inconsistent != 2 {
		t.Fatalf("expected 2 Z09_INCONSISTENT_MX_DATA entries (one per variant), got %d", inconsistent)
	}
	if got := byTarget["mail1.example"]; len(got) != 1 || got[0] != "192.0.2.1" {
		t.Fatalf("expected mail1.example from 192.0.2.1, got %#v", got)
	}
	if got := byTarget["mail2.example"]; len(got) != 1 || got[0] != "192.0.2.2" {
		t.Fatalf("expected mail2.example from 192.0.2.2, got %#v", got)
	}
}

// TestZone09NonAuthMXResponseReportsResponders verifies Z09_NON_AUTH_MX_RESPONSE
// reports the name servers that actually returned a non-authoritative MX
// response, not the (here empty) no-response set.
func TestZone09NonAuthMXResponseReportsResponders(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return soaPacket("example.com", 1, 1, 1, 1, 1)
		case "MX":
			msg := new(dns.Msg)
			dnsutil.SetQuestion(msg, dnsutil.Fqdn("example.com"), dns.TypeMX)
			msg.Authoritative = false
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}
		default:
			return packet.Packet{}
		}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone09(ctx, &z)
	if err != nil {
		t.Fatalf("zone09: %v", err)
	}

	var nonAuth *logger.Entry
	for _, entry := range entries {
		if entry != nil && entry.Tag == "Z09_NON_AUTH_MX_RESPONSE" {
			nonAuth = entry
			break
		}
	}
	if nonAuth == nil {
		t.Fatalf("expected Z09_NON_AUTH_MX_RESPONSE")
	}
	addrs, ok := nonAuth.Args["addresses"].([]string)
	if !ok || len(addrs) != 1 || addrs[0] != "192.0.2.1" {
		t.Fatalf("expected non-authoritative responder [192.0.2.1], got %#v", nonAuth.Args["addresses"])
	}
}

func TestZone11SpfSyntaxError(t *testing.T) {
	ctx := setupTest(t)

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	z, err := zonepkg.New("example.com")
	if err != nil {
		t.Fatalf("zone11: %v", err)
	}

	newNameserver(t, ctx, "ns1.example.com", "192.0.2.10", func(qname string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype != "TXT" {
			return packet.Packet{}
		}
		return txtPacket(qname, "v=spf1 amx-all")
	})

	delegationNameservers = func(_ context.Context, _ *zonepkg.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name:       dnsname.New("ns1.example.com"),
			Address:    netip.MustParseAddr("192.0.2.10"),
			HasAddress: true,
		}}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zonepkg.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	}

	entries, err := Zone11(ctx, &z)
	if err != nil {
		t.Fatalf("zone11: %v", err)
	}
	if !hasEntryTag(entries, "Z11_SPF_SYNTAX_ERROR") {
		t.Fatalf("expected Z11_SPF_SYNTAX_ERROR")
	}
}

func TestZone11NoSpfNonMailDomain(t *testing.T) {
	ctx := setupTest(t)

	origDel := delegationNameservers
	origZone := zoneNameservers
	t.Cleanup(func() {
		delegationNameservers = origDel
		zoneNameservers = origZone
	})

	z, err := zonepkg.New("se")
	if err != nil {
		t.Fatalf("zone11: %v", err)
	}

	newNameserver(t, ctx, "ns1.se", "192.0.2.10", func(qname string, qtype string, _ *ens.QueryOptions) packet.Packet {
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, dnsutil.Fqdn(qname), dns.TypeTXT)
		msg.Authoritative = true
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}
	})

	delegationNameservers = func(_ context.Context, _ *zonepkg.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{{
			Name:       dnsname.New("ns1.se"),
			Address:    netip.MustParseAddr("192.0.2.10"),
			HasAddress: true,
		}}, nil
	}
	zoneNameservers = func(_ context.Context, _ *zonepkg.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	}

	entries, err := Zone11(ctx, &z)
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

func setupTest(t *testing.T) context.Context {
	t.Helper()

	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })
	return ens.WithCache(context.Background(), ens.NewCacheStore())
}

func newNameserver(t *testing.T, ctx context.Context, name string, ip string, handler func(qname string, qtype string, opts *ens.QueryOptions) packet.Packet) ens.Nameserver {
	t.Helper()

	ns, err := ens.NewWithContext(ctx, name, ip, nil)
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
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	const serial uint32 = 2024010101
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "CSYNC":
			return csyncPacket("example", serial, 0x0001, []uint16{dns.TypeNS, dns.TypeA, dns.TypeAAAA})
		case "SOA":
			return soaPacket("example", serial, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(ctx, &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_CSYNC_FOUND") {
		t.Fatalf("expected Z12_CSYNC_FOUND")
	}
}

func TestZone12NoCSYNC(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
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
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(ctx, &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_NO_CSYNC") {
		t.Fatalf("expected Z12_NO_CSYNC")
	}
}

func TestZone12SerialMismatch(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "CSYNC":
			// CSYNC carries serial 100, but SOA has serial 200.
			return csyncPacket("example", 100, 0, []uint16{dns.TypeNS})
		case "SOA":
			return soaPacket("example", 200, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(ctx, &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_SERIAL_MISMATCH") {
		t.Fatalf("expected Z12_SERIAL_MISMATCH")
	}
}

func TestZone12SerialMismatchSoaMinimumNewerCSYNC(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "CSYNC":
			// soaminimum set, but CSYNC serial is newer than current SOA serial.
			return csyncPacket("example", 300, csyncFlagSoaMinimum, []uint16{dns.TypeNS})
		case "SOA":
			return soaPacket("example", 200, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(ctx, &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_SERIAL_MISMATCH") {
		t.Fatalf("expected Z12_SERIAL_MISMATCH")
	}
}

func TestZone12SerialMismatchSoaMinimumOlderCSYNCNotMismatch(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "CSYNC":
			// soaminimum set and SOA serial has advanced, which is valid.
			return csyncPacket("example", 100, csyncFlagSoaMinimum, []uint16{dns.TypeNS})
		case "SOA":
			return soaPacket("example", 200, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(ctx, &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if hasEntryTag(entries, "Z12_SERIAL_MISMATCH") {
		t.Fatalf("did not expect Z12_SERIAL_MISMATCH")
	}
}

func TestZone12MultipleCSYNC(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
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
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(ctx, &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_MULTIPLE_CSYNC") {
		t.Fatalf("expected Z12_MULTIPLE_CSYNC")
	}
}

func TestZone12InconsistentCSYNC(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	// ns1 and ns2 return CSYNC records with different serials.
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "CSYNC" {
			return csyncPacket("example", 2024010101, 0, []uint16{dns.TypeNS})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "CSYNC" {
			return csyncPacket("example", 2024010199, 0, []uint16{dns.TypeNS})
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(ctx, &z)
	if err != nil {
		t.Fatalf("zone12: %v", err)
	}
	if !hasEntryTag(entries, "Z12_INCONSISTENT_CSYNC") {
		t.Fatalf("expected Z12_INCONSISTENT_CSYNC")
	}
}

func TestZone12MixedPresence(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	// ns1 returns CSYNC, ns2 returns authoritative NOERROR without CSYNC.
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "CSYNC" {
			return csyncPacket("example", 2024010101, 0, []uint16{dns.TypeNS})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "CSYNC" {
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone12(ctx, &z)
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

func setupZone13(t *testing.T) context.Context {
	t.Helper()
	ctx := setupTest(t)
	origQueryAuth := queryAuth
	origRecurse := recurse
	t.Cleanup(func() {
		queryAuth = origQueryAuth
		recurse = origRecurse
	})
	return ctx
}

func TestZone13LookupCountOK_NoLookups(t *testing.T) {
	ctx := setupZone13(t)

	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 ip4:192.0.2.0/24 ip6:2001:db8::/32 -all"), nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(ctx, &z)
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
	ctx := setupZone13(t)

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
	entries, err := Zone13(ctx, &z)
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
	ctx := setupZone13(t)

	// SPF with 11 mechanisms: a mx ptr exists:x1 ... exists:x8
	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 a mx ptr exists:x1.example.com exists:x2.example.com exists:x3.example.com exists:x4.example.com exists:x5.example.com exists:x6.example.com exists:x7.example.com exists:x8.example.com -all"), nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(ctx, &z)
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
	ctx := setupZone13(t)

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
	entries, err := Zone13(ctx, &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_SPF_LOOKUP_LOOP") {
		t.Fatalf("expected Z13_SPF_LOOKUP_LOOP, got tags: %v", entryTags(entries))
	}
}

func TestZone13RecursiveError(t *testing.T) {
	ctx := setupZone13(t)

	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 include:missing.example.com -all"), nil
	}
	recurse = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return packet.Packet{}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(ctx, &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_SPF_RECURSIVE_ERROR") {
		t.Fatalf("expected Z13_SPF_RECURSIVE_ERROR, got tags: %v", entryTags(entries))
	}
}

func TestZone13PtrDeprecated(t *testing.T) {
	ctx := setupZone13(t)

	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 ptr -all"), nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(ctx, &z)
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
	ctx := setupZone13(t)

	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, "example.com.", dns.TypeTXT)
		msg.Authoritative = true
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(ctx, &z)
	if err != nil {
		t.Fatalf("zone13: %v", err)
	}
	if !hasEntryTag(entries, "Z13_NO_SPF_FOUND") {
		t.Fatalf("expected Z13_NO_SPF_FOUND, got tags: %v", entryTags(entries))
	}
}

func TestZone13CustomLimit(t *testing.T) {
	ctx := setupZone13(t)

	profile.Effective().TestCasesVars.Zone13.SPFLookupLimit = 5

	// SPF with 6 mechanisms: a mx exists:x1 exists:x2 exists:x3 exists:x4
	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 a mx exists:x1.example.com exists:x2.example.com exists:x3.example.com exists:x4.example.com -all"), nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(ctx, &z)
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
	ctx := setupZone13(t)

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
	entries, err := Zone13(ctx, &z)
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
	ctx := setupZone13(t)

	const macroTarget = "%{l1r+}.%{d}._spf.example.net"
	queryAuth = func(_ context.Context, _ *zonepkg.Zone, _ string, _ string) (packet.Packet, error) {
		return spfTxtPacket("example.com", "v=spf1 redirect="+macroTarget), nil
	}
	recurse = func(_ context.Context, _ *zonepkg.Zone, name string, _ string) (packet.Packet, error) {
		t.Fatalf("recurse should not be called for macro-laden redirect, got %q", name)
		return packet.Packet{}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone13(ctx, &z)
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
	ctx := setupZone13(t)

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
	entries, err := Zone13(ctx, &z)
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
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	const serial uint32 = 2024010101
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "ZONEMD":
			return zonemdPacket("example", []zonemdRecord{{serial: serial, scheme: 1, hash: 1, digest: "aabbcc"}})
		case "SOA":
			return soaPacket("example", serial, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
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
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_NO_ZONEMD") {
		t.Fatalf("expected Z14_NO_ZONEMD")
	}
}

func TestZone14SerialMismatch(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "ZONEMD":
			return zonemdPacket("example", []zonemdRecord{{serial: 100, scheme: 1, hash: 1, digest: "aabbcc"}})
		case "SOA":
			return soaPacket("example", 200, 3600, 900, 604800, 300)
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_SERIAL_MISMATCH") {
		t.Fatalf("expected Z14_SERIAL_MISMATCH")
	}
}

func TestZone14DuplicateSchemeHash(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{
				{serial: 2024010101, scheme: 1, hash: 1, digest: "aabbcc"},
				{serial: 2024010101, scheme: 1, hash: 1, digest: "ddeeff"},
			})
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_DUPLICATE_SCHEME_HASH") {
		t.Fatalf("expected Z14_DUPLICATE_SCHEME_HASH")
	}
}

func TestZone14MultipleZONEMD(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{
				{serial: 2024010101, scheme: 1, hash: 1, digest: "aabbcc"},
				{serial: 2024010101, scheme: 1, hash: 2, digest: "ddeeff"},
			})
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
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
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 2024010101, scheme: 1, hash: 1, digest: "aabbcc"}})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 2024010101, scheme: 1, hash: 1, digest: "112233"}})
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_INCONSISTENT_ZONEMD") {
		t.Fatalf("expected Z14_INCONSISTENT_ZONEMD")
	}
}

func TestZone14MixedPresence(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 2024010101, scheme: 1, hash: 1, digest: "aabbcc"}})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
	if err != nil {
		t.Fatalf("zone14: %v", err)
	}
	if !hasEntryTag(entries, "Z14_MIXED_PRESENCE") {
		t.Fatalf("expected Z14_MIXED_PRESENCE")
	}
}

func TestZone14ConsolidatedFound(t *testing.T) {
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	rec := zonemdRecord{serial: 2024010101, scheme: 1, hash: 1, digest: "aabbcc"}
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{rec})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{rec})
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
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
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
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
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
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
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 2024010101, scheme: 1, hash: 99, digest: "aabbcc"}})
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
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
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	// Two ZONEMD records with the same unsupported hash but different schemes.
	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{
				{serial: 2024010101, scheme: 1, hash: 99, digest: "aabbcc"},
				{serial: 2024010101, scheme: 2, hash: 99, digest: "ddeeff"},
			})
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
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
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 100, scheme: 1, hash: 1, digest: "aabbcc"}})
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
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
	ctx := setupTest(t)

	origMethod4and5 := authoritativeNS
	t.Cleanup(func() { authoritativeNS = origMethod4and5 })

	ns1 := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 2024010101, scheme: 1, hash: 1, digest: "aabbcc"}})
		}
		return packet.Packet{}
	})
	ns2 := newNameserver(t, ctx, "ns2.example", "192.0.2.2", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			return zonemdPacket("example", []zonemdRecord{{serial: 2024010101, scheme: 1, hash: 1, digest: "112233"}})
		}
		return packet.Packet{}
	})
	ns3 := newNameserver(t, ctx, "ns3.example", "192.0.2.3", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "ZONEMD" {
			msg := new(dns.Msg)
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}
		}
		return packet.Packet{}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns1, ns2, ns3}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example")}
	entries, err := Zone14(ctx, &z)
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

// findEntryByTag returns the first entry with the given tag, or nil.
func findEntryByTag(entries []*logger.Entry, tag string) *logger.Entry {
	for _, e := range entries {
		if e != nil && e.Tag == tag {
			return e
		}
	}
	return nil
}

// runZone09 wires a single-nameserver zone09 run for a given zone name and MX
// handler, and returns the emitted entries.
func runZone09(t *testing.T, name string, mx func() packet.Packet) []*logger.Entry {
	t.Helper()
	ctx := setupTest(t)

	orig := authoritativeNS
	t.Cleanup(func() { authoritativeNS = orig })

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "SOA":
			return soaPacket(name, 1, 1, 1, 1, 1)
		case "MX":
			return mx()
		default:
			return packet.Packet{}
		}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New(name)}
	entries, err := Zone09(ctx, &z)
	if err != nil {
		t.Fatalf("zone09: %v", err)
	}
	return entries
}

// TestZone09ArpaEmailDomain verifies that a zone in the .arpa tree carrying a
// non-null MX is flagged as an unexpected mail domain, not reported as ordinary
// MX data.
func TestZone09ArpaEmailDomain(t *testing.T) {
	entries := runZone09(t, "in-addr.arpa", func() packet.Packet {
		return mxPacket("in-addr.arpa", 300, mxRR{10, "mail.example."})
	})
	arpa := findEntryByTag(entries, "Z09_ARPA_EMAIL_DOMAIN")
	if arpa == nil {
		t.Fatalf("expected Z09_ARPA_EMAIL_DOMAIN, got %v", entryTags(entries))
	}
	if targets, ok := arpa.Args["mail_targets"].([]string); !ok || len(targets) != 1 || targets[0] != "mail.example" {
		t.Fatalf("expected mail_targets [mail.example], got %#v", arpa.Args["mail_targets"])
	}
	if hasEntryTag(entries, "Z09_MX_DATA") {
		t.Fatalf("arpa zone with MX must not emit Z09_MX_DATA, got %v", entryTags(entries))
	}
}

// TestZone09TLDEmailDomainReportsMailTargets verifies the TLD email-domain
// finding carries the mail target(s), mirroring a real ccTLD apex MX.
func TestZone09TLDEmailDomainReportsMailTargets(t *testing.T) {
	entries := runZone09(t, "example", func() packet.Packet {
		return mxPacket("example", 300, mxRR{0, "no.mx.example."})
	})
	tld := findEntryByTag(entries, "Z09_TLD_EMAIL_DOMAIN")
	if tld == nil {
		t.Fatalf("expected Z09_TLD_EMAIL_DOMAIN, got %v", entryTags(entries))
	}
	if targets, ok := tld.Args["mail_targets"].([]string); !ok || len(targets) != 1 || targets[0] != "no.mx.example" {
		t.Fatalf("expected mail_targets [no.mx.example], got %#v", tld.Args["mail_targets"])
	}
}

// TestZone09NoMXFoundOrExpectedForNonMailDomain verifies that a non-mail domain
// (here a TLD) with no MX gets the affirmative Z09_NO_MX_FOUND_OR_EXPECTED and
// not the missing-target warning.
func TestZone09NoMXFoundOrExpectedForNonMailDomain(t *testing.T) {
	entries := runZone09(t, "example", func() packet.Packet {
		return noMXPacket("example")
	})
	if !hasEntryTag(entries, "Z09_NO_MX_FOUND_OR_EXPECTED") {
		t.Fatalf("expected Z09_NO_MX_FOUND_OR_EXPECTED, got %v", entryTags(entries))
	}
	if hasEntryTag(entries, "Z09_MISSING_MAIL_TARGET") {
		t.Fatalf("non-mail domain must not emit Z09_MISSING_MAIL_TARGET, got %v", entryTags(entries))
	}
}

// TestZone09MissingMailTargetForNormalDomain verifies an ordinary domain with
// no MX still gets Z09_MISSING_MAIL_TARGET, not the non-mail-domain tag.
func TestZone09MissingMailTargetForNormalDomain(t *testing.T) {
	entries := runZone09(t, "example.com", func() packet.Packet {
		return noMXPacket("example.com")
	})
	if !hasEntryTag(entries, "Z09_MISSING_MAIL_TARGET") {
		t.Fatalf("expected Z09_MISSING_MAIL_TARGET, got %v", entryTags(entries))
	}
	if hasEntryTag(entries, "Z09_NO_MX_FOUND_OR_EXPECTED") {
		t.Fatalf("normal domain must not emit Z09_NO_MX_FOUND_OR_EXPECTED, got %v", entryTags(entries))
	}
}

// TestZone09ValidNullMX verifies that a single zero-preference null MX is
// reported as a valid "no mail" statement and not as a problem or as MX data.
func TestZone09ValidNullMX(t *testing.T) {
	entries := runZone09(t, "example.com", func() packet.Packet {
		return mxPacket("example.com", 300, mxRR{0, "."})
	})
	if !hasEntryTag(entries, "Z09_VALID_NULL_MX") {
		t.Fatalf("expected Z09_VALID_NULL_MX, got %v", entryTags(entries))
	}
	for _, tag := range []string{"Z09_NULL_MX_WITH_OTHER_MX", "Z09_NULL_MX_NON_ZERO_PREF", "Z09_MX_DATA"} {
		if hasEntryTag(entries, tag) {
			t.Fatalf("valid null MX must not emit %s, got %v", tag, entryTags(entries))
		}
	}
}

// TestZone09ValidNullMXSuppressedWithNonZeroPref verifies that a null MX with a
// non-zero preference is a problem, not a valid null MX.
func TestZone09ValidNullMXSuppressedWithNonZeroPref(t *testing.T) {
	entries := runZone09(t, "example.com", func() packet.Packet {
		return mxPacket("example.com", 300, mxRR{10, "."})
	})
	if !hasEntryTag(entries, "Z09_NULL_MX_NON_ZERO_PREF") {
		t.Fatalf("expected Z09_NULL_MX_NON_ZERO_PREF, got %v", entryTags(entries))
	}
	if hasEntryTag(entries, "Z09_VALID_NULL_MX") {
		t.Fatalf("non-zero-preference null MX must not emit Z09_VALID_NULL_MX, got %v", entryTags(entries))
	}
}

// TestZone09NoServersMXResponse verifies that when servers pass SOA gating but
// return no usable MX response, the aggregate Z09_NO_SERVERS_MX_RESPONSE is
// emitted alongside the per-server no-response tag.
func TestZone09NoServersMXResponse(t *testing.T) {
	entries := runZone09(t, "example.com", func() packet.Packet {
		return packet.Packet{}
	})
	if !hasEntryTag(entries, "Z09_NO_SERVERS_MX_RESPONSE") {
		t.Fatalf("expected Z09_NO_SERVERS_MX_RESPONSE, got %v", entryTags(entries))
	}
	if !hasEntryTag(entries, "Z09_NO_RESPONSE_MX_QUERY") {
		t.Fatalf("expected Z09_NO_RESPONSE_MX_QUERY, got %v", entryTags(entries))
	}
}

// TestZone09NoServersMXResponseSkippedWithoutServers verifies the aggregate tag
// is not emitted when there is no nameserver to query at all.
func TestZone09NoServersMXResponseSkippedWithoutServers(t *testing.T) {
	ctx := setupTest(t)

	orig := authoritativeNS
	t.Cleanup(func() { authoritativeNS = orig })

	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return nil, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone09(ctx, &z)
	if err != nil {
		t.Fatalf("zone09: %v", err)
	}
	if hasEntryTag(entries, "Z09_NO_SERVERS_MX_RESPONSE") {
		t.Fatalf("no servers to query; Z09_NO_SERVERS_MX_RESPONSE must not fire, got %v", entryTags(entries))
	}
}

// TestZone09EvaluatesMXWithoutSOA verifies the MX query has no SOA precondition:
// a server that does not answer SOA at all but returns MX data still produces
// MX findings. A reintroduced SOA gate would skip this server and drop the tag.
func TestZone09EvaluatesMXWithoutSOA(t *testing.T) {
	ctx := setupTest(t)

	orig := authoritativeNS
	t.Cleanup(func() { authoritativeNS = orig })

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.1", func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		switch qtype {
		case "MX":
			return mxPacket("example.com", 300, mxRR{10, "mail.example."})
		default:
			// No SOA (or anything else) is answered.
			return packet.Packet{}
		}
	})
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return []ens.Nameserver{ns}, nil
	}

	z := zonepkg.Zone{Name: dnsname.New("example.com")}
	entries, err := Zone09(ctx, &z)
	if err != nil {
		t.Fatalf("zone09: %v", err)
	}
	if !hasEntryTag(entries, "Z09_MX_DATA") {
		t.Fatalf("expected Z09_MX_DATA without a SOA precondition, got %v", entryTags(entries))
	}
}

// --- Zone15 (CAA at the zone apex) ---------------------------------------

// caaRecord is the test-side description of one CAA RR. The fields mirror
// rdata.CAA so that a test reads like the zone file line it stands for.
type caaRecord struct {
	flags uint8
	tag   string
	value string
}

// caaPacket builds an authoritative NOERROR answer carrying the given CAA
// records at the owner name. Passing no records yields the "authoritative, but
// the CAA RRset is empty" case, which is how a zone without CAA answers.
func caaPacket(name string, records []caaRecord) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(name), dns.TypeCAA)
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	for _, r := range records {
		caa := &dns.CAA{}
		caa.Hdr = dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 300}
		caa.CAA.Flag = r.flags
		caa.CAA.Tag = r.tag
		caa.CAA.Value = r.value
		msg.Answer = append(msg.Answer, caa)
	}
	return packet.Packet{Msg: msg}
}

// caaRcodePacket builds an authoritative response carrying only an RCODE, for
// the servers that answer the CAA query with a failure.
func caaRcodePacket(name string, rcode uint16) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(name), dns.TypeCAA)
	msg.Authoritative = true
	msg.Rcode = rcode
	return packet.Packet{Msg: msg}
}

// caaHandler answers CAA queries with the given packet and stays silent for
// every other query type, which also asserts that Zone15 asks for nothing else.
func caaHandler(resp packet.Packet) func(string, string, *ens.QueryOptions) packet.Packet {
	return func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
		if qtype == "CAA" {
			return resp
		}
		return packet.Packet{}
	}
}

// useNameservers points the module's authoritativeNS hook at a fixed list for
// the duration of one test.
func useNameservers(t *testing.T, nss ...ens.Nameserver) {
	t.Helper()
	orig := authoritativeNS
	t.Cleanup(func() { authoritativeNS = orig })
	authoritativeNS = func(_ context.Context, _ *zonepkg.Zone) ([]ens.Nameserver, error) {
		return nss, nil
	}
}

// runZone15 runs the testcase against a zone name and fails on error.
func runZone15(t *testing.T, ctx context.Context, zoneName string) []*logger.Entry {
	t.Helper()
	z := zonepkg.Zone{Name: dnsname.New(zoneName)}
	entries, err := Zone15(ctx, &z)
	if err != nil {
		t.Fatalf("zone15: %v", err)
	}
	return entries
}

func entriesWithTag(entries []*logger.Entry, tag string) []*logger.Entry {
	var out []*logger.Entry
	for _, entry := range entries {
		if entry != nil && entry.Tag == tag {
			out = append(out, entry)
		}
	}
	return out
}

func firstEntryWithTag(t *testing.T, entries []*logger.Entry, tag string) *logger.Entry {
	t.Helper()
	found := entriesWithTag(entries, tag)
	if len(found) == 0 {
		t.Fatalf("expected %s, got %v", tag, entryTags(entries))
	}
	return found[0]
}

func requireNoTag(t *testing.T, entries []*logger.Entry, tags ...string) {
	t.Helper()
	for _, tag := range tags {
		if hasEntryTag(entries, tag) {
			t.Fatalf("did not expect %s, got %v", tag, entryTags(entries))
		}
	}
}

func TestZone15CAAFound(t *testing.T) {
	ctx := setupTest(t)

	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", []caaRecord{{flags: 0, tag: "issue", value: "ca.example"}}),
	))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")

	found := firstEntryWithTag(t, entries, "Z15_CAA_FOUND")
	if got, ok := found.Args["caa_flags"].(uint8); !ok || got != 0 {
		t.Fatalf("expected caa_flags=0, got %#v", found.Args["caa_flags"])
	}
	if got, ok := found.Args["caa_property"].(string); !ok || got != "issue" {
		t.Fatalf("expected caa_property=issue, got %#v", found.Args["caa_property"])
	}
	if got, ok := found.Args["caa_value"].(string); !ok || got != "ca.example" {
		t.Fatalf("expected caa_value=ca.example, got %#v", found.Args["caa_value"])
	}
	// A single well-formed permitting record is a clean result: no defect tag,
	// and no policy verdict since issuance is not forbidden.
	requireNoTag(t, entries,
		"Z15_NO_CAA", "Z15_NO_CAA_TLD", "Z15_RESERVED_FLAGS", "Z15_INVALID_PROPERTY_TAG",
		"Z15_UNKNOWN_PROPERTY", "Z15_UNKNOWN_PROPERTY_CRITICAL", "Z15_INVALID_ISSUE_VALUE",
		"Z15_ISSUANCE_FORBIDDEN", "Z15_ISSUE_CONTRADICTION")
}

func TestZone15ConsolidatedFound(t *testing.T) {
	ctx := setupTest(t)

	// Two nameservers serving byte-identical content must collapse into one
	// finding listing both endpoints, rather than one finding per server.
	resp := caaPacket("example.com", []caaRecord{{flags: 0, tag: "issue", value: "ca.example"}})
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(resp))
	ns2 := newNameserver(t, ctx, "ns2.example.com", "192.0.2.2", caaHandler(resp))
	useNameservers(t, ns1, ns2)

	entries := runZone15(t, ctx, "example.com")

	found := entriesWithTag(entries, "Z15_CAA_FOUND")
	if len(found) != 1 {
		t.Fatalf("expected one consolidated Z15_CAA_FOUND, got %d", len(found))
	}
	servers, ok := found[0].Args["servers"].([]map[string]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("expected both name servers in servers, got %#v", found[0].Args["servers"])
	}
	requireNoTag(t, entries, "Z15_INCONSISTENT_CAA", "Z15_MIXED_PRESENCE")
}

func TestZone15NoCAA(t *testing.T) {
	ctx := setupTest(t)

	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", nil),
	))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")

	noCAA := firstEntryWithTag(t, entries, "Z15_NO_CAA")
	servers, ok := noCAA.Args["servers"].([]map[string]any)
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one server in Z15_NO_CAA, got %#v", noCAA.Args["servers"])
	}
	// A normal delegated domain must not get the TLD wording.
	requireNoTag(t, entries, "Z15_NO_CAA_TLD", "Z15_MIXED_PRESENCE")
}

func TestZone15NoCAATLD(t *testing.T) {
	// A TLD (and the root) cannot hold a publicly trusted certificate for its
	// own name, so the "any CA may issue for this domain" wording of
	// Z15_NO_CAA would be false there.
	for _, zoneName := range []string{"se", "."} {
		t.Run(zoneName, func(t *testing.T) {
			ctx := setupTest(t)
			ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
				caaPacket(zoneName, nil),
			))
			useNameservers(t, ns1)

			entries := runZone15(t, ctx, zoneName)

			if !hasEntryTag(entries, "Z15_NO_CAA_TLD") {
				t.Fatalf("expected Z15_NO_CAA_TLD for %q, got %v", zoneName, entryTags(entries))
			}
			requireNoTag(t, entries, "Z15_NO_CAA")
		})
	}
}

func TestZone15PublicSuffixIsNotATLD(t *testing.T) {
	// The absence tag keys off label count, not the Public Suffix List, and
	// that is deliberate. Z15_NO_CAA_TLD claims the apex cannot hold a
	// publicly trusted certificate, which is true for dotless names only.
	// A multi-label public suffix is an ordinary FQDN that may hold one
	// (github.io demonstrably does), so it must get the plain Z15_NO_CAA.
	for _, zoneName := range []string{"co.uk", "github.io"} {
		t.Run(zoneName, func(t *testing.T) {
			ctx := setupTest(t)
			ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
				caaPacket(zoneName, nil),
			))
			useNameservers(t, ns1)

			entries := runZone15(t, ctx, zoneName)

			if !hasEntryTag(entries, "Z15_NO_CAA") {
				t.Fatalf("expected Z15_NO_CAA for public suffix %q, got %v", zoneName, entryTags(entries))
			}
			requireNoTag(t, entries, "Z15_NO_CAA_TLD")
		})
	}
}

func TestZone15NoResponse(t *testing.T) {
	ctx := setupTest(t)

	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(packet.Packet{}))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")

	noResp := firstEntryWithTag(t, entries, "Z15_NO_RESPONSE_CAA_QUERY")
	addrs, ok := noResp.Args["addresses"].([]string)
	if !ok || len(addrs) != 1 || addrs[0] != "192.0.2.1" {
		t.Fatalf("expected silent endpoint [192.0.2.1], got %#v", noResp.Args["addresses"])
	}
	// A server that never answered is not a server that "has no CAA".
	requireNoTag(t, entries, "Z15_NO_CAA", "Z15_NO_CAA_TLD", "Z15_MIXED_PRESENCE")
}

func TestZone15UnexpectedRcode(t *testing.T) {
	ctx := setupTest(t)

	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaRcodePacket("example.com", dns.RcodeServerFailure),
	))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")

	bad := firstEntryWithTag(t, entries, "Z15_UNEXPECTED_RCODE_CAA")
	if got, ok := bad.Args["rcode"].(string); !ok || got != "SERVFAIL" {
		t.Fatalf("expected rcode=SERVFAIL, got %#v", bad.Args["rcode"])
	}
	addrs, ok := bad.Args["addresses"].([]string)
	if !ok || len(addrs) != 1 || addrs[0] != "192.0.2.1" {
		t.Fatalf("expected failing endpoint [192.0.2.1], got %#v", bad.Args["addresses"])
	}
	requireNoTag(t, entries, "Z15_NO_CAA", "Z15_MIXED_PRESENCE")
}

func TestZone15NonAuthoritativeSkipped(t *testing.T) {
	ctx := setupTest(t)

	// NOERROR without AA is another module's finding (lameness); Zone15 keeps
	// quiet rather than restating it, matching Zone14.
	resp := caaPacket("example.com", []caaRecord{{flags: 0, tag: "issue", value: "ca.example"}})
	resp.Msg.Authoritative = false
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(resp))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")

	requireNoTag(t, entries,
		"Z15_CAA_FOUND", "Z15_NO_CAA", "Z15_NO_CAA_TLD", "Z15_MIXED_PRESENCE",
		"Z15_INCONSISTENT_CAA", "Z15_NO_RESPONSE_CAA_QUERY", "Z15_UNEXPECTED_RCODE_CAA")
	if !hasEntryTag(entries, "TEST_CASE_END") {
		t.Fatalf("expected TEST_CASE_END even when every server is skipped")
	}
}

func TestZone15MixedPresence(t *testing.T) {
	ctx := setupTest(t)

	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", []caaRecord{{flags: 0, tag: "issue", value: "ca.example"}}),
	))
	ns2 := newNameserver(t, ctx, "ns2.example.com", "192.0.2.2", caaHandler(
		caaPacket("example.com", nil),
	))
	useNameservers(t, ns1, ns2)

	entries := runZone15(t, ctx, "example.com")

	if !hasEntryTag(entries, "Z15_MIXED_PRESENCE") {
		t.Fatalf("expected Z15_MIXED_PRESENCE, got %v", entryTags(entries))
	}
	if !hasEntryTag(entries, "Z15_CAA_FOUND") || !hasEntryTag(entries, "Z15_NO_CAA") {
		t.Fatalf("expected both presence groups reported, got %v", entryTags(entries))
	}
}

func TestZone15Inconsistent(t *testing.T) {
	ctx := setupTest(t)

	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", []caaRecord{{flags: 0, tag: "issue", value: "ca1.example"}}),
	))
	ns2 := newNameserver(t, ctx, "ns2.example.com", "192.0.2.2", caaHandler(
		caaPacket("example.com", []caaRecord{{flags: 0, tag: "issue", value: "ca2.example"}}),
	))
	useNameservers(t, ns1, ns2)

	entries := runZone15(t, ctx, "example.com")

	if !hasEntryTag(entries, "Z15_INCONSISTENT_CAA") {
		t.Fatalf("expected Z15_INCONSISTENT_CAA, got %v", entryTags(entries))
	}
	if got := len(entriesWithTag(entries, "Z15_CAA_FOUND")); got != 2 {
		t.Fatalf("expected one Z15_CAA_FOUND per distinct content, got %d", got)
	}
	requireNoTag(t, entries, "Z15_MIXED_PRESENCE")
}

func TestZone15ReservedFlags(t *testing.T) {
	ctx := setupTest(t)

	// RFC 8659 section 4.1 defines only bit 0 (value 128, issuer critical).
	// Flags 1 sets a reserved bit; flags 129 sets a reserved bit alongside the
	// critical bit. Neither record has an unknown property.
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", []caaRecord{
			{flags: 1, tag: "issue", value: "ca.example"},
			{flags: 129, tag: "issuewild", value: "ca.example"},
		}),
	))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")

	if got := len(entriesWithTag(entries, "Z15_RESERVED_FLAGS")); got != 2 {
		t.Fatalf("expected Z15_RESERVED_FLAGS per offending record, got %d", got)
	}
	requireNoTag(t, entries, "Z15_UNKNOWN_PROPERTY", "Z15_UNKNOWN_PROPERTY_CRITICAL")
}

func TestZone15InvalidPropertyTag(t *testing.T) {
	ctx := setupTest(t)

	// RFC 8659 section 4.1: tags MUST NOT contain characters outside ASCII
	// letters and digits, and the wire-format tag length must be at least 1.
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", []caaRecord{
			{flags: 0, tag: "", value: "ca.example"},
			{flags: 0, tag: "iss ue", value: "ca.example"},
			{flags: 0, tag: "iss_ue", value: "ca.example"},
		}),
	))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")

	if got := len(entriesWithTag(entries, "Z15_INVALID_PROPERTY_TAG")); got != 3 {
		t.Fatalf("expected Z15_INVALID_PROPERTY_TAG per offending record, got %d", got)
	}
	// An unusable tag is not classified further: there is no property to
	// validate the value against, and the critical flag is not set.
	requireNoTag(t, entries,
		"Z15_UNKNOWN_PROPERTY", "Z15_UNKNOWN_PROPERTY_CRITICAL", "Z15_INVALID_ISSUE_VALUE")
}

func TestZone15InvalidPropertyTagCritical(t *testing.T) {
	ctx := setupTest(t)

	// A critical-flagged garbage tag blocks issuance exactly as a well-formed
	// unknown tag does: RFC 8659 section 4.1 forbids issuance for a critical
	// property that is "unknown or unsupported", and an invalid tag can never
	// be supported.
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", []caaRecord{{flags: 128, tag: "iss_ue", value: "ca.example"}}),
	))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")

	if !hasEntryTag(entries, "Z15_INVALID_PROPERTY_TAG") {
		t.Fatalf("expected Z15_INVALID_PROPERTY_TAG, got %v", entryTags(entries))
	}
	critical := firstEntryWithTag(t, entries, "Z15_UNKNOWN_PROPERTY_CRITICAL")
	if got, ok := critical.Args["caa_flags"].(uint8); !ok || got != 128 {
		t.Fatalf("expected caa_flags=128, got %#v", critical.Args["caa_flags"])
	}
}

func TestZone15UnknownProperty(t *testing.T) {
	tests := []struct {
		name     string
		flags    uint8
		wantTag  string
		otherTag string
	}{
		// Consumers ignore unrecognized non-critical properties (RFC 8659
		// section 3), so this is a hygiene notice only.
		{name: "non-critical", flags: 0, wantTag: "Z15_UNKNOWN_PROPERTY", otherTag: "Z15_UNKNOWN_PROPERTY_CRITICAL"},
		// With the critical bit set, every conforming CA refuses all issuance.
		{name: "critical", flags: 128, wantTag: "Z15_UNKNOWN_PROPERTY_CRITICAL", otherTag: "Z15_UNKNOWN_PROPERTY"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := setupTest(t)
			ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
				caaPacket("example.com", []caaRecord{{flags: tc.flags, tag: "futureprop", value: "x"}}),
			))
			useNameservers(t, ns1)

			entries := runZone15(t, ctx, "example.com")

			if !hasEntryTag(entries, tc.wantTag) {
				t.Fatalf("expected %s, got %v", tc.wantTag, entryTags(entries))
			}
			requireNoTag(t, entries, tc.otherTag)
			prop := firstEntryWithTag(t, entries, tc.wantTag)
			if got, ok := prop.Args["caa_property"].(string); !ok || got != "futureprop" {
				t.Fatalf("expected caa_property=futureprop, got %#v", prop.Args["caa_property"])
			}
		})
	}
}

func TestZone15KnownPropertyCaseInsensitive(t *testing.T) {
	ctx := setupTest(t)

	// RFC 8659 section 4.1 makes tag matching case insensitive, so "Issue"
	// classifies as issue. Mixed case is not itself a finding.
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", []caaRecord{{flags: 0, tag: "Issue", value: "ca.example"}}),
	))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")

	requireNoTag(t, entries,
		"Z15_UNKNOWN_PROPERTY", "Z15_UNKNOWN_PROPERTY_CRITICAL", "Z15_INVALID_PROPERTY_TAG")
	// The value is still validated against the issue-value grammar.
	requireNoTag(t, entries, "Z15_INVALID_ISSUE_VALUE")
}

func TestZone15RegisteredPropertiesKnown(t *testing.T) {
	ctx := setupTest(t)

	// Every assignable entry in the IANA registry is recognized, so domains
	// using the contact properties or issuevmc get no false unknown finding.
	known := []caaRecord{
		{flags: 0, tag: "issuemail", value: "ca.example"},
		{flags: 0, tag: "contactemail", value: "admin@example.com"},
		{flags: 0, tag: "contactphone", value: "+46 8 000 000"},
		{flags: 0, tag: "issuevmc", value: "ca.example"},
	}
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", known),
	))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")
	requireNoTag(t, entries, "Z15_UNKNOWN_PROPERTY", "Z15_UNKNOWN_PROPERTY_CRITICAL")

	// The Reserved entries are deliberately treated as unknown: RFC 8659
	// reserved them so they can never be assigned a meaning, so no CA will
	// ever act on them.
	for _, tag := range []string{"auth", "path", "policy"} {
		t.Run(tag, func(t *testing.T) {
			ctx := setupTest(t)
			ns := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
				caaPacket("example.com", []caaRecord{{flags: 0, tag: tag, value: "x"}}),
			))
			useNameservers(t, ns)

			entries := runZone15(t, ctx, "example.com")
			if !hasEntryTag(entries, "Z15_UNKNOWN_PROPERTY") {
				t.Fatalf("expected reserved tag %q to be reported unknown, got %v", tag, entryTags(entries))
			}
		})
	}
}

func TestZone15InvalidIssueValue(t *testing.T) {
	for _, property := range []string{"issue", "issuewild", "issuemail"} {
		t.Run(property, func(t *testing.T) {
			ctx := setupTest(t)
			ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
				caaPacket("example.com", []caaRecord{
					{flags: 0, tag: property, value: "bad_domain!"},
				}),
			))
			useNameservers(t, ns1)

			entries := runZone15(t, ctx, "example.com")

			bad := firstEntryWithTag(t, entries, "Z15_INVALID_ISSUE_VALUE")
			if got, ok := bad.Args["caa_property"].(string); !ok || got != property {
				t.Fatalf("expected caa_property=%s, got %#v", property, bad.Args["caa_property"])
			}
			if got, ok := bad.Args["caa_value"].(string); !ok || got != "bad_domain!" {
				t.Fatalf("expected caa_value=bad_domain!, got %#v", bad.Args["caa_value"])
			}
		})
	}
}

func TestZone15IssueParametersValid(t *testing.T) {
	ctx := setupTest(t)

	// RFC 8657 parameters are CA-defined; only the grammar is checked, so a
	// well-formed parameter list must not be reported.
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", []caaRecord{
			{flags: 0, tag: "issue", value: "ca.example; validationmethods=dns-01"},
		}),
	))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")
	requireNoTag(t, entries, "Z15_INVALID_ISSUE_VALUE", "Z15_ISSUANCE_FORBIDDEN")
}

func TestZone15InvalidIodef(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantBad bool
	}{
		{name: "not a url", value: "not a url", wantBad: true},
		// RFC 8659 section 4.4: mailto, http and https are the only supported
		// schemes.
		{name: "ftp scheme", value: "ftp://x.example/report", wantBad: true},
		{name: "empty mailto", value: "mailto:", wantBad: true},
		{name: "mailto", value: "mailto:security@example.com", wantBad: false},
		{name: "https", value: "https://example.com/iodef", wantBad: false},
		{name: "http", value: "http://example.com/iodef", wantBad: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := setupTest(t)
			ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
				caaPacket("example.com", []caaRecord{{flags: 0, tag: "iodef", value: tc.value}}),
			))
			useNameservers(t, ns1)

			entries := runZone15(t, ctx, "example.com")

			got := hasEntryTag(entries, "Z15_INVALID_IODEF_VALUE")
			if got != tc.wantBad {
				t.Fatalf("iodef %q: got Z15_INVALID_IODEF_VALUE=%v, want %v", tc.value, got, tc.wantBad)
			}
		})
	}
}

func TestZone15IssuanceForbidden(t *testing.T) {
	ctx := setupTest(t)

	// The deliberate lockdown: RFC 8659 section 4.2 spells it issue ";".
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", []caaRecord{{flags: 0, tag: "issue", value: ";"}}),
	))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")

	if !hasEntryTag(entries, "Z15_ISSUANCE_FORBIDDEN") {
		t.Fatalf("expected Z15_ISSUANCE_FORBIDDEN, got %v", entryTags(entries))
	}
	// Nothing contradicts it, and ";" is valid syntax rather than an error.
	requireNoTag(t, entries, "Z15_ISSUE_CONTRADICTION", "Z15_INVALID_ISSUE_VALUE")
}

func TestZone15MalformedIssueForbids(t *testing.T) {
	ctx := setupTest(t)

	// RFC 8659 section 4.2: an issue-value that does not match the grammar
	// "MUST be treated the same as one specifying an empty issuer-domain-name".
	// So a lone malformed record is not merely untidy, it blocks issuance.
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", []caaRecord{{flags: 0, tag: "issue", value: "bad_domain!"}}),
	))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")

	if !hasEntryTag(entries, "Z15_INVALID_ISSUE_VALUE") {
		t.Fatalf("expected Z15_INVALID_ISSUE_VALUE, got %v", entryTags(entries))
	}
	if !hasEntryTag(entries, "Z15_ISSUANCE_FORBIDDEN") {
		t.Fatalf("expected a malformed issue value to forbid issuance, got %v", entryTags(entries))
	}
}

func TestZone15IssueWildOverridesForbidden(t *testing.T) {
	// RFC 8659 section 4.3: issuewild takes precedence over issue for wildcard
	// names. With issue ";" plus a permitting issuewild, wildcard issuance is
	// still open, so claiming all issuance is forbidden would be wrong.
	t.Run("issuewild permits", func(t *testing.T) {
		ctx := setupTest(t)
		ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
			caaPacket("example.com", []caaRecord{
				{flags: 0, tag: "issue", value: ";"},
				{flags: 0, tag: "issuewild", value: "ca.example"},
			}),
		))
		useNameservers(t, ns1)

		entries := runZone15(t, ctx, "example.com")
		requireNoTag(t, entries, "Z15_ISSUANCE_FORBIDDEN")
	})

	t.Run("issuewild also forbids", func(t *testing.T) {
		ctx := setupTest(t)
		ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
			caaPacket("example.com", []caaRecord{
				{flags: 0, tag: "issue", value: ";"},
				{flags: 0, tag: "issuewild", value: ";"},
			}),
		))
		useNameservers(t, ns1)

		entries := runZone15(t, ctx, "example.com")
		if !hasEntryTag(entries, "Z15_ISSUANCE_FORBIDDEN") {
			t.Fatalf("expected Z15_ISSUANCE_FORBIDDEN when both properties forbid, got %v", entryTags(entries))
		}
	})
}

func TestZone15IssueContradiction(t *testing.T) {
	// Under RFC 8659 union semantics the forbidding record is inert when a
	// sibling permits a CA, so it is almost always a leftover.
	for _, property := range []string{"issue", "issuewild"} {
		t.Run(property, func(t *testing.T) {
			ctx := setupTest(t)
			ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
				caaPacket("example.com", []caaRecord{
					{flags: 0, tag: property, value: ";"},
					{flags: 0, tag: property, value: "ca.example"},
				}),
			))
			useNameservers(t, ns1)

			entries := runZone15(t, ctx, "example.com")

			contradiction := firstEntryWithTag(t, entries, "Z15_ISSUE_CONTRADICTION")
			if got, ok := contradiction.Args["caa_property"].(string); !ok || got != property {
				t.Fatalf("expected caa_property=%s, got %#v", property, contradiction.Args["caa_property"])
			}
			requireNoTag(t, entries, "Z15_ISSUANCE_FORBIDDEN")
		})
	}
}

func TestZone15PolicySuppressedWhenInconsistent(t *testing.T) {
	ctx := setupTest(t)

	// The union of a forbidding NS1 and a permitting NS2 looks like a
	// contradiction, but neither server actually publishes that RRset: on NS1
	// issuance really is forbidden. The inconsistency is the finding.
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket("example.com", []caaRecord{{flags: 0, tag: "issue", value: ";"}}),
	))
	ns2 := newNameserver(t, ctx, "ns2.example.com", "192.0.2.2", caaHandler(
		caaPacket("example.com", []caaRecord{{flags: 0, tag: "issue", value: "ca.example"}}),
	))
	useNameservers(t, ns1, ns2)

	entries := runZone15(t, ctx, "example.com")

	if !hasEntryTag(entries, "Z15_INCONSISTENT_CAA") {
		t.Fatalf("expected Z15_INCONSISTENT_CAA, got %v", entryTags(entries))
	}
	requireNoTag(t, entries, "Z15_ISSUANCE_FORBIDDEN", "Z15_ISSUE_CONTRADICTION")
}

func TestZone15RootNoPolicyVerdict(t *testing.T) {
	ctx := setupTest(t)

	// RFC 8659 section 3 climbs "up to, but not including, the DNS root", so a
	// CAA RRset at the root apex is never consulted by any CA. It is still
	// reported, but no policy verdict can follow from it.
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaPacket(".", []caaRecord{{flags: 0, tag: "issue", value: ";"}}),
	))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, ".")

	if !hasEntryTag(entries, "Z15_CAA_FOUND") {
		t.Fatalf("expected Z15_CAA_FOUND at the root, got %v", entryTags(entries))
	}
	requireNoTag(t, entries, "Z15_ISSUANCE_FORBIDDEN", "Z15_ISSUE_CONTRADICTION")
}

func TestZone15PartialFailure(t *testing.T) {
	ctx := setupTest(t)

	// One server fails the query, the other serves records. The failure is
	// reported, but a failed server must not be counted as "has no CAA", so
	// there is no mixed presence.
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(
		caaRcodePacket("example.com", dns.RcodeServerFailure),
	))
	ns2 := newNameserver(t, ctx, "ns2.example.com", "192.0.2.2", caaHandler(
		caaPacket("example.com", []caaRecord{{flags: 0, tag: "issue", value: "ca.example"}}),
	))
	useNameservers(t, ns1, ns2)

	entries := runZone15(t, ctx, "example.com")

	if !hasEntryTag(entries, "Z15_UNEXPECTED_RCODE_CAA") {
		t.Fatalf("expected Z15_UNEXPECTED_RCODE_CAA, got %v", entryTags(entries))
	}
	if !hasEntryTag(entries, "Z15_CAA_FOUND") {
		t.Fatalf("expected Z15_CAA_FOUND from the healthy server, got %v", entryTags(entries))
	}
	requireNoTag(t, entries, "Z15_MIXED_PRESENCE", "Z15_INCONSISTENT_CAA")
}

func TestZone15ApexCNAME(t *testing.T) {
	ctx := setupTest(t)

	// A CNAME answer yields no CAA records for the owner name. Apex CNAME
	// problems belong to other testcases, so this counts as absence here.
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn("example.com"), dns.TypeCAA)
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	cname := &dns.CNAME{Hdr: dns.Header{Name: dnsutil.Fqdn("example.com"), Class: dns.ClassINET, TTL: 300}}
	cname.CNAME.Target = dnsutil.Fqdn("target.example")
	msg.Answer = []dns.RR{cname}

	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", caaHandler(packet.Packet{Msg: msg}))
	useNameservers(t, ns1)

	entries := runZone15(t, ctx, "example.com")

	if !hasEntryTag(entries, "Z15_NO_CAA") {
		t.Fatalf("expected a CNAME answer to count as absence, got %v", entryTags(entries))
	}
	requireNoTag(t, entries, "Z15_CAA_FOUND", "Z15_ISSUANCE_FORBIDDEN")
}

func TestParseCAAIssueValue(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		wantValid   bool
		wantForbids bool
	}{
		// Every element of the ABNF is optional, so both spellings of "no
		// issuer is authorized" parse cleanly.
		{name: "empty", value: "", wantValid: true, wantForbids: true},
		{name: "semicolon only", value: ";", wantValid: true, wantForbids: true},
		{name: "whitespace only", value: "  ", wantValid: true, wantForbids: true},
		{name: "semicolon with spaces", value: " ; ", wantValid: true, wantForbids: true},

		{name: "plain issuer", value: "ca.example", wantValid: true, wantForbids: false},
		{name: "single label issuer", value: "ca", wantValid: true, wantForbids: false},
		{name: "leading whitespace", value: "  ca.example", wantValid: true, wantForbids: false},
		{name: "trailing whitespace", value: "ca.example  ", wantValid: true, wantForbids: false},
		{name: "tab whitespace", value: "\tca.example\t", wantValid: true, wantForbids: false},
		{name: "digits in label", value: "ca1.example2", wantValid: true, wantForbids: false},
		// A label may carry hyphens in the interior, including consecutive
		// ones: label = (ALPHA / DIGIT) *( *("-") (ALPHA / DIGIT)).
		{name: "interior hyphen", value: "my-ca.example", wantValid: true, wantForbids: false},
		{name: "consecutive hyphens", value: "my--ca.example", wantValid: true, wantForbids: false},

		{name: "one parameter", value: "ca.example; account=123", wantValid: true, wantForbids: false},
		{name: "two parameters", value: "ca.example; a=1; b=2", wantValid: true, wantForbids: false},
		{name: "whitespace around equals", value: "ca.example; a = 1", wantValid: true, wantForbids: false},
		{name: "no space after semicolon", value: "ca.example;a=1", wantValid: true, wantForbids: false},
		{name: "hyphen in parameter tag", value: "ca.example; my-tag=1", wantValid: true, wantForbids: false},
		{name: "parameters without issuer", value: "; a=1", wantValid: true, wantForbids: true},

		// Invalid values forbid issuance too (RFC 8659 section 4.2).
		{name: "underscore in label", value: "bad_domain", wantValid: false, wantForbids: true},
		{name: "bang in label", value: "bad!", wantValid: false, wantForbids: true},
		{name: "wildcard label", value: "*.example", wantValid: false, wantForbids: true},
		{name: "leading hyphen", value: "-bad.example", wantValid: false, wantForbids: true},
		{name: "trailing hyphen", value: "bad-.example", wantValid: false, wantForbids: true},
		{name: "trailing dot", value: "ca.example.", wantValid: false, wantForbids: true},
		{name: "leading dot", value: ".ca.example", wantValid: false, wantForbids: true},
		{name: "empty label", value: "ca..example", wantValid: false, wantForbids: true},
		{name: "space inside issuer", value: "ca example", wantValid: false, wantForbids: true},
		{name: "parameter without name", value: "ca.example; =broken", wantValid: false, wantForbids: true},
		{name: "parameter without equals", value: "ca.example; broken", wantValid: false, wantForbids: true},
		// A parameter value is %x21-3A / %x3C-7E, which excludes the space.
		{name: "space in parameter value", value: "ca.example; a=1 2", wantValid: false, wantForbids: true},
		{name: "empty parameter", value: "ca.example; ;", wantValid: false, wantForbids: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCAAIssueValue(tc.value)
			if got.valid != tc.wantValid {
				t.Fatalf("parseCAAIssueValue(%q).valid = %v, want %v", tc.value, got.valid, tc.wantValid)
			}
			if got.forbids != tc.wantForbids {
				t.Fatalf("parseCAAIssueValue(%q).forbids = %v, want %v", tc.value, got.forbids, tc.wantForbids)
			}
		})
	}
}

func TestCaaIodefValueValid(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "mailto", value: "mailto:security@example.com", want: true},
		{name: "https", value: "https://example.com/iodef", want: true},
		{name: "http", value: "http://example.com/iodef", want: true},
		// Scheme matching is case insensitive per RFC 3986.
		{name: "uppercase scheme", value: "MAILTO:security@example.com", want: true},

		{name: "empty", value: "", want: false},
		{name: "plain text", value: "not a url", want: false},
		{name: "ftp", value: "ftp://example.com/iodef", want: false},
		{name: "mailto without address", value: "mailto:", want: false},
		{name: "https without host", value: "https:///iodef", want: false},
		{name: "bare hostname", value: "example.com", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := caaIodefValueValid(tc.value)
			if got != tc.want {
				t.Fatalf("caaIodefValueValid(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestZone15QueryBudget(t *testing.T) {
	ctx := setupTest(t)

	// The query budget is a hard constraint: exactly one CAA query per
	// authoritative endpoint and nothing else. Unlike Zone14 there is no
	// companion SOA query, so a regression here would be easy to miss.
	var mu sync.Mutex
	queries := map[string][]string{}
	record := func(endpoint string) func(string, string, *ens.QueryOptions) packet.Packet {
		return func(_ string, qtype string, _ *ens.QueryOptions) packet.Packet {
			mu.Lock()
			queries[endpoint] = append(queries[endpoint], qtype)
			mu.Unlock()
			if qtype == "CAA" {
				return caaPacket("example.com", []caaRecord{{flags: 0, tag: "issue", value: "ca.example"}})
			}
			return packet.Packet{}
		}
	}
	ns1 := newNameserver(t, ctx, "ns1.example.com", "192.0.2.1", record("192.0.2.1"))
	ns2 := newNameserver(t, ctx, "ns2.example.com", "192.0.2.2", record("192.0.2.2"))
	useNameservers(t, ns1, ns2)

	runZone15(t, ctx, "example.com")

	if len(queries) != 2 {
		t.Fatalf("expected both endpoints queried, got %#v", queries)
	}
	for endpoint, types := range queries {
		if len(types) != 1 || types[0] != "CAA" {
			t.Fatalf("endpoint %s: expected exactly one CAA query, got %v", endpoint, types)
		}
	}
}
