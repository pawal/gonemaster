package server

import (
	"testing"
	"time"
)

func intPtr(v int) *int { return &v }

func int64Ptr(v int64) *int64 { return &v }

func stringPtr(v string) *string { return &v }

func TestSQLJobStoreAnalysisEntityHelpers(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			early := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
			late := early.Add(2 * time.Hour)

			ns1, err := s.UpsertAnalysisNameserver("ns1.example.net", late)
			if err != nil {
				t.Fatalf("UpsertAnalysisNameserver: %v", err)
			}
			ns2, err := s.UpsertAnalysisNameserver("ns1.example.net", early)
			if err != nil {
				t.Fatalf("UpsertAnalysisNameserver second: %v", err)
			}
			if ns1.ID != ns2.ID {
				t.Fatalf("nameserver id changed: %d vs %d", ns1.ID, ns2.ID)
			}
			if !ns2.FirstSeenAt.Equal(early) || !ns2.LastSeenAt.Equal(late) {
				t.Fatalf("unexpected nameserver seen window: %+v", ns2)
			}
			if got, ok := s.GetAnalysisNameserverByName("ns1.example.net"); !ok || got.ID != ns1.ID {
				t.Fatalf("GetAnalysisNameserverByName failed: %+v ok=%v", got, ok)
			}

			addr1, err := s.UpsertAnalysisAddress("192.0.2.1", "ipv4", late)
			if err != nil {
				t.Fatalf("UpsertAnalysisAddress: %v", err)
			}
			addr2, err := s.UpsertAnalysisAddress("192.0.2.1", "ipv4", early)
			if err != nil {
				t.Fatalf("UpsertAnalysisAddress second: %v", err)
			}
			if addr1.ID != addr2.ID {
				t.Fatalf("address id changed: %d vs %d", addr1.ID, addr2.ID)
			}
			if !addr2.FirstSeenAt.Equal(early) || !addr2.LastSeenAt.Equal(late) {
				t.Fatalf("unexpected address seen window: %+v", addr2)
			}
			if got, ok := s.GetAnalysisAddressByAddress("192.0.2.1"); !ok || got.ID != addr1.ID {
				t.Fatalf("GetAnalysisAddressByAddress failed: %+v ok=%v", got, ok)
			}

			pfx1, err := s.UpsertAnalysisPrefix("192.0.2.0/24", "ipv4", late)
			if err != nil {
				t.Fatalf("UpsertAnalysisPrefix: %v", err)
			}
			pfx2, err := s.UpsertAnalysisPrefix("192.0.2.0/24", "ipv4", early)
			if err != nil {
				t.Fatalf("UpsertAnalysisPrefix second: %v", err)
			}
			if pfx1.ID != pfx2.ID {
				t.Fatalf("prefix id changed: %d vs %d", pfx1.ID, pfx2.ID)
			}
			if !pfx2.FirstSeenAt.Equal(early) || !pfx2.LastSeenAt.Equal(late) {
				t.Fatalf("unexpected prefix seen window: %+v", pfx2)
			}
			if got, ok := s.GetAnalysisPrefixByPrefix("192.0.2.0/24"); !ok || got.ID != pfx1.ID {
				t.Fatalf("GetAnalysisPrefixByPrefix failed: %+v ok=%v", got, ok)
			}

			asn1, err := s.UpsertAnalysisASN(64496, "Example ASN", early)
			if err != nil {
				t.Fatalf("UpsertAnalysisASN: %v", err)
			}
			asn2, err := s.UpsertAnalysisASN(64496, "Example ASN updated", late)
			if err != nil {
				t.Fatalf("UpsertAnalysisASN second: %v", err)
			}
			if asn1.ASN != asn2.ASN {
				t.Fatalf("asn changed: %d vs %d", asn1.ASN, asn2.ASN)
			}
			if asn2.Label != "Example ASN updated" {
				t.Fatalf("expected ASN label update, got %q", asn2.Label)
			}
			if !asn2.FirstSeenAt.Equal(early) || !asn2.LastSeenAt.Equal(late) {
				t.Fatalf("unexpected ASN seen window: %+v", asn2)
			}
		})
	}
}

