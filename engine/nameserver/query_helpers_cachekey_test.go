package nameserver

import (
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
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

	expectedName := strings.ToLower(dnsname.New("ExAmPlE.CoM.").String())
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
