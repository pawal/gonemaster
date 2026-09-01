package nameserver

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

func TestBuildCacheKeyDefaultLayout(t *testing.T) {
	t.Parallel()

	key, ednsSize, dnssec, err := buildCacheKey("ExAmPlE.CoM.", "A", "IN", nil)
	if err != nil {
		t.Fatalf("buildCacheKey: %v", err)
	}
	if dnssec {
		t.Fatalf("dnssec = true, want false")
	}
	if ednsSize != 0 {
		t.Fatalf("ednsSize = %d, want 0", ednsSize)
	}

	expectedName := dnsname.New("ExAmPlE.CoM.").String()
	gotParts := strings.Split(key, "|")
	wantParts := []string{
		"NAME=" + expectedName,
		"TYPE=A",
		"CLASS=IN",
		"DNSSEC=false",
		"USEVC=false",
		"RECURSE=false",
		"EDNS_SIZE=0",
	}
	if !reflect.DeepEqual(gotParts, wantParts) {
		t.Fatalf("cache key parts mismatch:\n got: %v\nwant: %v", gotParts, wantParts)
	}
}

func TestBuildCacheKeyPreservesQNameCase(t *testing.T) {
	t.Parallel()

	mixedKey, _, _, err := buildCacheKey("ExAmPlE.CoM.", "A", "IN", nil)
	if err != nil {
		t.Fatalf("buildCacheKey mixed: %v", err)
	}
	lowerKey, _, _, err := buildCacheKey("example.com.", "A", "IN", nil)
	if err != nil {
		t.Fatalf("buildCacheKey lower: %v", err)
	}
	if mixedKey == lowerKey {
		t.Fatalf("cache key must preserve QNAME case:\n mixed: %s\n lower: %s", mixedKey, lowerKey)
	}
	if !strings.Contains(mixedKey, "NAME=ExAmPlE.CoM") {
		t.Fatalf("mixed-case key lost QNAME case: %q", mixedKey)
	}
}

func TestBuildCacheKeyWithEDNSDetailsLayout(t *testing.T) {
	t.Parallel()

	dnssec := true
	usevc := true
	recurse := true
	ednsSize := uint16(1232)
	ednsVersion := uint8(1)
	ednsZ := uint16(2)
	ednsRcode := uint8(3)
	data := []dns.EDNS0{&dns.NSID{Nsid: "beef"}}

	opts := &QueryOptions{
		DNSSEC:  &dnssec,
		UseVC:   &usevc,
		Recurse: &recurse,
		EDNSDetails: &transport.EDNSDetails{
			Size:    &ednsSize,
			Version: &ednsVersion,
			Z:       &ednsZ,
			Rcode:   &ednsRcode,
			Data:    data,
		},
	}

	key, gotSize, gotDNSSEC, err := buildCacheKey("example.com", "AAAA", "IN", opts)
	if err != nil {
		t.Fatalf("buildCacheKey: %v", err)
	}
	if !gotDNSSEC {
		t.Fatalf("dnssec = false, want true")
	}
	if gotSize != ednsSize {
		t.Fatalf("ednsSize = %d, want %d", gotSize, ednsSize)
	}

	gotParts := strings.Split(key, "|")
	wantOrder := []string{
		"NAME",
		"TYPE",
		"CLASS",
		"DNSSEC",
		"USEVC",
		"RECURSE",
		"EDNS_VERSION",
		"EDNS_Z",
		"EDNS_RCODE",
		"EDNS_DATA",
		"EDNS_SIZE",
	}
	if len(gotParts) != len(wantOrder) {
		t.Fatalf("parts length = %d, want %d (%v)", len(gotParts), len(wantOrder), gotParts)
	}
	parts := parseCacheKeyParts(t, key, wantOrder)
	if parts["TYPE"] != "AAAA" {
		t.Fatalf("TYPE = %q, want AAAA", parts["TYPE"])
	}
	if parts["DNSSEC"] != "true" {
		t.Fatalf("DNSSEC = %q, want true", parts["DNSSEC"])
	}
	if parts["USEVC"] != "true" {
		t.Fatalf("USEVC = %q, want true", parts["USEVC"])
	}
	if parts["RECURSE"] != "true" {
		t.Fatalf("RECURSE = %q, want true", parts["RECURSE"])
	}
	if parts["EDNS_VERSION"] != "1" {
		t.Fatalf("EDNS_VERSION = %q, want 1", parts["EDNS_VERSION"])
	}
	if parts["EDNS_Z"] != "2" {
		t.Fatalf("EDNS_Z = %q, want 2", parts["EDNS_Z"])
	}
	if parts["EDNS_RCODE"] != "3" {
		t.Fatalf("EDNS_RCODE = %q, want 3", parts["EDNS_RCODE"])
	}
	if parts["EDNS_DATA"] != formatEDNSData(data) {
		t.Fatalf("EDNS_DATA = %q, want %q", parts["EDNS_DATA"], formatEDNSData(data))
	}
	if parts["EDNS_SIZE"] != "1232" {
		t.Fatalf("EDNS_SIZE = %q, want 1232", parts["EDNS_SIZE"])
	}
}

