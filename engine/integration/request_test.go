package integration

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
)

func TestParseRequestRejectsUnknownFields(t *testing.T) {
	_, err := ParseRequest([]byte(`{"domain":"example.com","testcase":"basic01"}`))
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeInvalidRequest {
		t.Fatalf("ParseRequest = %v, want %s", err, CodeInvalidRequest)
	}
}

func TestParseRequestDecodesEveryField(t *testing.T) {
	doc := `{"domain":"example.com","module":"basic","testcases":["basic01"],"exclude":["basic02"],` +
		`"profile":{"net":{"ipv4":true}},"min_level":"info","ipv4":true,"ipv6":false,` +
		`"resolver":{"timeout":3,"retry":1,"retrans":2,"fallback":false,"parallel":4,"unordered":true,` +
		`"source4":"192.0.2.1","source6":"2001:db8::1","error_cache_ttl":5,"positive_cache_ttl":6,"negative_cache_ttl":7},` +
		`"undelegated_nameservers":[{"name":"ns1.example.com","ip":"192.0.2.53"}],` +
		`"undelegated_ds":[{"keytag":1,"algorithm":13,"digest_type":2,"digest":"AB"}],` +
		`"badkeys_path":"/tmp/badkeys","include_timings":true}`
	req, err := ParseRequest([]byte(doc))
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}
	if req.Domain != "example.com" || req.Module != "basic" || !slices.Equal(req.Testcases, []string{"basic01"}) || !slices.Equal(req.Exclude, []string{"basic02"}) {
		t.Fatalf("selection = %+v", req)
	}
	if string(req.Profile) != `{"net":{"ipv4":true}}` || req.MinLevel != "info" || *req.IPv4 != true || *req.IPv6 != false {
		t.Fatalf("profile and levels = %+v", req)
	}
	r := req.Resolver
	if *r.Timeout != 3 || *r.Retry != 1 || *r.Retrans != 2 || *r.Fallback != false || *r.Parallel != 4 || *r.Unordered != true {
		t.Fatalf("resolver = %+v", r)
	}
	if *r.Source4 != "192.0.2.1" || *r.Source6 != "2001:db8::1" || *r.ErrorCacheTTL != 5 || *r.PositiveCacheTTL != 6 || *r.NegativeCacheTTL != 7 {
		t.Fatalf("resolver addresses and ttls = %+v", r)
	}
	if !slices.Equal(req.UndelegatedNameservers, []UndelegatedNameserver{{Name: "ns1.example.com", IP: "192.0.2.53"}}) {
		t.Fatalf("undelegated nameservers = %+v", req.UndelegatedNameservers)
	}
	if !slices.Equal(req.UndelegatedDS, []UndelegatedDS{{KeyTag: 1, Algorithm: 13, DigestType: 2, Digest: "AB"}}) {
		t.Fatalf("undelegated ds = %+v", req.UndelegatedDS)
	}
	if req.BadkeysPath != "/tmp/badkeys" || !req.IncludeTimings {
		t.Fatalf("badkeys and timings = %+v", req)
	}
}

func TestRunRequestMapsFields(t *testing.T) {
	timeout, retry := 3, 1
	on, off := true, false
	source := "192.0.2.1"
	digest := strings.Repeat("ab", 20)
	req := Request{
		Domain:                 " Exämple.com ",
		Module:                 "Basic",
		Testcases:              []string{"basic01"},
		Exclude:                []string{"basic02"},
		Profile:                []byte(`{"net":{"ipv4":true},"test_cases":["basic01","nope"]}`),
		MinLevel:               "info",
		IPv4:                   &on,
		IPv6:                   &off,
		Resolver:               Resolver{Timeout: &timeout, Retry: &retry, Source4: &source},
		UndelegatedNameservers: []UndelegatedNameserver{{Name: "ns1.example.com", IP: "192.0.2.53"}},
		UndelegatedDS:          []UndelegatedDS{{KeyTag: 1, Algorithm: 13, DigestType: 2, Digest: digest}},
		BadkeysPath:            " /tmp/badkeys ",
	}
	got, err := req.runRequest()
	if err != nil {
		t.Fatalf("runRequest: %v", err)
	}
	if got.Domain != "xn--exmple-cua.com" || got.Module != "basic" || !slices.Equal(got.Testcases, []string{"basic01"}) || !slices.Equal(got.Exclude, []string{"basic02"}) {
		t.Fatalf("selection = %+v", got)
	}
	if got.MinLevel != "INFO" || got.CaptureMinLevel != "INFO" || got.ProfileData != string(req.Profile) {
		t.Fatalf("levels and profile = %q %q %q", got.MinLevel, got.CaptureMinLevel, got.ProfileData)
	}
	if *got.IPv4 != true || *got.IPv6 != false || *got.Timeout != 3 || *got.Retry != 1 || *got.SourceAddr4 != "192.0.2.1" || got.Retrans != nil {
		t.Fatalf("overrides = %+v", got)
	}
	if *got.BadkeysPath != "/tmp/badkeys" {
		t.Fatalf("badkeys path = %q", *got.BadkeysPath)
	}
	if !slices.Equal(got.UndelegatedNameservers, []engine.UndelegatedNameserver{{Name: "ns1.example.com", IP: "192.0.2.53"}}) {
		t.Fatalf("undelegated nameservers = %+v", got.UndelegatedNameservers)
	}
	if !slices.Equal(got.UndelegatedDSInfo, []engine.UndelegatedDSInfo{{KeyTag: 1, Algorithm: 13, DigestType: 2, Digest: digest}}) {
		t.Fatalf("undelegated ds = %+v", got.UndelegatedDSInfo)
	}
}

func TestRunRequestRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		req  Request
		code string
	}{
		{"empty domain", Request{}, CodeInvalidDomain},
		{"bad label", Request{Domain: "exa mple.com"}, CodeInvalidDomain},
		{"min level", Request{Domain: "example.com", MinLevel: "LOUD"}, CodeInvalidRequest},
		{"module", Request{Domain: "example.com", Module: "nope"}, CodeUnknownModule},
		{"profile property", Request{Domain: "example.com", Profile: []byte(`{"net":1}`)}, CodeInvalidProfile},
		{"profile json", Request{Domain: "example.com", Profile: []byte(`{`)}, CodeInvalidProfile},
		{"ds digest", Request{Domain: "example.com", UndelegatedDS: []UndelegatedDS{{KeyTag: 1, Algorithm: 13, DigestType: 2, Digest: "zz"}}}, CodeInvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.req.runRequest()
			var e *Error
			if !errors.As(err, &e) || e.Code != tc.code {
				t.Fatalf("runRequest = %v, want %s", err, tc.code)
			}
		})
	}
}
