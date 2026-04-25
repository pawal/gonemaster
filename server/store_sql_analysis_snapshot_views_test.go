package server

import (
	"testing"
	"time"
)

// snapshotViewFixture seeds one (cohort, batch) snapshot with two domains:
//   - example.test: served by ns1 (192.0.2.1 ipv4, 2001:db8::1 ipv6) on AS 64496
//   - other.test:   served by ns1 (192.0.2.1) and ns2 (192.0.2.2) on AS 64497
//
// One out-of-batch run on a third domain proves scoping. Returns the cohort
// ID and the snapshot ID the views should target.
func snapshotViewFixture(t *testing.T, s *SQLJobStore) (cohortID, snapshotID int64) {
	t.Helper()
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)

	cohort, err := s.UpsertAnalysisCohort(AnalysisCohort{
		SourceType:      "tag",
		SourceTag:       "tld",
		Label:           "tld",
		AnalysisEnabled: true,
	})
	if err != nil {
		t.Fatalf("seed cohort: %v", err)
	}
	if err := s.CreateBatch(Batch{
		ID:             "batch-x",
		Tag:            "tld",
		CreatedAt:      now,
		DomainCount:    2,
		SnapshotIntent: true,
	}); err != nil {
		t.Fatalf("create batch: %v", err)
	}
	if err := s.CreateBatch(Batch{
		ID:             "batch-other",
		Tag:            "tld",
		CreatedAt:      now,
		DomainCount:    1,
		SnapshotIntent: true,
	}); err != nil {
		t.Fatalf("create batch-other: %v", err)
	}

	ns1, err := s.UpsertAnalysisNameserver("ns1.example", now)
	if err != nil {
		t.Fatalf("ns1: %v", err)
	}
	ns2, err := s.UpsertAnalysisNameserver("ns2.example", now)
	if err != nil {
		t.Fatalf("ns2: %v", err)
	}
	addr4a, err := s.UpsertAnalysisAddress("192.0.2.1", "ipv4", now)
	if err != nil {
		t.Fatalf("addr4a: %v", err)
	}
	addr4b, err := s.UpsertAnalysisAddress("192.0.2.2", "ipv4", now)
	if err != nil {
		t.Fatalf("addr4b: %v", err)
	}
	addr6, err := s.UpsertAnalysisAddress("2001:db8::1", "ipv6", now)
	if err != nil {
		t.Fatalf("addr6: %v", err)
	}
	pfx4, err := s.UpsertAnalysisPrefix("192.0.2.0/24", "ipv4", now)
	if err != nil {
		t.Fatalf("pfx4: %v", err)
	}
	pfx6, err := s.UpsertAnalysisPrefix("2001:db8::/32", "ipv6", now)
	if err != nil {
		t.Fatalf("pfx6: %v", err)
	}
	if _, err := s.UpsertAnalysisASN(64496, "Example AS", now); err != nil {
		t.Fatalf("asn 64496: %v", err)
	}
	if _, err := s.UpsertAnalysisASN(64497, "Other AS", now); err != nil {
		t.Fatalf("asn 64497: %v", err)
	}

	// Two domains in the snapshot batch + one out-of-batch domain that must
	// not leak into the views.
	domA, err := s.GetOrCreateDomain("example.test")
	if err != nil {
		t.Fatalf("domA: %v", err)
	}
	domB, err := s.GetOrCreateDomain("other.test")
	if err != nil {
		t.Fatalf("domB: %v", err)
	}
	domC, err := s.GetOrCreateDomain("leak.test")
	if err != nil {
		t.Fatalf("domC: %v", err)
	}

	for i, rec := range []struct {
		runID    string
		domainID int64
		domain   string
		batchID  string
	}{
		{"run-a", domA.ID, domA.Name, "batch-x"},
		{"run-b", domB.ID, domB.Name, "batch-x"},
		{"run-c", domC.ID, domC.Name, "batch-other"},
	} {
		job := Job{
			ID: rec.runID, DomainID: rec.domainID, Domain: rec.domain,
			BatchID: rec.batchID, Status: JobSucceeded,
			CreatedAt: now, StartedAt: now, FinishedAt: now.Add(time.Duration(i) * time.Minute),
		}
		if _, err := s.Create(job); err != nil {
			t.Fatalf("create job %s: %v", rec.runID, err)
		}
		if err := s.GraduateJob(job, nil); err != nil {
			t.Fatalf("graduate %s: %v", rec.runID, err)
		}
		if err := s.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
			CohortID: cohort.ID, RunID: rec.runID, DomainID: rec.domainID, WorstLevel: "OK",
		}); err != nil {
			t.Fatalf("summary %s: %v", rec.runID, err)
		}
	}

	// run-a: example.test served by ns1 on (v4 + v6), AS 64496.
	if err := s.ReplaceAnalysisRunNSEndpoints(cohort.ID, "run-a", []AnalysisRunNameserverEndpoint{
		{CohortID: cohort.ID, RunID: "run-a", DomainID: domA.ID, NameserverID: ns1.ID, AddressID: addr4a.ID, Role: "auth", Family: "ipv4", QueryCount: 3},
		{CohortID: cohort.ID, RunID: "run-a", DomainID: domA.ID, NameserverID: ns1.ID, AddressID: addr6.ID, Role: "auth", Family: "ipv6", QueryCount: 2},
	}); err != nil {
		t.Fatalf("ns endpoints run-a: %v", err)
	}
	asn64496 := int64(64496)
	pfx4ID := pfx4.ID
	pfx6ID := pfx6.ID
	if err := s.ReplaceAnalysisRunAddressASNs(cohort.ID, "run-a", []AnalysisRunAddressASN{
		{CohortID: cohort.ID, RunID: "run-a", DomainID: domA.ID, AddressID: addr4a.ID, PrefixID: &pfx4ID, ASN: &asn64496},
		{CohortID: cohort.ID, RunID: "run-a", DomainID: domA.ID, AddressID: addr6.ID, PrefixID: &pfx6ID, ASN: &asn64496},
	}); err != nil {
		t.Fatalf("address asns run-a: %v", err)
	}
	if err := s.ReplaceAnalysisRunDomainASNs(cohort.ID, "run-a", []AnalysisRunDomainASN{
		{CohortID: cohort.ID, RunID: "run-a", DomainID: domA.ID, ASN: 64496, Family: "ipv4"},
	}); err != nil {
		t.Fatalf("domain asns run-a: %v", err)
	}

	// run-b: other.test served by ns1 (192.0.2.1) and ns2 (192.0.2.2), AS 64497.
	asn64497 := int64(64497)
	if err := s.ReplaceAnalysisRunNSEndpoints(cohort.ID, "run-b", []AnalysisRunNameserverEndpoint{
		{CohortID: cohort.ID, RunID: "run-b", DomainID: domB.ID, NameserverID: ns1.ID, AddressID: addr4a.ID, Role: "auth", Family: "ipv4", QueryCount: 1},
		{CohortID: cohort.ID, RunID: "run-b", DomainID: domB.ID, NameserverID: ns2.ID, AddressID: addr4b.ID, Role: "auth", Family: "ipv4", QueryCount: 4},
		// Parent-role row that must be filtered out by Compute.
		{CohortID: cohort.ID, RunID: "run-b", DomainID: domB.ID, NameserverID: ns1.ID, AddressID: addr4a.ID, Role: "parent", Source: "delegation", Family: "ipv4"},
	}); err != nil {
		t.Fatalf("ns endpoints run-b: %v", err)
	}
	if err := s.ReplaceAnalysisRunAddressASNs(cohort.ID, "run-b", []AnalysisRunAddressASN{
		{CohortID: cohort.ID, RunID: "run-b", DomainID: domB.ID, AddressID: addr4a.ID, PrefixID: &pfx4ID, ASN: &asn64497},
		{CohortID: cohort.ID, RunID: "run-b", DomainID: domB.ID, AddressID: addr4b.ID, PrefixID: &pfx4ID, ASN: &asn64497},
	}); err != nil {
		t.Fatalf("address asns run-b: %v", err)
	}

	// run-c: out-of-batch domain. Different ASN that must not surface.
	asn65000 := int64(65000)
	if _, err := s.UpsertAnalysisASN(65000, "Leak AS", now); err != nil {
		t.Fatalf("asn 65000: %v", err)
	}
	if err := s.ReplaceAnalysisRunNSEndpoints(cohort.ID, "run-c", []AnalysisRunNameserverEndpoint{
		{CohortID: cohort.ID, RunID: "run-c", DomainID: domC.ID, NameserverID: ns2.ID, AddressID: addr4b.ID, Role: "auth", Family: "ipv4"},
	}); err != nil {
		t.Fatalf("ns endpoints run-c: %v", err)
	}
	if err := s.ReplaceAnalysisRunAddressASNs(cohort.ID, "run-c", []AnalysisRunAddressASN{
		{CohortID: cohort.ID, RunID: "run-c", DomainID: domC.ID, AddressID: addr4b.ID, ASN: &asn65000},
	}); err != nil {
		t.Fatalf("address asns run-c: %v", err)
	}

	snap, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
		CohortID: cohort.ID, BatchID: "batch-x", Slug: "2026-04-25",
		Status: AnalysisSnapshotStatusCaptured, IsPublic: true,
		CapturedAt: now, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("upsert snapshot: %v", err)
	}
	return cohort.ID, snap.ID
}

