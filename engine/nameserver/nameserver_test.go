package nameserver

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func TestFakeDSResponse(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	err = ns.AddFakeDS("example", []DSData{{
		KeyTag:     1234,
		Algorithm:  8,
		DigestType: 2,
		Digest:     "ABCD",
	}})
	if err != nil {
		t.Fatalf("add fake DS: %v", err)
	}

	resp, err := ns.QueryWithOptions(context.Background(), "example", "DS", nil)
	if err != nil {
		t.Fatalf("query fake DS: %v", err)
	}
	if resp.Msg == nil || len(resp.Msg.Answer) != 1 {
		t.Fatalf("expected DS answer, got %#v", resp.Msg)
	}
	if _, ok := resp.Msg.Answer[0].(*dns.DS); !ok {
		t.Fatalf("expected DS record, got %T", resp.Msg.Answer[0])
	}
	if !resp.Msg.Authoritative {
		t.Fatalf("expected authoritative response")
	}
}

func TestFakeDelegationNS(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	err = ns.AddFakeDelegation("example", map[string][]string{
		"ns1.example.": {"192.0.2.2"},
	})
	if err != nil {
		t.Fatalf("add fake delegation: %v", err)
	}

	resp, err := ns.QueryWithOptions(context.Background(), "example", "NS", nil)
	if err != nil {
		t.Fatalf("query fake delegation: %v", err)
	}
	if resp.Msg == nil || len(resp.Msg.Answer) == 0 {
		t.Fatalf("expected delegation answer, got %#v", resp.Msg)
	}
	if _, ok := resp.Msg.Answer[0].(*dns.NS); !ok {
		t.Fatalf("expected NS record, got %T", resp.Msg.Answer[0])
	}
	if len(resp.Msg.Extra) == 0 {
		t.Fatalf("expected glue in additional section")
	}
}

func TestQueryCacheHit(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.10", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "example.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				A: net.IPv4(192, 0, 2, 5),
			},
		}
		return packet.Packet{Msg: msg}, nil
	})

	_, err = ns.QueryWithOptions(context.Background(), "example", "A", nil)
	if err != nil {
		t.Fatalf("query 1: %v", err)
	}
	_, err = ns.QueryWithOptions(context.Background(), "example", "A", nil)
	if err != nil {
		t.Fatalf("query 2: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestErrorCacheSkipsQueries(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.15", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	defer profile.ResetEffective()
	defer EmptyCache()

	profile.Effective().Resolver.Defaults.ErrorCacheTTL = 60

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("network error")
	})

	_, err = ns.QueryWithOptions(context.Background(), "example1", "A", nil)
	if err == nil {
		t.Fatalf("expected error on first query")
	}
	_, err = ns.QueryWithOptions(context.Background(), "example2", "A", nil)
	if err != nil {
		t.Fatalf("expected error cache to suppress second query error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call due to error cache, got %d", calls)
	}
}

func TestQueryIPv4Disabled(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.11", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	defer profile.ResetEffective()
	profile.Effective().Net.IPv4 = false

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, nil
	})

	resp, err := ns.QueryWithOptions(context.Background(), "example", "A", nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if resp.Msg != nil {
		t.Fatalf("expected no response when IPv4 disabled")
	}
	if calls != 0 {
		t.Fatalf("expected hook not called, got %d", calls)
	}
}

func TestClientForOptionsDefaults(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.12", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	defer profile.ResetEffective()
	profile.Effective().Resolver.Defaults.Recurse = true
	profile.Effective().Resolver.Defaults.UseVC = true

	client, err := ns.clientForOptions(nil)
	if err != nil {
		t.Fatalf("client for nil opts: %v", err)
	}
	if client.RecursionDesired {
		t.Fatalf("expected recursion disabled by default")
	}
	if client.UseTCP {
		t.Fatalf("expected UDP by default")
	}

	on := true
	client, err = ns.clientForOptions(&QueryOptions{Recurse: &on, UseVC: &on})
	if err != nil {
		t.Fatalf("client for explicit opts: %v", err)
	}
	if !client.RecursionDesired {
		t.Fatalf("expected recursion enabled when requested")
	}
	if !client.UseTCP {
		t.Fatalf("expected TCP when requested")
	}
}

