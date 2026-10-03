package server

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// delegation01Entries is a run whose child set is ns1, ns2 and the unresolved ns9.
func delegation01Entries() []engine.LogEntry {
	return []engine.LogEntry{
		{Testcase: "Delegation01", Tag: "ENOUGH_NS_DEL", Args: map[string]any{
			"servers": []map[string]any{{"ns": "ns1.example.test"}, {"ns": "old.example.test"}},
		}},
		{Testcase: "Delegation01", Tag: "ENOUGH_NS_CHILD", Args: map[string]any{
			"servers": []map[string]any{{"ns": "NS1.example.test."}, {"ns": "ns2.example.test"}, {"ns": "ns9.example.test"}},
		}},
		{Testcase: "Delegation01", Tag: "ENOUGH_IPV4_NS_CHILD", Args: map[string]any{
			"servers": []map[string]any{{"ns": "ns1.example.test", "address": "192.0.2.1"}, {"ns": "ns2.example.test", "address": "192.0.2.2"}},
		}},
		{Testcase: "Delegation01", Tag: "NO_IPV6_NS_CHILD", Args: map[string]any{"count": 0}},
		{Testcase: "Basic02", Tag: "B02_AUTH_RESPONSE_SOA", Args: map[string]any{
			"servers": []map[string]any{{"ns": "a.root-servers.net", "address": "198.41.0.4"}},
		}},
	}
}

func targetKeys(targets []nameserverTimingTarget) map[string]bool {
	keys := map[string]bool{}
	for _, item := range targets {
		keys[item.name+"/"+item.address] = true
	}
	return keys
}

func TestChildNameserversFromEntriesReadsDelegation01ChildTags(t *testing.T) {
	got := childNameserversFromEntries(delegation01Entries())
	keys := targetKeys(got)
	want := []string{"ns1.example.test/192.0.2.1", "ns2.example.test/192.0.2.2", "ns9.example.test/"}
	for _, key := range want {
		if !keys[key] {
			t.Fatalf("missing %s in %+v", key, got)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("targets = %+v, want exactly %v", got, want)
	}
	if keys["old.example.test/"] {
		t.Fatal("ENOUGH_NS_DEL leaked a parent-only name into the targets")
	}
	if keys["ns1.example.test/"] {
		t.Fatal("name-only ns1 kept although its address is known")
	}
}

func TestCollectNameserverTimingsUsesRunEntriesBeforeLookup(t *testing.T) {
	called := false
	s := &Server{delegationLookup: func(context.Context, string) DelegationInfo {
		called = true
		return DelegationInfo{Nameservers: []DelegationNS{{NS: "ns7.example.test", IP: "192.0.2.7"}}}
	}}
	queryTimings := map[string][]time.Duration{
		"ns1.example.test/192.0.2.1":    {10 * time.Millisecond},
		"ns2.example.test/192.0.2.2":    {20 * time.Millisecond},
		"a.root-servers.net/198.41.0.4": {5 * time.Millisecond},
	}

	got := s.collectNameserverTimings(Job{Domain: "example.test"}, queryTimings, nil, nil, delegation01Entries())
	if called {
		t.Fatal("delegation lookup called although the run logged the child set")
	}
	byKey := map[string]NameserverTiming{}
	for _, item := range got {
		byKey[item.Nameserver+"/"+item.Address] = item
	}
	if len(got) != 3 {
		t.Fatalf("rows = %+v, want ns1, ns2 and unresolved ns9", got)
	}
	if byKey["ns1.example.test/192.0.2.1"].Status != NameserverTimingStatusOK {
		t.Fatalf("ns1 row = %+v, want ok", byKey["ns1.example.test/192.0.2.1"])
	}
	if byKey["ns9.example.test/"].Status != NameserverTimingStatusUnresolved {
		t.Fatalf("ns9 row = %+v, want unresolved", byKey["ns9.example.test/"])
	}
	if _, ok := byKey["a.root-servers.net/198.41.0.4"]; ok {
		t.Fatal("root server leaked into the timing rows")
	}
}

func TestCollectNameserverTimingsFallsBackToLookupWithoutChildTags(t *testing.T) {
	called := false
	s := &Server{delegationLookup: func(context.Context, string) DelegationInfo {
		called = true
		return DelegationInfo{Nameservers: []DelegationNS{{NS: "ns1.example.test", IP: "192.0.2.1"}}}
	}}
	entries := []engine.LogEntry{{Testcase: "Delegation01", Tag: "ENOUGH_NS_DEL", Args: map[string]any{
		"servers": []map[string]any{{"ns": "ns1.example.test"}},
	}}}
	queryTimings := map[string][]time.Duration{"ns1.example.test/192.0.2.1": {10 * time.Millisecond}}

	got := s.collectNameserverTimings(Job{Domain: "example.test"}, queryTimings, nil, nil, entries)
	if !called {
		t.Fatal("delegation lookup not called although the run logged no child set")
	}
	if len(got) != 1 || got[0].Nameserver != "ns1.example.test" || got[0].Status != NameserverTimingStatusOK {
		t.Fatalf("rows = %+v, want one ok row for ns1", got)
	}
}

func TestCollectNameserverTimingsWarnsWhenNoChildSetIsKnown(t *testing.T) {
	var buf bytes.Buffer
	s := &Server{
		logger:           slog.New(slog.NewTextHandler(&buf, nil)),
		delegationLookup: func(context.Context, string) DelegationInfo { return DelegationInfo{} },
	}
	queryTimings := map[string][]time.Duration{"ns1.example.test/192.0.2.1": {10 * time.Millisecond}}

	got := s.collectNameserverTimings(Job{ID: "job-1", Domain: "example.test"}, queryTimings, nil, nil, nil)
	if got != nil {
		t.Fatalf("rows = %+v, want nil", got)
	}
	logged := buf.String()
	if !strings.Contains(logged, "nameserver timings dropped") || !strings.Contains(logged, "domain=example.test") {
		t.Fatalf("log = %q, want a warning naming the domain", logged)
	}
}