func TestComputeSnapshotEntityViewsScopedToBatch(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			cohortID, _ := snapshotViewFixture(t, s)

			views, err := s.ComputeSnapshotEntityViews(cohortID, "batch-x")
			if err != nil {
				t.Fatalf("ComputeSnapshotEntityViews: %v", err)
			}

			// Two nameservers (ns1 + ns2), parent-role must be excluded.
			if len(views.Nameservers) != 2 {
				t.Fatalf("nameservers = %d, want 2: %+v", len(views.Nameservers), views.Nameservers)
			}
			byNS := map[string]AnalysisSnapshotNameserverView{}
			for _, ns := range views.Nameservers {
				byNS[ns.NameserverName] = ns
			}
			ns1 := byNS["ns1.example"]
			if ns1.DomainCount != 2 {
				t.Errorf("ns1 domain_count = %d, want 2", ns1.DomainCount)
			}
			if ns1.EndpointCount != 2 {
				t.Errorf("ns1 endpoint_count = %d, want 2 (192.0.2.1 + 2001:db8::1)", ns1.EndpointCount)
			}
			if ns1.IPv4Count != 1 || ns1.IPv6Count != 1 {
				t.Errorf("ns1 family counts = (%d,%d), want (1,1)", ns1.IPv4Count, ns1.IPv6Count)
			}
			if ns1.ASNCount != 2 {
				t.Errorf("ns1 asn_count = %d, want 2", ns1.ASNCount)
			}
			if ns1.Operator != "Multiple" {
				t.Errorf("ns1 operator = %q, want Multiple", ns1.Operator)
			}
			if ns1.QueryCount != 6 { // 3 + 2 (run-a) + 1 (run-b)
				t.Errorf("ns1 query_count = %d, want 6", ns1.QueryCount)
			}

			ns2 := byNS["ns2.example"]
			if ns2.DomainCount != 1 {
				t.Errorf("ns2 domain_count = %d, want 1 (out-of-batch leak.test must not show)", ns2.DomainCount)
			}
			if ns2.OperatorASN == nil || *ns2.OperatorASN != 64497 {
				t.Errorf("ns2 OperatorASN = %v, want 64497", ns2.OperatorASN)
			}
			if ns2.Operator != "Other AS" {
				t.Errorf("ns2 operator = %q, want \"Other AS\"", ns2.Operator)
			}

			// Endpoints: (ns1, 192.0.2.1), (ns1, 2001:db8::1), (ns2, 192.0.2.2).
			if len(views.Endpoints) != 3 {
				t.Fatalf("endpoints = %d, want 3: %+v", len(views.Endpoints), views.Endpoints)
			}
			byEP := map[string]AnalysisSnapshotEndpointView{}
			for _, ep := range views.Endpoints {
				byEP[ep.NameserverName+"|"+ep.Address] = ep
			}
			ns1v4 := byEP["ns1.example|192.0.2.1"]
			if ns1v4.DomainCount != 2 {
				t.Errorf("ns1+v4 domain_count = %d, want 2", ns1v4.DomainCount)
			}
			// Two domains use this endpoint with different ASNs → not single → ASN must be nil.
			if ns1v4.ASN != nil {
				t.Errorf("ns1+v4 ASN = %v, want nil (mixed across domains)", *ns1v4.ASN)
			}
			ns1v6 := byEP["ns1.example|2001:db8::1"]
			if ns1v6.ASN == nil || *ns1v6.ASN != 64496 {
				t.Errorf("ns1+v6 ASN = %v, want 64496", ns1v6.ASN)
			}
			if ns1v6.Prefix != "2001:db8::/32" {
				t.Errorf("ns1+v6 prefix = %q, want 2001:db8::/32", ns1v6.Prefix)
			}

			// ASNs: 64496 (from run-a) + 64497 (from run-b) + the domain-only ASN 64496.
			// Out-of-batch ASN 65000 must not surface.
			if len(views.ASNs) != 2 {
				t.Fatalf("asns = %d, want 2: %+v", len(views.ASNs), views.ASNs)
			}
			byASN := map[int64]AnalysisSnapshotASNView{}
			for _, asn := range views.ASNs {
				byASN[asn.ASN] = asn
			}
			a := byASN[64496]
			if a.DomainCount != 1 || a.AddressCount != 2 || a.NameserverCount != 1 {
				t.Errorf("asn 64496 = %+v", a)
			}
			if a.IPv4Count != 1 || a.IPv6Count != 1 {
				t.Errorf("asn 64496 family counts = (%d,%d), want (1,1)", a.IPv4Count, a.IPv6Count)
			}
			if a.Label != "Example AS" {
				t.Errorf("asn 64496 label = %q, want \"Example AS\"", a.Label)
			}
			b := byASN[64497]
			if b.DomainCount != 1 || b.AddressCount != 2 || b.NameserverCount != 2 {
				t.Errorf("asn 64497 = %+v", b)
			}
			if _, leaked := byASN[65000]; leaked {
				t.Fatal("out-of-batch ASN 65000 leaked into views")
			}
		})
	}
}