func TestAXFRHook(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.22", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	rr1, err := dns.NewRR("example. 60 IN SOA ns.example. hostmaster.example. 1 3600 600 86400 60")
	if err != nil {
		t.Fatalf("soa rr: %v", err)
	}
	rr2, err := dns.NewRR("example. 60 IN A 192.0.2.10")
	if err != nil {
		t.Fatalf("a rr: %v", err)
	}

	ns.SetAXFRHook(func(_ context.Context, domain string, callback func(dns.RR) bool, class string) error {
		if domain != "example" {
			return fmt.Errorf("unexpected domain %q", domain)
		}
		if class != "CH" {
			return fmt.Errorf("unexpected class %q", class)
		}
		callback(rr1)
		callback(rr2)
		return nil
	})

	var seen int
	err = ns.AXFR(context.Background(), "example", func(_ dns.RR) bool {
		seen++
		return true
	}, "CH")
	if err != nil {
		t.Fatalf("axfr hook: %v", err)
	}
	if seen != 2 {
		t.Fatalf("expected 2 records, got %d", seen)
	}
}

func TestAXFRNoNetwork(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.23", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	defer profile.ResetEffective()
	profile.Effective().NoNetwork = true

	err = ns.AXFR(context.Background(), "example", nil, "")
	if err == nil {
		t.Fatalf("expected error when no_network is set")
	}
	if !strings.Contains(err.Error(), "External AXFR query") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAXFRIPv4Disabled(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.24", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	defer profile.ResetEffective()
	profile.Effective().Net.IPv4 = false

	var called int
	ns.SetAXFRHook(func(_ context.Context, _ string, _ func(dns.RR) bool, _ string) error {
		called++
		return nil
	})

	err = ns.AXFR(context.Background(), "example", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != 0 {
		t.Fatalf("expected hook not called, got %d", called)
	}
}

func TestEmptyCache(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.25", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if ns.state == nil || ns.state.cache == nil {
		t.Fatalf("expected nameserver cache state")
	}

	cacheKey := "example-cache"
	ns.state.cache.set(cacheKey, &packet.Packet{Msg: new(dns.Msg)})
	if _, ok := ns.state.cache.get(cacheKey); !ok {
		t.Fatalf("expected cached packet")
	}

	nameKey := strings.ToLower(ns.Name.String())
	addrKey := ns.Address.String()
	if cached := cachedNameserver(nameKey, addrKey); cached == nil {
		t.Fatalf("expected nameserver cached")
	}

	EmptyCache()

	if cached := cachedNameserver(nameKey, addrKey); cached != nil {
		t.Fatalf("expected nameserver cache cleared")
	}
	if _, ok := ns.state.cache.get(cacheKey); ok {
		t.Fatalf("expected query cache cleared")
	}
}

func TestQueryLogging(t *testing.T) {
	// Reset log hook after test
	defer SetLogFunc(nil)

	ns, err := New("ns.example", "127.0.0.1", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	var loggedEntries []struct {
		tag  string
		args map[string]any
	}
	SetLogFunc(func(tag string, args map[string]any, module, testcase string) (any, error) {
		loggedEntries = append(loggedEntries, struct {
			tag  string
			args map[string]any
		}{tag, args})
		return nil, nil
	})

	// Use a very short timeout since we expect network failure
	timeout := 10 * time.Millisecond
	opts := &QueryOptions{Timeout: &timeout}

	// This query will likely fail due to no server at 127.0.0.1:53, but logging happens before exchange
	_, _ = ns.QueryWithOptions(context.Background(), "example.com", "SOA", opts)

	var foundQuery bool
	for _, entry := range loggedEntries {
		if entry.tag == "EXTERNAL_QUERY" {
			foundQuery = true
			loggedArgs := entry.args
			if name, ok := loggedArgs["name"]; !ok || name != "example.com" {
				t.Errorf("expected name=example.com, got %v", name)
			}
			if qtype, ok := loggedArgs["type"]; !ok || qtype != "SOA" {
				t.Errorf("expected type=SOA, got %v", qtype)
			}
			if ip, ok := loggedArgs["ip"]; !ok || ip != "127.0.0.1" {
				t.Errorf("expected ip=127.0.0.1, got %v", ip)
			}
			if flags, ok := loggedArgs["flags"]; !ok || flags != "{\"class\":\"IN\"}" {
				t.Errorf("expected flags={\"class\":\"IN\"}, got %v", flags)
			}
			break
		}
	}

	if !foundQuery {
		t.Errorf("expected EXTERNAL_QUERY tag, got %v", loggedEntries)
	}
}