// TestSQLJobStoreAnalysisEntityUpsertNoOp covers the rebuild hot path:
// when the seen window already covers the new seenAt, the upsert skips
// the UPDATE (avoiding MVCC bloat on PostgreSQL) but still returns the
// existing row's identity.
func TestSQLJobStoreAnalysisEntityUpsertNoOp(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			early := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
			late := early.Add(2 * time.Hour)
			mid := early.Add(1 * time.Hour)

			// Seed each entity with the full early..late window.
			if _, err := s.UpsertAnalysisNameserver("ns1.example.net", late); err != nil {
				t.Fatalf("seed nameserver late: %v", err)
			}
			ns0, err := s.UpsertAnalysisNameserver("ns1.example.net", early)
			if err != nil {
				t.Fatalf("seed nameserver early: %v", err)
			}
			if _, err := s.UpsertAnalysisAddress("192.0.2.1", "ipv4", late); err != nil {
				t.Fatalf("seed address late: %v", err)
			}
			addr0, err := s.UpsertAnalysisAddress("192.0.2.1", "ipv4", early)
			if err != nil {
				t.Fatalf("seed address early: %v", err)
			}
			if _, err := s.UpsertAnalysisPrefix("192.0.2.0/24", "ipv4", late); err != nil {
				t.Fatalf("seed prefix late: %v", err)
			}
			pfx0, err := s.UpsertAnalysisPrefix("192.0.2.0/24", "ipv4", early)
			if err != nil {
				t.Fatalf("seed prefix early: %v", err)
			}
			if _, err := s.UpsertAnalysisASN(64496, "Example ASN", late); err != nil {
				t.Fatalf("seed asn late: %v", err)
			}
			asn0, err := s.UpsertAnalysisASN(64496, "Example ASN", early)
			if err != nil {
				t.Fatalf("seed asn early: %v", err)
			}

			// A subsequent upsert with seenAt already inside the window
			// must return the same identity and an unchanged window.
			ns, err := s.UpsertAnalysisNameserver("ns1.example.net", mid)
			if err != nil {
				t.Fatalf("noop nameserver: %v", err)
			}
			if ns.ID != ns0.ID || !ns.FirstSeenAt.Equal(early) || !ns.LastSeenAt.Equal(late) {
				t.Fatalf("nameserver no-op upsert mutated state: %+v", ns)
			}

			addr, err := s.UpsertAnalysisAddress("192.0.2.1", "ipv4", mid)
			if err != nil {
				t.Fatalf("noop address: %v", err)
			}
			if addr.ID != addr0.ID || !addr.FirstSeenAt.Equal(early) || !addr.LastSeenAt.Equal(late) {
				t.Fatalf("address no-op upsert mutated state: %+v", addr)
			}

			pfx, err := s.UpsertAnalysisPrefix("192.0.2.0/24", "ipv4", mid)
			if err != nil {
				t.Fatalf("noop prefix: %v", err)
			}
			if pfx.ID != pfx0.ID || !pfx.FirstSeenAt.Equal(early) || !pfx.LastSeenAt.Equal(late) {
				t.Fatalf("prefix no-op upsert mutated state: %+v", pfx)
			}

			// ASN with empty label must not blank out the existing label
			// even when seenAt is already covered.
			asn, err := s.UpsertAnalysisASN(64496, "", mid)
			if err != nil {
				t.Fatalf("noop asn: %v", err)
			}
			if asn.ASN != asn0.ASN || asn.Label != "Example ASN" || !asn.FirstSeenAt.Equal(early) || !asn.LastSeenAt.Equal(late) {
				t.Fatalf("asn no-op upsert mutated state: %+v", asn)
			}
		})
	}
}

