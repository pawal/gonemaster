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
		t.Fatalf("timings len = %d, want 3 (%+v)", len(got), got)
	}
	// All three matched targets have samples → status=ok, sorted
	// slowest-avg first (ns1 at 30ms, ns2/v6 at 25, ns2/v4 at 15).
	for i, item := range got {
		if item.Status != NameserverTimingStatusOK {
			t.Fatalf("row %d status = %q, want ok", i, item.Status)
		}
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
	// Samples for a target that is not in the delegation list must be
	// dropped entirely (e.g. parent-side gtld-servers queries).
	for _, item := range got {
		if item.Nameserver == "a.gtld-servers.net" {
			t.Fatalf("non-target leaked into timings: %+v", item)
		}
	}
}

// TestSummarizeNameserverTimingsEmitsUnreachableForTargetWithoutSamples
// pins the .ck "circa" case: the delegation lists an (ns, addr) pair but
// the engine never got a successful sample (CN01/CN02 NO_RESPONSE).
// Previously the row was dropped; now it must surface as unreachable.
func TestSummarizeNameserverTimingsEmitsUnreachableForTargetWithoutSamples(t *testing.T) {
	queryTimings := map[string][]time.Duration{
		"ok.example.test/192.0.2.1": {10 * time.Millisecond},
	}
	targets := []nameserverTimingTarget{
		{name: "ok.example.test", address: "192.0.2.1"},
		{name: "dead.example.test", address: "192.0.2.99"},
	}
	got := summarizeNameserverTimings(queryTimings, targets)
	if len(got) != 2 {
		t.Fatalf("timings len = %d (%+v), want 2", len(got), got)
	}
	byNS := map[string]NameserverTiming{}
	for _, item := range got {
		byNS[item.Nameserver] = item
	}
	ok := byNS["ok.example.test"]
	if ok.Status != NameserverTimingStatusOK || ok.Count != 1 || ok.AvgMS == 0 {
		t.Fatalf("unexpected ok row: %+v", ok)
	}
	dead := byNS["dead.example.test"]
	if dead.Status != NameserverTimingStatusUnreachable {
		t.Fatalf("expected unreachable status for dead NS, got %+v", dead)
	}
	if dead.Address != "192.0.2.99" {
		t.Fatalf("expected unreachable row to keep its address, got %+v", dead)
	}
	if dead.Count != 0 || dead.AvgMS != 0 {
		t.Fatalf("unreachable row should have zero stats, got %+v", dead)
	}
	// Problems sort to the top.
	if got[0].Nameserver != "dead.example.test" {
		t.Fatalf("expected unreachable first, got %+v", got[0])
	}
}

// TestSummarizeNameserverTimingsEmitsUnresolvedForNameOnlyTargetWithoutSamples
// pins the .ck "downstage" case: the delegation lists the NS name, but
// no address could be resolved (CAN_NOT_BE_RESOLVED). The target comes
// in name-only; we emit a single unresolved row.
func TestSummarizeNameserverTimingsEmitsUnresolvedForNameOnlyTargetWithoutSamples(t *testing.T) {
	queryTimings := map[string][]time.Duration{
		"other.example/192.0.2.1": {5 * time.Millisecond},
	}
	targets := []nameserverTimingTarget{
		{name: "ghost.example"}, // no address, no samples anywhere
	}
	got := summarizeNameserverTimings(queryTimings, targets)
	if len(got) != 1 {
		t.Fatalf("timings len = %d, want 1 (%+v)", len(got), got)
	}
	if got[0].Nameserver != "ghost.example" || got[0].Status != NameserverTimingStatusUnresolved {
		t.Fatalf("expected unresolved row for ghost, got %+v", got[0])
	}
	if got[0].Address != "" {
		t.Fatalf("unresolved row should have no address, got %q", got[0].Address)
	}
}

// TestSummarizeNameserverTimingsNameOnlyTargetWithSamples preserves the
// existing name-only fallback: if a target has no address but the engine
// probed some addresses for that name, emit one ok row per address.
func TestSummarizeNameserverTimingsNameOnlyTargetWithSamples(t *testing.T) {
	queryTimings := map[string][]time.Duration{
		"multi.example/192.0.2.1":   {10 * time.Millisecond},
		"multi.example/2001:db8::1": {12 * time.Millisecond},
	}
	targets := []nameserverTimingTarget{{name: "multi.example"}}
	got := summarizeNameserverTimings(queryTimings, targets)
	if len(got) != 2 {
		t.Fatalf("timings len = %d, want 2 (%+v)", len(got), got)
	}
	for _, item := range got {
		if item.Status != NameserverTimingStatusOK || item.Count == 0 {
			t.Fatalf("expected ok rows with samples, got %+v", item)
		}
	}
}
