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

	got := summarizeNameserverTimings(queryTimings, nil, nil, targets)
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
	got := summarizeNameserverTimings(queryTimings, nil, nil, targets)
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
	got := summarizeNameserverTimings(queryTimings, nil, nil, targets)
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

// TestSummarizeNameserverTimingsCarriesTimeoutCount pins the fix for the
// server path dropping queryTimeouts on the floor. The engine has always
// counted per-address timeouts; collectNameserverTimings received the map
// and the summarizer ignored it, so no stored run has ever carried the
// number. Three shapes are covered: an address that answered and also
// timed out (the rate-limit signature), an address in the delegation that
// only ever timed out, and a clean address that must stay at zero.
func TestSummarizeNameserverTimingsCarriesTimeoutCount(t *testing.T) {
	queryTimings := map[string][]time.Duration{
		"ns1.example.test/192.0.2.1": {20 * time.Millisecond, 40 * time.Millisecond},
		"ns3.example.test/192.0.2.3": {10 * time.Millisecond},
	}
	// Keys arrive from the engine unnormalized (trailing dot, mixed case);
	// the summarizer normalizes names on both sides or the counts miss.
	queryTimeouts := map[string]int{
		"NS1.example.test./192.0.2.1": 6,
		"ns2.example.test/192.0.2.2":  9,
	}
	targets := []nameserverTimingTarget{
		{name: "ns1.example.test", address: "192.0.2.1"},
		{name: "ns2.example.test", address: "192.0.2.2"},
		{name: "ns3.example.test", address: "192.0.2.3"},
	}

	got := summarizeNameserverTimings(queryTimings, queryTimeouts, nil, targets)
	byKey := map[string]NameserverTiming{}
	for _, item := range got {
		byKey[item.Nameserver+"/"+item.Address] = item
	}

	answered := byKey["ns1.example.test/192.0.2.1"]
	if answered.Status != NameserverTimingStatusOK || answered.TimeoutCount != 6 {
		t.Fatalf("answered-and-timed-out row = %+v, want ok with TimeoutCount 6", answered)
	}
	dead := byKey["ns2.example.test/192.0.2.2"]
	if dead.Status != NameserverTimingStatusUnreachable || dead.TimeoutCount != 9 {
		t.Fatalf("timeout-only row = %+v, want unreachable with TimeoutCount 9", dead)
	}
	if clean := byKey["ns3.example.test/192.0.2.3"]; clean.TimeoutCount != 0 {
		t.Fatalf("clean row = %+v, want TimeoutCount 0", clean)
	}
}

// TestSummarizeNameserverTimingsNameOnlyTargetDiscoversTimeoutOnlyAddress
// covers the name-only target path: a delegation that lists only the NS
// name, where one of its addresses answered and another never did. Before
// the timeout map was threaded through, the silent address produced no row
// at all, so the very addresses a rate limiter drops were the ones missing
// from the measurement.
func TestSummarizeNameserverTimingsNameOnlyTargetDiscoversTimeoutOnlyAddress(t *testing.T) {
	queryTimings := map[string][]time.Duration{
		"multi.example/192.0.2.1": {10 * time.Millisecond},
	}
	queryTimeouts := map[string]int{
		"multi.example/2001:db8::1": 4,
	}
	targets := []nameserverTimingTarget{{name: "multi.example"}}

	got := summarizeNameserverTimings(queryTimings, queryTimeouts, nil, targets)
	if len(got) != 2 {
		t.Fatalf("timings len = %d, want 2 (%+v)", len(got), got)
	}
	byAddr := map[string]NameserverTiming{}
	for _, item := range got {
		byAddr[item.Address] = item
	}
	if v4 := byAddr["192.0.2.1"]; v4.Status != NameserverTimingStatusOK || v4.TimeoutCount != 0 {
		t.Fatalf("v4 row = %+v, want ok with no timeouts", v4)
	}
	v6 := byAddr["2001:db8::1"]
	if v6.Status != NameserverTimingStatusUnreachable || v6.TimeoutCount != 4 {
		t.Fatalf("v6 row = %+v, want unreachable with TimeoutCount 4", v6)
	}
}

// TestSummarizeNameserverTimingsCarriesRefusedCount pins the REFUSED half
// of the load signal onto stored rows. Unlike timeouts, REFUSED never
// creates a row of its own: a refusing address answered, so it has timing
// samples and reaches the summarizer through the normal "ok" path. The
// interesting row is therefore a healthy-looking one - fast, plenty of
// samples, status ok - that is nonetheless refusing a share of the traffic.
func TestSummarizeNameserverTimingsCarriesRefusedCount(t *testing.T) {
	queryTimings := map[string][]time.Duration{
		"ns1.example.test/192.0.2.1": {20 * time.Millisecond, 40 * time.Millisecond},
		"ns2.example.test/192.0.2.2": {15 * time.Millisecond},
	}
	queryRefused := map[string]int{
		"ns1.example.test./192.0.2.1": 7,
	}
	targets := []nameserverTimingTarget{
		{name: "ns1.example.test", address: "192.0.2.1"},
		{name: "ns2.example.test", address: "192.0.2.2"},
	}

	got := summarizeNameserverTimings(queryTimings, nil, queryRefused, targets)
	byKey := map[string]NameserverTiming{}
	for _, item := range got {
		byKey[item.Nameserver+"/"+item.Address] = item
	}

	refusing := byKey["ns1.example.test/192.0.2.1"]
	if refusing.Status != NameserverTimingStatusOK || refusing.Count != 2 {
		t.Fatalf("refusing address must still be an ok row with its samples, got %+v", refusing)
	}
	if refusing.RefusedCount != 7 {
		t.Fatalf("RefusedCount = %d, want 7 (%+v)", refusing.RefusedCount, refusing)
	}
	if clean := byKey["ns2.example.test/192.0.2.2"]; clean.RefusedCount != 0 {
		t.Fatalf("clean row = %+v, want RefusedCount 0", clean)
	}
}

// TestSummarizeNameserverTimingsCombinesTimeoutAndRefused covers the shape
// the characterization is looking for on a loaded farm: one address both
// dropping queries and refusing others. The two counters are accumulated
// into the same row from two separate maps, so a keying slip on either side
// silently zeroes one of them.
func TestSummarizeNameserverTimingsCombinesTimeoutAndRefused(t *testing.T) {
	queryTimings := map[string][]time.Duration{
		"ns1.example.test/192.0.2.1": {20 * time.Millisecond},
	}
	queryTimeouts := map[string]int{"ns1.example.test/192.0.2.1": 5}
	queryRefused := map[string]int{"ns1.example.test/192.0.2.1": 3}
	targets := []nameserverTimingTarget{{name: "ns1.example.test", address: "192.0.2.1"}}

	got := summarizeNameserverTimings(queryTimings, queryTimeouts, queryRefused, targets)
	if len(got) != 1 {
		t.Fatalf("timings len = %d, want 1 (%+v)", len(got), got)
	}
	if got[0].TimeoutCount != 5 || got[0].RefusedCount != 3 {
		t.Fatalf("row = %+v, want TimeoutCount 5 and RefusedCount 3", got[0])
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
	got := summarizeNameserverTimings(queryTimings, nil, nil, targets)
	if len(got) != 2 {
		t.Fatalf("timings len = %d, want 2 (%+v)", len(got), got)
	}
	for _, item := range got {
		if item.Status != NameserverTimingStatusOK || item.Count == 0 {
			t.Fatalf("expected ok rows with samples, got %+v", item)
		}
	}
}