func TestSQLJobStoreAnalysisFactHelpers(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			cohort1, err := s.UpsertAnalysisCohort(AnalysisCohort{
				SourceType:      "tag",
				SourceTag:       "tld",
				Label:           "TLD",
				AnalysisEnabled: true,
			})
			if err != nil {
				t.Fatalf("UpsertAnalysisCohort cohort1: %v", err)
			}
			cohort2, err := s.UpsertAnalysisCohort(AnalysisCohort{
				SourceType:      "tag",
				SourceTag:       "gov",
				Label:           "Government",
				AnalysisEnabled: true,
			})
			if err != nil {
				t.Fatalf("UpsertAnalysisCohort cohort2: %v", err)
			}

			ns, _ := s.UpsertAnalysisNameserver("ns1.example.net", time.Now().UTC())
			addr, _ := s.UpsertAnalysisAddress("192.0.2.1", "ipv4", time.Now().UTC())
			addr2, _ := s.UpsertAnalysisAddress("2001:db8::1", "ipv6", time.Now().UTC())
			pfx, _ := s.UpsertAnalysisPrefix("192.0.2.0/24", "ipv4", time.Now().UTC())

			if err := s.ReplaceAnalysisRunNSEndpoints(cohort1.ID, "run-1", []AnalysisRunNameserverEndpoint{
				{
					DomainID:     100,
					NameserverID: ns.ID,
					AddressID:    addr.ID,
					Role:         "authoritative",
					Source:       "timings",
					Family:       "ipv4",
					AvgMS:        12.5,
					MinMS:        10,
					MaxMS:        15,
					QueryCount:   3,
				},
				{
					DomainID:     100,
					NameserverID: ns.ID,
					AddressID:    addr2.ID,
					Role:         "authoritative",
					Source:       "timings",
					Family:       "ipv6",
					AvgMS:        18,
					MinMS:        17,
					MaxMS:        20,
					QueryCount:   2,
				},
			}); err != nil {
				t.Fatalf("ReplaceAnalysisRunNSEndpoints cohort1 initial: %v", err)
			}
			if err := s.ReplaceAnalysisRunNSEndpoints(cohort2.ID, "run-1", []AnalysisRunNameserverEndpoint{
				{
					DomainID:     200,
					NameserverID: ns.ID,
					AddressID:    addr.ID,
					Role:         "authoritative",
					Source:       "entries",
					Family:       "ipv4",
					QueryCount:   1,
				},
			}); err != nil {
				t.Fatalf("ReplaceAnalysisRunNSEndpoints cohort2: %v", err)
			}

			gotC1 := s.ListAnalysisRunNSEndpoints(cohort1.ID, "run-1")
			if len(gotC1) != 2 {
				t.Fatalf("expected 2 ns endpoint rows for cohort1, got %d", len(gotC1))
			}
			gotC2 := s.ListAnalysisRunNSEndpoints(cohort2.ID, "run-1")
			if len(gotC2) != 1 {
				t.Fatalf("expected 1 ns endpoint row for cohort2, got %d", len(gotC2))
			}

			if err := s.ReplaceAnalysisRunNSEndpoints(cohort1.ID, "run-1", []AnalysisRunNameserverEndpoint{
				{
					DomainID:     100,
					NameserverID: ns.ID,
					AddressID:    addr.ID,
					Role:         "authoritative",
					Source:       "timings",
					Family:       "ipv4",
					AvgMS:        9,
					MinMS:        8,
					MaxMS:        11,
					QueryCount:   4,
				},
			}); err != nil {
				t.Fatalf("ReplaceAnalysisRunNSEndpoints cohort1 replace: %v", err)
			}
			gotC1 = s.ListAnalysisRunNSEndpoints(cohort1.ID, "run-1")
			if len(gotC1) != 1 || gotC1[0].QueryCount != 4 {
				t.Fatalf("expected replaced ns endpoint rows for cohort1, got %+v", gotC1)
			}
			if len(s.ListAnalysisRunNSEndpoints(cohort2.ID, "run-1")) != 1 {
				t.Fatal("cohort2 ns endpoint rows were unexpectedly modified")
			}

			if err := s.ReplaceAnalysisRunAddressASNs(cohort1.ID, "run-1", []AnalysisRunAddressASN{
				{
					DomainID:     100,
					AddressID:    addr.ID,
					PrefixID:     int64Ptr(pfx.ID),
					ASN:          int64Ptr(64496),
					LookupStatus: "ok",
					Source:       "bgp",
				},
				{
					DomainID:     100,
					AddressID:    addr2.ID,
					LookupStatus: "missing",
					Source:       "bgp",
				},
			}); err != nil {
				t.Fatalf("ReplaceAnalysisRunAddressASNs: %v", err)
			}
			addrASNRows := s.ListAnalysisRunAddressASNs(cohort1.ID, "run-1")
			if len(addrASNRows) != 2 {
				t.Fatalf("expected 2 address/asn rows, got %d", len(addrASNRows))
			}
			if addrASNRows[0].ASN == nil || *addrASNRows[0].ASN != 64496 {
				t.Fatalf("expected ASN in first row, got %+v", addrASNRows[0])
			}
			if addrASNRows[1].ASN != nil {
				t.Fatalf("expected nil ASN in second row, got %+v", addrASNRows[1])
			}

			if err := s.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
				CohortID:        cohort1.ID,
				RunID:           "run-1",
				DomainID:        100,
				Score:           intPtr(88),
				Grade:           stringPtr("B"),
				NameserverCount: 2,
				EndpointCount:   3,
				ASNCount:        1,
				PrefixCount:     1,
				WorstLevel:      "WARNING",
			}); err != nil {
				t.Fatalf("UpsertAnalysisRunDomainSummary insert: %v", err)
			}
			if err := s.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
				CohortID:        cohort1.ID,
				RunID:           "run-1",
				DomainID:        100,
				Score:           intPtr(91),
				Grade:           stringPtr("A"),
				NameserverCount: 1,
				EndpointCount:   2,
				ASNCount:        1,
				PrefixCount:     1,
				WorstLevel:      "NOTICE",
			}); err != nil {
				t.Fatalf("UpsertAnalysisRunDomainSummary update: %v", err)
			}
			summary, ok := s.GetAnalysisRunDomainSummary(cohort1.ID, "run-1", 100)
			if !ok {
				t.Fatal("expected analysis run domain summary")
			}
			if summary.Score == nil || *summary.Score != 91 || summary.Grade == nil || *summary.Grade != "A" {
				t.Fatalf("unexpected updated summary: %+v", summary)
			}

			projectedAt := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
			if err := s.SetAnalysisProjectionState(AnalysisProjectionState{
				CohortID:         cohort1.ID,
				RunID:            "run-1",
				ProjectorVersion: "v1",
				Status:           "ready",
				ProjectedAt:      projectedAt,
			}); err != nil {
				t.Fatalf("SetAnalysisProjectionState insert: %v", err)
			}
			if err := s.SetAnalysisProjectionState(AnalysisProjectionState{
				CohortID:         cohort1.ID,
				RunID:            "run-1",
				ProjectorVersion: "v2",
				Status:           "failed",
				ProjectedAt:      projectedAt.Add(1 * time.Hour),
				Error:            "boom",
			}); err != nil {
				t.Fatalf("SetAnalysisProjectionState update: %v", err)
			}
			state, ok := s.GetAnalysisProjectionState(cohort1.ID, "run-1")
			if !ok {
				t.Fatal("expected analysis projection state")
			}
			if state.ProjectorVersion != "v2" || state.Status != "failed" || state.Error != "boom" {
				t.Fatalf("unexpected projection state: %+v", state)
			}
		})
	}
}

