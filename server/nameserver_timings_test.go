package server

import (
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

func TestNameserverTimingTargetsPreferUndelegatedInput(t *testing.T) {
	job := Job{
		UndelegatedNS: []engine.UndelegatedNameserver{
			{Name: "ns1.example.test", IP: "192.0.2.1"},
			{Name: "ns2.example.test"},
		},
	}
	info := DelegationInfo{
		Nameservers: []DelegationNS{
			{NS: "ignored.example.test", IP: "192.0.2.9"},
		},
	}

	targets := nameserverTimingTargets(job, info)
	if len(targets) != 2 {
		t.Fatalf("targets len = %d, want 2", len(targets))
	}
	if targets[0].name != "ns1.example.test" || targets[0].address != "192.0.2.1" {
		t.Fatalf("unexpected first target: %+v", targets[0])
	}
	if targets[1].name != "ns2.example.test" || targets[1].address != "" {
		t.Fatalf("unexpected second target: %+v", targets[1])
	}
}

func TestChildNameserversFromEntriesCollectsChildSideOnly(t *testing.T) {
	entries := []engine.LogEntry{
		// Child-side: collected.
		{
			Tag: "B01_CHILD_FOUND",
			Args: map[string]any{
				"child_servers": []any{
					map[string]any{"ns": "NS1.CHILD.EXAMPLE.", "address": "192.0.2.1"},
					map[string]any{"ns": "ns2.child.example", "address": "2001:db8::2"},
				},
			},
		},
		// Different child-side key shape: also collected, dedup by (ns, addr).
		{
			Args: map[string]any{
				"zone_servers": []any{
					map[string]any{"ns": "ns1.child.example.", "address": "192.0.2.1"},
					map[string]any{"ns": "ns3.child.example", "address": "192.0.2.3"},
				},
			},
		},
		// Parent-side: must not leak into targets.
		{
			Args: map[string]any{
				"parent_servers": []any{
					map[string]any{"ns": "a.root-servers.net", "address": "198.41.0.4"},
				},
			},
		},
		// Ambiguous generic `servers`: also ignored by the fallback.
		{
			Args: map[string]any{
				"servers": []any{
					map[string]any{"ns": "ambiguous.example", "address": "203.0.113.1"},
				},
			},
		},
	}

	got := childNameserversFromEntries(entries)
	if len(got) != 3 {
		t.Fatalf("targets len = %d (%+v), want 3", len(got), got)
	}
	byName := map[string]nameserverTimingTarget{}
	for _, tg := range got {
		byName[tg.name+"/"+tg.address] = tg
	}
	if _, ok := byName["ns1.child.example/192.0.2.1"]; !ok {
		t.Fatalf("missing ns1.child.example/192.0.2.1 in %+v", got)
	}
	if _, ok := byName["ns2.child.example/2001:db8::2"]; !ok {
		t.Fatalf("missing ns2.child.example/2001:db8::2 in %+v", got)
	}
	if _, ok := byName["ns3.child.example/192.0.2.3"]; !ok {
		t.Fatalf("missing ns3.child.example/192.0.2.3 in %+v", got)
	}
	if _, ok := byName["a.root-servers.net/198.41.0.4"]; ok {
		t.Fatal("parent_servers leaked into fallback targets")
	}
	if _, ok := byName["ambiguous.example/203.0.113.1"]; ok {
		t.Fatal("ambiguous servers entry leaked into fallback targets")
	}
}

func TestSummarizeNameserverTimingsFiltersAndSorts(t *testing.T) {
	queryTimings := map[string][]time.Duration{
		"ns1.example.test/192.0.2.1":    {20 * time.Millisecond, 40 * time.Millisecond},
		"ns2.example.test/192.0.2.2":    {15 * time.Millisecond},
		"ns2.example.test/2001:db8::2":  {25 * time.Millisecond},
		"a.gtld-servers.net/192.5.6.30": {100 * time.Millisecond},
	}
	targets := []nameserverTimingTarget{
		{name: "ns1.example.test", address: "192.0.2.1"},
		{name: "ns2.example.test"},
	}

	got := summarizeNameserverTimings(queryTimings, targets)
	if len(got) != 3 {
		t.Fatalf("timings len = %d, want 3", len(got))
	}
	if got[0].Nameserver != "ns1.example.test" || got[0].AvgMS != 30 {
		t.Fatalf("unexpected first timing: %+v", got[0])
	}
	if got[1].Nameserver != "ns2.example.test" || got[1].Address != "2001:db8::2" {
		t.Fatalf("unexpected second timing: %+v", got[1])
	}
	if got[2].Nameserver != "ns2.example.test" || got[2].Address != "192.0.2.2" {
		t.Fatalf("unexpected third timing: %+v", got[2])
	}
}
