package profile

import "testing"

func TestDefaultProfileLoads(t *testing.T) {
	p, err := Default()
	if err != nil {
		t.Fatalf("default profile: %v", err)
	}
	if !p.Net.IPv4 || !p.Net.IPv6 {
		t.Fatalf("expected IPv4/IPv6 enabled by default")
	}
	if p.Resolver.Defaults.Timeout != 5 {
		t.Fatalf("unexpected default timeout: %d", p.Resolver.Defaults.Timeout)
	}
	if p.TestLevels == nil {
		t.Fatalf("expected test levels to be loaded")
	}
	if p.TestCases == nil {
		t.Fatalf("expected test cases to be loaded")
	}
}

func TestSystemSeverityParity(t *testing.T) {
	p, err := Default()
	if err != nil {
		t.Fatalf("default profile: %v", err)
	}
	sys := p.TestLevels["SYSTEM"]
	if sys == nil {
		t.Fatal("SYSTEM test_levels section missing from default profile")
	}

	// Expected SYSTEM tag severities in the shipped default profile.
	// Prevents accidental severity drift.
	expected := map[string]string{
		"CACHED_RETURN":                 "DEBUG3",
		"EMPTY_RETURN":                  "DEBUG3",
		"EXTERNAL_RESPONSE":             "DEBUG3",
		"FAKE_PACKET_RETURNED":          "DEBUG3",
		"CACHE_CREATED":                 "DEBUG2",
		"CACHE_FETCHED":                 "DEBUG2",
		"ERROR_CACHE_SKIP":              "DEBUG2",
		"FAKE_DELEGATION_ADDED":         "DEBUG2",
		"FAKE_DELEGATION_RETURNED":      "DEBUG2",
		"FAKE_DELEGATION_TO_SELF":       "DEBUG2",
		"FAKE_DS_ADDED":                 "DEBUG2",
		"FAKE_DS_RETURNED":              "DEBUG2",
		"IPV4_BLOCKED":                  "DEBUG2",
		"IPV6_BLOCKED":                  "DEBUG2",
		"IS_REDIRECT":                   "DEBUG2",
		"LOOP_PROTECTION":               "DEBUG2",
		"NO_SUCH_NAME":                  "DEBUG2",
		"NO_SUCH_RECORD":                "DEBUG2",
		"NS_CREATED":                    "DEBUG2",
		"QUERY":                         "DEBUG2",
		"REACHABILITY_CACHE_SKIP":       "DEBUG2",
		"RECURSE":                       "DEBUG2",
		"RECURSE_QUERY":                 "DEBUG2",
		"RESTORED_NS_CACHE":             "DEBUG2",
		"SAVED_NS_CACHE":                "DEBUG2",
		"ASN_LOOKUP_SOURCE":             "DEBUG",
		"BLACKLISTING":                  "DEBUG",
		"DEPENDENCY_VERSION":            "DEBUG",
		"EXTERNAL_QUERY":                "DEBUG",
		"IS_BLACKLISTED":                "DEBUG",
		"LOGGER_CALLBACK_ERROR":         "DEBUG",
		"MODULE_END":                    "DEBUG",
		"MODULE_VERSION":                "DEBUG",
		"PACKET_BIG":                    "DEBUG",
		"SKIP_IPV4_DISABLED":            "DEBUG",
		"SKIP_IPV6_DISABLED":            "DEBUG",
		"START_TIME":                    "DEBUG",
		"TEST_TARGET":                   "DEBUG",
		"GLOBAL_VERSION":                "INFO",
		"IPV6_AUTO_DISABLED":            "INFO",
		"FAKE_DELEGATION_IN_ZONE_NO_IP": "ERROR",
		"FAKE_DELEGATION_NO_IP":         "ERROR",
		"CANNOT_CONTINUE":               "CRITICAL",
		"MODULE_ERROR":                  "CRITICAL",
		"NO_NETWORK":                    "CRITICAL",
		"UNKNOWN_METHOD":                "CRITICAL",
		"UNKNOWN_MODULE":                "CRITICAL",
	}

	for tag, want := range expected {
		got, ok := sys[tag]
		if !ok {
			t.Errorf("SYSTEM tag %s missing from profile (want %s)", tag, want)
		} else if got != want {
			t.Errorf("SYSTEM.%s = %s, want %s", tag, got, want)
		}
	}
}

func TestResetEffective(t *testing.T) {
	Effective().Net.IPv4 = false
	ResetEffective()
	if !Effective().Net.IPv4 {
		t.Fatalf("expected ResetEffective to restore defaults")
	}
}