func TestReplaceSnapshotEntityViewsRoundTrip(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			cohortID, snapID := snapshotViewFixture(t, s)

			views, err := s.ComputeSnapshotEntityViews(cohortID, "batch-x")
			if err != nil {
				t.Fatalf("compute: %v", err)
			}
			if err := s.ReplaceSnapshotEntityViews(snapID, views); err != nil {
				t.Fatalf("replace: %v", err)
			}

			// Read back and verify counts match.
			gotNS := s.ListSnapshotNameserverViews(snapID)
			if len(gotNS) != len(views.Nameservers) {
				t.Fatalf("listed nameservers = %d, want %d", len(gotNS), len(views.Nameservers))
			}
			for _, row := range gotNS {
				if row.SnapshotID != snapID {
					t.Errorf("ns row snapshot_id = %d, want %d", row.SnapshotID, snapID)
				}
			}
			gotEP := s.ListSnapshotEndpointViews(snapID)
			if len(gotEP) != len(views.Endpoints) {
				t.Fatalf("listed endpoints = %d, want %d", len(gotEP), len(views.Endpoints))
			}
			gotASN := s.ListSnapshotASNViews(snapID)
			if len(gotASN) != len(views.ASNs) {
				t.Fatalf("listed asns = %d, want %d", len(gotASN), len(views.ASNs))
			}

			// List ordering: domain_count DESC then a stable secondary sort.
			for i := 1; i < len(gotNS); i++ {
				if gotNS[i-1].DomainCount < gotNS[i].DomainCount {
					t.Errorf("ns rows not sorted by domain_count desc: %+v", gotNS)
				}
			}
		})
	}
}