func TestBuildCacheKeyEDNSDefaultsWithDetailsObject(t *testing.T) {
	t.Parallel()

	opts := &QueryOptions{
		EDNSDetails: &transport.EDNSDetails{},
	}
	key, ednsSize, dnssec, err := buildCacheKey("example.com", "A", "IN", opts)
	if err != nil {
		t.Fatalf("buildCacheKey: %v", err)
	}
	if dnssec {
		t.Fatalf("dnssec = true, want false")
	}
	if ednsSize != constants.EDNSUDPPayloadDefault {
		t.Fatalf("ednsSize = %d, want %d", ednsSize, constants.EDNSUDPPayloadDefault)
	}
	parts := parseCacheKeyParts(t, key, []string{
		"NAME",
		"TYPE",
		"CLASS",
		"DNSSEC",
		"USEVC",
		"RECURSE",
		"EDNS_VERSION",
		"EDNS_Z",
		"EDNS_RCODE",
		"EDNS_DATA",
		"EDNS_SIZE",
	})
	if parts["EDNS_VERSION"] != "0" || parts["EDNS_Z"] != "0" || parts["EDNS_RCODE"] != "0" || parts["EDNS_DATA"] != "" {
		t.Fatalf("unexpected default EDNS details: %+v", parts)
	}
}

func TestBuildCacheKeyEDNSSizeUsesDNSSECDefault(t *testing.T) {
	t.Parallel()

	do := true
	key, ednsSize, dnssec, err := buildCacheKey("example.com", "A", "IN", &QueryOptions{DNSSEC: &do})
	if err != nil {
		t.Fatalf("buildCacheKey: %v", err)
	}
	if !dnssec {
		t.Fatalf("dnssec = false, want true")
	}
	if ednsSize != constants.EDNSUDPPayloadDNSSECDefault {
		t.Fatalf("ednsSize = %d, want %d", ednsSize, constants.EDNSUDPPayloadDNSSECDefault)
	}

	parts := parseCacheKeyParts(t, key, []string{"NAME", "TYPE", "CLASS", "DNSSEC", "USEVC", "RECURSE", "EDNS_SIZE"})
	if parts["EDNS_SIZE"] != strconv.Itoa(constants.EDNSUDPPayloadDNSSECDefault) {
		t.Fatalf("EDNS_SIZE = %q, want %d", parts["EDNS_SIZE"], constants.EDNSUDPPayloadDNSSECDefault)
	}
}

func TestBuildCacheKeyConcurrentDeterministic(t *testing.T) {
	t.Parallel()

	do := true
	usevc := true
	recurse := true
	ednsSize := uint16(1410)
	opts := &QueryOptions{
		DNSSEC:  &do,
		UseVC:   &usevc,
		Recurse: &recurse,
		EDNSDetails: &transport.EDNSDetails{
			Size: &ednsSize,
		},
	}

	const goroutines = 48
	const iterations = 80
	keys := make(chan string, goroutines*iterations)
	errs := make(chan error, goroutines*iterations)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				key, _, _, err := buildCacheKey("concurrent.example", "AAAA", "IN", opts)
				if err != nil {
					errs <- err
					return
				}
				keys <- key
			}
		}()
	}
	wg.Wait()
	close(keys)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("buildCacheKey concurrent error: %v", err)
		}
	}

	var first string
	for key := range keys {
		if first == "" {
			first = key
			continue
		}
		if key != first {
			t.Fatalf("nondeterministic key under concurrency:\nfirst: %s\n got : %s", first, key)
		}
	}
}

func parseCacheKeyParts(t *testing.T, key string, expectedOrder []string) map[string]string {
	t.Helper()

	rawParts := strings.Split(key, "|")
	parts := make(map[string]string, len(rawParts))
	gotOrder := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		chunks := strings.SplitN(part, "=", 2)
		if len(chunks) != 2 {
			t.Fatalf("invalid part %q", part)
		}
		gotOrder = append(gotOrder, chunks[0])
		parts[chunks[0]] = chunks[1]
	}
	if !reflect.DeepEqual(gotOrder, expectedOrder) {
		t.Fatalf("part order mismatch:\n got: %v\nwant: %v", gotOrder, expectedOrder)
	}
	return parts
}