func TestSQLJobStoreClearAnalysisCohortMaterialization(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
			cohort1, err := s.UpsertAnalysisCohort(AnalysisCohort{
				SourceType:      "tag",
				SourceTag:       "tld",
				Label:           "TLD",
				AnalysisEnabled: true,
			})
			if err != nil {
				t.Fatalf("UpsertAnalysisCohort cohort1: %v", err)
			}
			cohort2, err := s.UpsertAnalysisCohort(AnalysisCohort{
				SourceType:      "tag",
				SourceTag:       "gov",
				Label:           "Government",
				AnalysisEnabled: true,
			})
			if err != nil {
				t.Fatalf("UpsertAnalysisCohort cohort2: %v", err)
			}

			ns, _ := s.UpsertAnalysisNameserver("ns1.example.net", time.Now().UTC())
			addr, _ := s.UpsertAnalysisAddress("192.0.2.1", "ipv4", time.Now().UTC())
			pfx, _ := s.UpsertAnalysisPrefix("192.0.2.0/24", "ipv4", time.Now().UTC())

			if err := s.ReplaceAnalysisRunNSEndpoints(cohort1.ID, "run-1", []AnalysisRunNameserverEndpoint{{
				DomainID:     100,
				NameserverID: ns.ID,
				AddressID:    addr.ID,
				Role:         "authoritative",
				Source:       "timings",
				Family:       "ipv4",
			}}); err != nil {
				t.Fatalf("ReplaceAnalysisRunNSEndpoints cohort1: %v", err)
			}
			if err := s.ReplaceAnalysisRunNSEndpoints(cohort2.ID, "run-1", []AnalysisRunNameserverEndpoint{{
				DomainID:     200,
				NameserverID: ns.ID,
				AddressID:    addr.ID,
				Role:         "authoritative",
				Source:       "timings",
				Family:       "ipv4",
			}}); err != nil {
				t.Fatalf("ReplaceAnalysisRunNSEndpoints cohort2: %v", err)
			}
			if err := s.ReplaceAnalysisRunAddressASNs(cohort1.ID, "run-1", []AnalysisRunAddressASN{{
				DomainID:     100,
				AddressID:    addr.ID,
				PrefixID:     int64Ptr(pfx.ID),
				ASN:          int64Ptr(64496),
				LookupStatus: "ok",
			}}); err != nil {
				t.Fatalf("ReplaceAnalysisRunAddressASNs cohort1: %v", err)
			}
			if err := s.ReplaceAnalysisRunAddressASNs(cohort2.ID, "run-1", []AnalysisRunAddressASN{{
				DomainID:     200,
				AddressID:    addr.ID,
				LookupStatus: "ok",
			}}); err != nil {
				t.Fatalf("ReplaceAnalysisRunAddressASNs cohort2: %v", err)
			}
			if err := s.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
				CohortID:        cohort1.ID,
				RunID:           "run-1",
				DomainID:        100,
				NameserverCount: 1,
				EndpointCount:   1,
			}); err != nil {
				t.Fatalf("UpsertAnalysisRunDomainSummary cohort1: %v", err)
			}
			if err := s.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
				CohortID:        cohort2.ID,
				RunID:           "run-1",
				DomainID:        200,
				NameserverCount: 1,
				EndpointCount:   1,
			}); err != nil {
				t.Fatalf("UpsertAnalysisRunDomainSummary cohort2: %v", err)
			}
			if err := s.SetAnalysisProjectionState(AnalysisProjectionState{
				CohortID:         cohort1.ID,
				RunID:            "run-1",
				ProjectorVersion: "v1",
				Status:           AnalysisMaterializationReady,
				ProjectedAt:      time.Now().UTC(),
			}); err != nil {
				t.Fatalf("SetAnalysisProjectionState cohort1: %v", err)
			}
			if err := s.SetAnalysisProjectionState(AnalysisProjectionState{
				CohortID:         cohort2.ID,
				RunID:            "run-1",
				ProjectorVersion: "v1",
				Status:           AnalysisMaterializationReady,
				ProjectedAt:      time.Now().UTC(),
			}); err != nil {
				t.Fatalf("SetAnalysisProjectionState cohort2: %v", err)
			}

			if err := s.ClearAnalysisCohortMaterialization(cohort1.ID); err != nil {
				t.Fatalf("ClearAnalysisCohortMaterialization: %v", err)
			}

			if got := s.ListAnalysisRunNSEndpoints(cohort1.ID, "run-1"); len(got) != 0 {
				t.Fatalf("expected cohort1 ns endpoints to be cleared, got %+v", got)
			}
			if got := s.ListAnalysisRunAddressASNs(cohort1.ID, "run-1"); len(got) != 0 {
				t.Fatalf("expected cohort1 address facts to be cleared, got %+v", got)
			}
			if _, ok := s.GetAnalysisRunDomainSummary(cohort1.ID, "run-1", 100); ok {
				t.Fatal("expected cohort1 summary to be cleared")
			}
			if _, ok := s.GetAnalysisProjectionState(cohort1.ID, "run-1"); ok {
				t.Fatal("expected cohort1 projection state to be cleared")
			}

			if got := s.ListAnalysisRunNSEndpoints(cohort2.ID, "run-1"); len(got) != 1 {
				t.Fatalf("expected cohort2 ns endpoints to remain, got %+v", got)
			}
			if got := s.ListAnalysisRunAddressASNs(cohort2.ID, "run-1"); len(got) != 1 {
				t.Fatalf("expected cohort2 address facts to remain, got %+v", got)
			}
			if _, ok := s.GetAnalysisRunDomainSummary(cohort2.ID, "run-1", 200); !ok {
				t.Fatal("expected cohort2 summary to remain")
			}
			if _, ok := s.GetAnalysisProjectionState(cohort2.ID, "run-1"); !ok {
				t.Fatal("expected cohort2 projection state to remain")
			}
		})
	}
}