func TestReplaceSnapshotEntityViewsIdempotent(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
	cohortID, snapID := snapshotViewFixture(t, s)

	views, err := s.ComputeSnapshotEntityViews(cohortID, "batch-x")
	if err != nil {
		t.Fatalf("compute first: %v", err)
	}
	if err := s.ReplaceSnapshotEntityViews(snapID, views); err != nil {
		t.Fatalf("replace first: %v", err)
	}
	first := s.ListSnapshotNameserverViews(snapID)

	// Run again. Should DELETE+INSERT, leaving the same row count.
	if err := s.ReplaceSnapshotEntityViews(snapID, views); err != nil {
		t.Fatalf("replace second: %v", err)
	}
	second := s.ListSnapshotNameserverViews(snapID)
	if len(second) != len(first) {
		t.Fatalf("row count after rerun = %d, want %d (idempotence broken)", len(second), len(first))
	}

	// Replace with empty: rows must be cleared.
	if err := s.ReplaceSnapshotEntityViews(snapID, SnapshotEntityViews{}); err != nil {
		t.Fatalf("replace empty: %v", err)
	}
	if rows := s.ListSnapshotNameserverViews(snapID); len(rows) != 0 {
		t.Fatalf("after empty replace: ns rows = %d, want 0", len(rows))
	}
	if rows := s.ListSnapshotEndpointViews(snapID); len(rows) != 0 {
		t.Fatalf("after empty replace: endpoint rows = %d, want 0", len(rows))
	}
	if rows := s.ListSnapshotASNViews(snapID); len(rows) != 0 {
		t.Fatalf("after empty replace: asn rows = %d, want 0", len(rows))
	}
}