// A no-fallback probe wants the truncated answer a fallback-allowed query never
// returns, so every override present in the options must appear in the key.
func TestBuildCacheKeyTransportOverrideLayout(t *testing.T) {
	t.Parallel()

	fallback := false
	retry := 2
	retrans := 3 * time.Second
	timeout := 4 * time.Second

	opts := &QueryOptions{
		Fallback: &fallback,
		Retry:    &retry,
		Retrans:  &retrans,
		Timeout:  &timeout,
	}

	key, _, _, err := buildCacheKey("example.com", "MX", "IN", opts)
	if err != nil {
		t.Fatalf("buildCacheKey: %v", err)
	}

	gotParts := strings.Split(key, "|")
	wantParts := []string{
		"NAME=example.com",
		"TYPE=MX",
		"CLASS=IN",
		"DNSSEC=false",
		"USEVC=false",
		"RECURSE=false",
		"FALLBACK=false",
		"RETRY=2",
		"RETRANS=" + strconv.FormatInt(int64(retrans), 10),
		"TIMEOUT=" + strconv.FormatInt(int64(timeout), 10),
		"EDNS_SIZE=0",
	}
	if !reflect.DeepEqual(gotParts, wantParts) {
		t.Fatalf("cache key parts mismatch:\n got: %v\nwant: %v", gotParts, wantParts)
	}
}

// Each override must separate the entry from an otherwise identical query.
func TestBuildCacheKeyTransportOverridesAreDistinct(t *testing.T) {
	t.Parallel()

	fallbackOff, fallbackOn := false, true
	zeroRetry, oneRetry := 0, 1
	zeroDur, oneSec := time.Duration(0), time.Second

	cases := []struct {
		name string
		opts *QueryOptions
	}{
		{"no overrides", &QueryOptions{}},
		{"fallback off", &QueryOptions{Fallback: &fallbackOff}},
		{"fallback on", &QueryOptions{Fallback: &fallbackOn}},
		{"retry zero", &QueryOptions{Retry: &zeroRetry}},
		{"retry one", &QueryOptions{Retry: &oneRetry}},
		{"retrans zero", &QueryOptions{Retrans: &zeroDur}},
		{"retrans one second", &QueryOptions{Retrans: &oneSec}},
		{"timeout zero", &QueryOptions{Timeout: &zeroDur}},
		{"timeout one second", &QueryOptions{Timeout: &oneSec}},
	}

	seen := map[string]string{}
	for _, tc := range cases {
		key, _, _, err := buildCacheKey("example.com", "MX", "IN", tc.opts)
		if err != nil {
			t.Fatalf("%s: buildCacheKey: %v", tc.name, err)
		}
		if other, dup := seen[key]; dup {
			t.Errorf("%q and %q share cache key %q", tc.name, other, key)
			continue
		}
		seen[key] = tc.name
	}
}

// Saved cache files carry these keys, so a query with no override must keep the
// existing layout.
func TestBuildCacheKeyOmitsUnsetTransportOverrides(t *testing.T) {
	t.Parallel()

	dnssec := true
	nilOptsKey, _, _, err := buildCacheKey("example.com", "A", "IN", nil)
	if err != nil {
		t.Fatalf("buildCacheKey nil opts: %v", err)
	}
	for _, part := range []string{"FALLBACK", "RETRY", "RETRANS", "TIMEOUT"} {
		if strings.Contains(nilOptsKey, part) {
			t.Errorf("default key gained a %s field: %q", part, nilOptsKey)
		}
	}

	emptyOptsKey, _, _, err := buildCacheKey("example.com", "A", "IN", &QueryOptions{})
	if err != nil {
		t.Fatalf("buildCacheKey empty opts: %v", err)
	}
	if emptyOptsKey != nilOptsKey {
		t.Errorf("empty options changed the key:\n got %q\nwant %q", emptyOptsKey, nilOptsKey)
	}

	dnssecKey, _, _, err := buildCacheKey("example.com", "A", "IN", &QueryOptions{DNSSEC: &dnssec})
	if err != nil {
		t.Fatalf("buildCacheKey dnssec opts: %v", err)
	}
	wantDNSSECKey := "NAME=example.com|TYPE=A|CLASS=IN|DNSSEC=true|USEVC=false|RECURSE=false|EDNS_SIZE=" +
		strconv.Itoa(constants.EDNSUDPPayloadDNSSECDefault)
	if dnssecKey != wantDNSSECKey {
		t.Errorf("DNSSEC key layout changed:\n got %q\nwant %q", dnssecKey, wantDNSSECKey)
	}
}

// zone08 asks for MX with default options while zone09 asks the same server with
// fallback disabled. One shared entry serves whichever ran first to both.
func TestFallbackOverrideDoesNotShareCacheEntry(t *testing.T) {
	ctx, _ := testContext(t)

	const qname = "fallback-split.example"
	truncated := dnsutil.SetQuestion(&dns.Msg{}, dnsutil.Fqdn(qname), dns.TypeMX)
	truncated.Response = true
	truncated.Truncated = true

	full := dnsutil.SetQuestion(&dns.Msg{}, dnsutil.Fqdn(qname), dns.TypeMX)
	full.Response = true
	mx := &dns.MX{Hdr: dns.Header{Name: dnsutil.Fqdn(qname), Class: dns.ClassINET, TTL: 3600}}
	mx.Mx = dnsutil.Fqdn("mail." + qname)
	full.Answer = []dns.RR{mx}

	var calls int
	ns := hookedNS(t, CacheFromContext(ctx), "ns1.example", "192.0.2.53", func(_ context.Context, _ string, _ string, _ string, opts *QueryOptions) (packet.Packet, error) {
		calls++
		if opts != nil && opts.Fallback != nil && !*opts.Fallback {
			return packet.Packet{Msg: truncated}, nil
		}
		return packet.Packet{Msg: full}, nil
	})

	withFallback, err := ns.QueryWithOptions(ctx, qname, "MX", nil)
	if err != nil {
		t.Fatalf("default query: %v", err)
	}
	fallbackOff := false
	noFallback, err := ns.QueryWithOptions(ctx, qname, "MX", &QueryOptions{Fallback: &fallbackOff})
	if err != nil {
		t.Fatalf("no-fallback query: %v", err)
	}

	if calls != 2 {
		t.Fatalf("hook calls = %d, want 2: the no-fallback probe was served the cached fallback answer", calls)
	}
	if withFallback.Msg == nil || withFallback.Msg.Truncated {
		t.Errorf("default query got the truncated answer")
	}
	if noFallback.Msg == nil || !noFallback.Msg.Truncated {
		t.Errorf("no-fallback probe did not get the truncated answer")
	}
}

// A CD probe and a plain query differ in what the answer means at a validating
// resolver, so they must not share an entry.
func TestBuildCacheKeyCheckingDisabled(t *testing.T) {
	t.Parallel()

	cd := true
	noCD := false

	plain, _, _, err := buildCacheKey("example.com", "A", "IN", &QueryOptions{})
	if err != nil {
		t.Fatalf("buildCacheKey plain: %v", err)
	}
	if strings.Contains(plain, "CD") {
		t.Errorf("a query with no CD override gained a CD field: %q", plain)
	}

	withCD, _, _, err := buildCacheKey("example.com", "A", "IN", &QueryOptions{CheckingDisabled: &cd})
	if err != nil {
		t.Fatalf("buildCacheKey CD: %v", err)
	}
	withoutCD, _, _, err := buildCacheKey("example.com", "A", "IN", &QueryOptions{CheckingDisabled: &noCD})
	if err != nil {
		t.Fatalf("buildCacheKey explicit no-CD: %v", err)
	}

	if withCD == plain || withoutCD == plain || withCD == withoutCD {
		t.Errorf("CD keys collide:\n plain: %q\n cd: %q\n nocd: %q", plain, withCD, withoutCD)
	}
	if !strings.HasSuffix(withCD, "|CD=true|EDNS_SIZE=0") {
		t.Errorf("unexpected CD key layout: %q", withCD)
	}
}

// QueryOptions.CheckingDisabled must reach the transport client.
func TestQueryOptionsCheckingDisabledReachesClient(t *testing.T) {
	ctx, _ := testContext(t)

	ns := newNS(t, ctx, "ns1.example", "192.0.2.53")
	cd := true

	client, err := ns.clientForOptions(ctx, &QueryOptions{CheckingDisabled: &cd})
	if err != nil {
		t.Fatalf("clientForOptions: %v", err)
	}
	if !client.CheckingDisabled {
		t.Error("CheckingDisabled did not reach the client")
	}

	plain, err := ns.clientForOptions(ctx, nil)
	if err != nil {
		t.Fatalf("clientForOptions nil opts: %v", err)
	}
	if plain.CheckingDisabled {
		t.Error("a query with no CD override produced a CD client")
	}
}