func TestReplaceSnapshotEntityViewsScopedBySnapshotID(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
	cohortID, snapID := snapshotViewFixture(t, s)
	now := time.Now().UTC()

	other, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
		CohortID: cohortID, BatchID: "batch-other", Slug: "other-slug",
		Status: AnalysisSnapshotStatusCaptured, IsPublic: true,
		CapturedAt: now, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("upsert other snapshot: %v", err)
	}

	views, err := s.ComputeSnapshotEntityViews(cohortID, "batch-x")
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if err := s.ReplaceSnapshotEntityViews(snapID, views); err != nil {
		t.Fatalf("replace primary: %v", err)
	}

	otherViews, err := s.ComputeSnapshotEntityViews(cohortID, "batch-other")
	if err != nil {
		t.Fatalf("compute other: %v", err)
	}
	if err := s.ReplaceSnapshotEntityViews(other.ID, otherViews); err != nil {
		t.Fatalf("replace other: %v", err)
	}

	// Re-replacing the other snapshot must not affect the primary's rows.
	if err := s.ReplaceSnapshotEntityViews(other.ID, SnapshotEntityViews{}); err != nil {
		t.Fatalf("clear other: %v", err)
	}
	if rows := s.ListSnapshotNameserverViews(snapID); len(rows) == 0 {
		t.Fatal("primary snapshot's view rows were wiped when clearing the other snapshot")
	}
}

func TestComputeSnapshotEntityViewsRequiresBatchID(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
	if _, err := s.ComputeSnapshotEntityViews(1, ""); err == nil {
		t.Fatal("expected error when batchID is empty")
	}
}
