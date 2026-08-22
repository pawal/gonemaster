package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// seedDetailFixture populates two cohort domains with endpoints, addresses,
// ASNs, prefixes, and entries so every detail endpoint has real data.
func seedDetailFixture(t *testing.T, f *analysisFixture) *analysisFixture {
	t.Helper()
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

	// alpha.example: served by ns1.shared.example (v4+v6) on AS64500.
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
	})
	f.seedEndpoint("run-alpha.example-"+ts.Format("20060102150405"),
		"alpha.example", "ns1.shared.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedEndpoint("run-alpha.example-"+ts.Format("20060102150405"),
		"alpha.example", "ns1.shared.example", "2001:db8::10", "ipv6", ts, 64500, "2001:db8::/32")

	// beta.example: also on ns1.shared.example but only v4.
	f.seedGraduatedRun("beta.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})
	f.seedEndpoint("run-beta.example-"+ts.Format("20060102150405"),
		"beta.example", "ns1.shared.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")

	return f
}

func TestPublicAnalysisCohortDetail(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld")
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisCohortDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.DatasetTag != "tld" || !got.IsDefault {
			t.Fatalf("unexpected cohort: %+v", got)
		}
		if got.DomainCount != 2 {
			t.Fatalf("expected 2 domains, got %d", got.DomainCount)
		}
		if got.NameserverCount != 1 {
			t.Fatalf("expected 1 nameserver, got %d", got.NameserverCount)
		}
		if got.ASNCount != 1 {
			t.Fatalf("expected 1 ASN, got %d", got.ASNCount)
		}
	})
}

func TestPublicAnalysisCohortDetailSeverityDistribution(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

		// Three domains with distinct worst_level outcomes so each landing bucket
		// is exercised: ERROR, WARNING, and OK (no entries above INFO).
		f.seedGraduatedRun("err.example", ts, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})
		f.seedGraduatedRun("warn.example", ts, []engine.LogEntry{
			{Module: "DELEGATION", Testcase: "delegation02", Tag: "REFERRAL_SIZE_OK", Level: "WARNING"},
		})
		f.seedGraduatedRun("ok.example", ts, []engine.LogEntry{
			{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "INFO"},
		})

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld")
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisCohortDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.DomainCount != 3 {
			t.Fatalf("expected 3 domains, got %d", got.DomainCount)
		}
		severity, ok := got.FactDistributions[FactCategorySeverity]
		if !ok {
			t.Fatalf("expected severity distribution, got %+v", got.FactDistributions)
		}
		counts := map[string]int{}
		for _, b := range severity.Buckets {
			counts[b.Key] = b.Count
		}
		want := map[string]int{"ERROR": 1, "WARNING": 1, "OK": 1}
		for level, wantCount := range want {
			if counts[level] != wantCount {
				t.Fatalf("severity[%s] = %d, want %d (buckets: %+v)",
					level, counts[level], wantCount, severity.Buckets)
			}
		}
		for _, level := range []string{"NOTICE", "CRITICAL"} {
			if _, present := counts[level]; present {
				t.Fatalf("severity[%s] should be absent, got %+v", level, severity.Buckets)
			}
		}
	})
}

func TestPublicAnalysisCohortDetailFactDistributions(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

		// signed.example: signed zone publishing algo 13, grade A.
		run1 := f.seedGraduatedRun("signed.example", ts, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec05", Tag: "DS05_ALGO_OK", Level: "INFO"},
		})
		d1, _ := f.store.GetDomainByName("signed.example")
		one := int64(1)
		if err := f.store.ReplaceAnalysisRunDomainFacts(f.cohort.ID, run1.ID, []AnalysisRunDomainFact{
			{CohortID: f.cohort.ID, RunID: run1.ID, DomainID: d1.ID, Category: FactCategoryDNSSECPosture, Key: FactKeyNSEC3},
			{CohortID: f.cohort.ID, RunID: run1.ID, DomainID: d1.ID, Category: FactCategoryDNSKEYAlgorithm, Key: "13", ValueNum: &one},
			{CohortID: f.cohort.ID, RunID: run1.ID, DomainID: d1.ID, Category: FactCategoryGrade, Key: "A"},
		}); err != nil {
			t.Fatalf("replace domain facts: %v", err)
		}
		gradeA := "A"
		scoreA := 95
		if err := f.store.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
			CohortID: f.cohort.ID, RunID: run1.ID, DomainID: d1.ID,
			Score: &scoreA, Grade: &gradeA, WorstLevel: "OK",
		}); err != nil {
			t.Fatalf("upsert grade-A summary: %v", err)
		}

		// unsigned.example: unsigned zone, grade F.
		run2 := f.seedGraduatedRun("unsigned.example", ts, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})
		d2, _ := f.store.GetDomainByName("unsigned.example")
		if err := f.store.ReplaceAnalysisRunDomainFacts(f.cohort.ID, run2.ID, []AnalysisRunDomainFact{
			{CohortID: f.cohort.ID, RunID: run2.ID, DomainID: d2.ID, Category: FactCategoryDNSSECPosture, Key: FactKeyUnsigned},
			{CohortID: f.cohort.ID, RunID: run2.ID, DomainID: d2.ID, Category: FactCategoryGrade, Key: "F"},
		}); err != nil {
			t.Fatalf("replace domain facts: %v", err)
		}
		gradeF := "F"
		scoreF := 10
		if err := f.store.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
			CohortID: f.cohort.ID, RunID: run2.ID, DomainID: d2.ID,
			Score: &scoreF, Grade: &gradeF, WorstLevel: "ERROR",
		}); err != nil {
			t.Fatalf("upsert grade-F summary: %v", err)
		}
		f.refreshSnapshotViews(f.batchID)

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld")
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisCohortDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		posture, ok := got.FactDistributions[FactCategoryDNSSECPosture]
		if !ok {
			t.Fatalf("expected %q distribution, got %+v", FactCategoryDNSSECPosture, got.FactDistributions)
		}
		if len(posture.Buckets) != 2 {
			t.Fatalf("expected unsigned+nsec3 buckets, got %+v", posture.Buckets)
		}
		algo, ok := got.FactDistributions[FactCategoryDNSKEYAlgorithm]
		if !ok {
			t.Fatalf("expected dnskey_algo distribution, got %+v", got.FactDistributions)
		}
		if len(algo.Buckets) != 1 || algo.Buckets[0].Key != "13" || algo.Buckets[0].Count != 1 {
			t.Fatalf("expected single algo=13 bucket with count 1, got %+v", algo.Buckets)
		}
		grade, ok := got.FactDistributions[FactCategoryGrade]
		if !ok {
			t.Fatalf("expected grade distribution, got %+v", got.FactDistributions)
		}
		if len(grade.Buckets) != 2 {
			t.Fatalf("expected A + F grade buckets, got %+v", grade.Buckets)
		}
		// A before F (gradeOrder registry).
		if grade.Buckets[0].Key != "A" || grade.Buckets[1].Key != "F" {
			t.Fatalf("expected A,F ordering, got %+v", grade.Buckets)
		}
		if grade.Buckets[0].Tone != "ok" || grade.Buckets[1].Tone != "critical" {
			t.Fatalf("unexpected grade tones: %+v", grade.Buckets)
		}
	})
}

// TestPublicAnalysisCohortDetailFactDistributionsRedactInternalIDs pins
// the public shape of fact_distributions: the response builds from rows
// that carry cohort_id, run_id, domain_id, but those keys must collapse
// into counts before reaching the wire. The general redaction test seeds
// no fact rows, so this one specifically exercises the fact_distributions
// surface to keep an extractor or registry refactor from leaking IDs.
func TestPublicAnalysisCohortDetailFactDistributionsRedactInternalIDs(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

		run := f.seedGraduatedRun("signed.example", ts, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec05", Tag: "DS05_ALGO_OK", Level: "INFO"},
		})
		d, _ := f.store.GetDomainByName("signed.example")
		one := int64(1)
		if err := f.store.ReplaceAnalysisRunDomainFacts(f.cohort.ID, run.ID, []AnalysisRunDomainFact{
			{CohortID: f.cohort.ID, RunID: run.ID, DomainID: d.ID, Category: FactCategoryDNSSECPosture, Key: FactKeyNSEC3},
			{CohortID: f.cohort.ID, RunID: run.ID, DomainID: d.ID, Category: FactCategoryDNSKEYAlgorithm, Key: "13", ValueNum: &one},
			{CohortID: f.cohort.ID, RunID: run.ID, DomainID: d.ID, Category: FactCategoryGrade, Key: "A"},
		}); err != nil {
			t.Fatalf("replace domain facts: %v", err)
		}
		f.refreshSnapshotViews(f.batchID)

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld")
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisCohortDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got.FactDistributions) == 0 {
			t.Fatalf("fixture did not populate fact_distributions; redaction check would be vacuous")
		}

		body := resp.Body.String()
		for _, needle := range []string{
			`"cohort_id"`, `"run_id"`, `"domain_id"`, `"value_num"`, `"id"`,
		} {
			if strings.Contains(body, needle) {
				t.Fatalf("fact_distributions response leaks %s: %s", needle, body)
			}
		}
	})
}

// TestPublicAnalysisDomainDetailSurfacesNameserverStatus pins the .ck
// "circa" and "downstage" shapes on the domain detail response. Each
// nameserver is rendered with an explicit status so the UI can draw a
// red badge for unreachable / unresolved NSes instead of silently
// dropping them.
func TestPublicAnalysisDomainDetailSurfacesNameserverStatus(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)

		// ok.example + 192.0.2.10 → reachable (has samples).
		f.seedEndpoint("run-ck", "ck.example", "ok.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")

		// dead.example + 192.0.2.99 → unreachable (address present, zero samples).
		dead, err := f.store.UpsertAnalysisAddress("192.0.2.99", "ipv4", ts)
		if err != nil {
			t.Fatalf("upsert dead address: %v", err)
		}
		deadNS, err := f.store.UpsertAnalysisNameserver("dead.example", ts)
		if err != nil {
			t.Fatalf("upsert dead ns: %v", err)
		}
		domain, _ := f.store.GetDomainByName("ck.example")
		// ghost.example → unresolved (synthetic delegation endpoint with
		// AddressID=0 - the projector's "no address for this NS" marker).
		ghostNS, err := f.store.UpsertAnalysisNameserver("ghost.example", ts)
		if err != nil {
			t.Fatalf("upsert ghost ns: %v", err)
		}

		existing := f.store.ListAnalysisRunNSEndpoints(f.cohort.ID, "run-ck")
		existing = append(existing,
			AnalysisRunNameserverEndpoint{
				CohortID: f.cohort.ID, RunID: "run-ck", DomainID: domain.ID,
				NameserverID: deadNS.ID, AddressID: dead.ID,
				Role: "authoritative", Source: "timings", Family: "ipv4", QueryCount: 0,
			},
			AnalysisRunNameserverEndpoint{
				CohortID: f.cohort.ID, RunID: "run-ck", DomainID: domain.ID,
				NameserverID: ghostNS.ID, AddressID: 0,
				Role: "authoritative", Source: "delegation",
			},
		)
		if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, "run-ck", existing); err != nil {
			t.Fatalf("replace endpoints: %v", err)
		}
		f.refreshSnapshotViews(f.batchID)

		resp := getPublic(t, f.srv, f.publicURL("domains/ck.example"))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisDomainDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		byName := map[string]PublicAnalysisDomainNameserver{}
		for _, ns := range got.Nameservers {
			byName[ns.Nameserver] = ns
		}
		ok := byName["ok.example"]
		if len(ok.Addresses) != 1 {
			t.Fatalf("expected one ok address, got %+v", ok)
		}
		if ok.Addresses[0].Status != "" {
			t.Fatalf("expected reachable address to have no status, got %q", ok.Addresses[0].Status)
		}
		if ok.Status != "" {
			t.Fatalf("expected reachable NS to have no status, got %q", ok.Status)
		}
		dead2 := byName["dead.example"]
		if len(dead2.Addresses) != 1 {
			t.Fatalf("expected one address for unreachable NS, got %+v", dead2)
		}
		if dead2.Addresses[0].Status != "unreachable" {
			t.Fatalf("expected unreachable address status, got %q", dead2.Addresses[0].Status)
		}
		ghost := byName["ghost.example"]
		if len(ghost.Addresses) != 0 {
			t.Fatalf("expected no addresses for unresolved NS, got %+v", ghost)
		}
		if ghost.Status != "unresolved" {
			t.Fatalf("expected unresolved NS status, got %q", ghost.Status)
		}
	})
}

func TestPublicAnalysisCohortDetailNotFound(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/unknown")
		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", resp.Code, resp.Body)
		}
	})
}

func TestPublicAnalysisDomainDetail(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisDomainDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Domain != "alpha.example" {
			t.Fatalf("expected domain=alpha.example, got %q", got.Domain)
		}
		if len(got.Nameservers) != 1 || got.Nameservers[0].Nameserver != "ns1.shared.example" {
			t.Fatalf("expected one nameserver row, got %+v", got.Nameservers)
		}
		if got.Nameservers[0].IPv4Count != 1 || got.Nameservers[0].IPv6Count != 1 {
			t.Fatalf("expected dual-stack nameserver, got %+v", got.Nameservers)
		}
		if len(got.Addresses) != 2 {
			t.Fatalf("expected 2 addresses, got %+v", got.Addresses)
		}
		tagsByName := map[string]PublicAnalysisDomainTag{}
		for _, t := range got.Tags {
			tagsByName[t.Tag] = t
		}
		ds07, ok := tagsByName["DS07_NOT_SIGNED"]
		if !ok {
			t.Fatalf("expected DS07_NOT_SIGNED tag, got %+v", got.Tags)
		}
		if ds07.Level != "ERROR" {
			t.Fatalf("expected DS07_NOT_SIGNED tag at ERROR level, got %+v", ds07)
		}
		if ds07.Module != "DNSSEC" || ds07.Testcase != "dnssec07" {
			t.Fatalf("expected DS07_NOT_SIGNED under DNSSEC/dnssec07, got %+v", ds07)
		}
	})
}

func TestPublicAnalysisDomainDetailNotInCohort(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, f.publicURL("domains/missing.example"))
		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", resp.Code, resp.Body)
		}
	})
}

func TestPublicAnalysisNameserverDetail(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, f.publicURL("nameservers/ns1.shared.example"))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisNameserverDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.DomainCount != 2 {
			t.Fatalf("expected 2 domains served, got %d", got.DomainCount)
		}
		if got.IPv4Count != 1 || got.IPv6Count != 1 {
			t.Fatalf("expected 1 IPv4 and 1 IPv6 address, got %+v", got)
		}
		if len(got.ASNs) != 1 || got.ASNs[0] != 64500 {
			t.Fatalf("expected ASN 64500, got %+v", got.ASNs)
		}
	})
}

func TestPublicAnalysisNameserverDetailNotFound(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, f.publicURL("nameservers/missing.example"))
		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})
}

func TestPublicAnalysisEndpointDetail(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, f.publicURL("endpoints/192.0.2.10"))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisEndpointDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.DomainCount != 2 {
			t.Fatalf("expected endpoint shared across 2 domains, got %d", got.DomainCount)
		}
		if got.ASN == nil || *got.ASN != 64500 {
			t.Fatalf("expected ASN 64500, got %+v", got.ASN)
		}
	})
}

func TestPublicAnalysisASNDetail(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, f.publicURL("asns/64500"))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisASNDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.DomainCount != 2 {
			t.Fatalf("expected 2 domains on AS64500, got %d", got.DomainCount)
		}
		if got.NameserverCount != 1 {
			t.Fatalf("expected 1 nameserver on AS64500, got %d", got.NameserverCount)
		}
		if got.PrefixCount != 2 {
			t.Fatalf("expected 2 prefixes on AS64500 (v4+v6), got %d", got.PrefixCount)
		}
	})
}

func TestPublicAnalysisASNDetailNotFound(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, f.publicURL("asns/99999"))
		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})
}

func TestPublicAnalysisPrefixDetail(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, f.publicURL("prefix?prefix=192.0.2.0/24"))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisPrefixDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Prefix != "192.0.2.0/24" || got.Family != "ipv4" {
			t.Fatalf("unexpected prefix detail: %+v", got)
		}
		if got.DomainCount != 2 {
			t.Fatalf("expected 2 domains on prefix, got %d", got.DomainCount)
		}
	})
}

// TestPublicAnalysisPrefixDetailHidesNonAuthoritativeAddresses pins the fix
// for the "404 on endpoint detail from prefix page" bug. An address whose
// only trace is in the address-fact table (no authoritative endpoint)
// would previously show up as a clickable chip on /prefix/<...> and then
// 404 when the user clicked through to /endpoints/<addr>. The
// cache-level filter restricts addressASNs to the authoritative-address
// set, so the prefix detail and the endpoint detail agree on what's in
// the cohort.
func TestPublicAnalysisPrefixDetailHidesNonAuthoritativeAddresses(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

		// alpha.example: authoritative endpoint 192.0.2.10 in prefix 192.0.2.0/24.
		f.seedGraduatedRun("alpha.example", ts, nil)
		f.seedEndpoint("run-alpha.example-"+ts.Format("20060102150405"),
			"alpha.example", "ns.alpha.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")

		// Inject a second address-fact row for the SAME prefix but a
		// different address (192.0.2.99) with NO endpoint. Mirrors what the
		// engine emits when it notes a parent-side address inside a CN04 or
		// DNSSEC entry without recording an authoritative (ns, addr) pair.
		alpha, _ := f.store.GetDomainByName("alpha.example")
		ghost, err := f.store.UpsertAnalysisAddress("192.0.2.99", "ipv4", ts)
		if err != nil {
			t.Fatalf("upsert ghost address: %v", err)
		}
		prefix, err := f.store.UpsertAnalysisPrefix("192.0.2.0/24", "ipv4", ts)
		if err != nil {
			t.Fatalf("upsert prefix: %v", err)
		}
		runID := "run-alpha.example-" + ts.Format("20060102150405")
		existing := f.store.ListAnalysisRunAddressASNs(f.cohort.ID, runID)
		pfxID := prefix.ID
		asn := int64(64500)
		ghostFact := AnalysisRunAddressASN{
			CohortID: f.cohort.ID, RunID: runID, DomainID: alpha.ID,
			AddressID: ghost.ID, PrefixID: &pfxID, ASN: &asn,
		}
		if err := f.store.ReplaceAnalysisRunAddressASNs(f.cohort.ID, runID, append(existing, ghostFact)); err != nil {
			t.Fatalf("seed ghost address fact: %v", err)
		}

		resp := getPublic(t, f.srv, f.publicURL("prefix?prefix=192.0.2.0/24"))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisPrefixDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got.Addresses) != 1 || got.Addresses[0] != "192.0.2.10" {
			t.Fatalf("prefix detail should list only authoritative address, got %+v", got.Addresses)
		}
		if got.AddressCount != 1 {
			t.Fatalf("address_count should reflect filtered set, got %d", got.AddressCount)
		}

		// And the mirror check: endpoint detail for the ghost address still
		// 404s (nothing to resolve), but that's fine because the UI no longer
		// generates a link to it.
		resp = getPublic(t, f.srv, f.publicURL("endpoints/192.0.2.99"))
		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for non-authoritative address, got %d: %s", resp.Code, resp.Body)
		}
	})
}

func TestPublicAnalysisPrefixDetailMissingParam(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, f.publicURL("prefix"))
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.Code)
		}
	})
}

func TestPublicAnalysisTagDetail(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, f.publicURL("tags/DS07_NOT_SIGNED"))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisTagDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.DomainCount != 2 {
			t.Fatalf("expected DS07 to cover 2 domains, got %d", got.DomainCount)
		}
		if got.Level != "ERROR" {
			t.Fatalf("expected level ERROR, got %q", got.Level)
		}
	})
}

func TestPublicAnalysisTagDetailNotFound(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, f.publicURL("tags/UNKNOWN_TAG"))
		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})
}

func TestPublicAnalysisTestcaseDetail(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		resp := getPublic(t, f.srv, f.publicURL("testcase?module=DNSSEC&testcase=dnssec07"))
		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisTestcaseDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.DomainCount != 2 {
			t.Fatalf("expected 2 domains on dnssec07, got %d", got.DomainCount)
		}
		if len(got.Tags) == 0 {
			t.Fatalf("expected at least one tag on dnssec07, got %+v", got.Tags)
		}
	})
}

func TestPublicAnalysisDetailsRedactInternalIDs(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedDetailFixture(t, f)
		paths := []string{
			"/pub/api/v1/analysis/cohorts/tld",
			"/pub/api/v1/analysis/domains/alpha.example",
			"/pub/api/v1/analysis/nameservers/ns1.shared.example",
			"/pub/api/v1/analysis/endpoints/192.0.2.10",
			"/pub/api/v1/analysis/asns/64500",
			"/pub/api/v1/analysis/prefix?prefix=192.0.2.0/24",
			"/pub/api/v1/analysis/tags/DS07_NOT_SIGNED",
			"/pub/api/v1/analysis/testcase?module=DNSSEC&testcase=dnssec07",
		}
		for _, p := range paths {
			resp := getPublic(t, f.srv, p)
			raw := resp.Body.String()
			for _, needle := range []string{
				`"id"`, `"cohort_id"`, `"run_id"`, `"domain_id"`,
				`"nameserver_id"`, `"address_id"`, `"prefix_id"`,
				`"public_enabled"`, `"analysis_enabled"`,
			} {
				if strings.Contains(raw, needle) {
					t.Fatalf("%s should not expose %s: %s", p, needle, raw)
				}
			}
		}
	})
}

// TestPublicAnalysisCohortAndDetailsScopedToSnapshot verifies that the
// cohort and domain detail handlers scope their counts and entries to
// the resolved snapshot: old-batch facts are not exposed under the
// auto-latest default, and old-batch-only entities 404 without the
// ?snapshot= override.
func TestPublicAnalysisCohortAndDetailsScopedToSnapshot(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		t1 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		t2 := t1.Add(time.Hour)

		older := f.seedAlternateSnapshot("batch-old", "2026-04-17-old", t1.Add(-time.Hour))
		f.seedGraduatedRunInBatch(older.BatchID, "alpha.example", t1, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "OLD_TAG", Level: "ERROR"},
		})
		f.seedEndpointInBatch(older.BatchID, "run-alpha.example-"+t1.Format("20060102150405"),
			"alpha.example", "ns-old.example", "192.0.2.10", "ipv4", t1, 64500, "192.0.2.0/24")

		f.seedGraduatedRun("alpha.example", t2, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "NEW_TAG", Level: "NOTICE"},
		})
		f.seedEndpoint("run-alpha.example-"+t2.Format("20060102150405"),
			"alpha.example", "ns-new.example", "198.51.100.20", "ipv4", t2, 64501, "198.51.100.0/24")

		cohortResp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld")
		var cohort PublicAnalysisCohortDetail
		if err := json.NewDecoder(cohortResp.Body).Decode(&cohort); err != nil {
			t.Fatalf("decode cohort detail: %v", err)
		}
		if cohort.NameserverCount != 1 || cohort.EndpointCount != 1 || cohort.ASNCount != 1 || cohort.PrefixCount != 1 {
			t.Fatalf("expected auto-latest snapshot counts, got %+v", cohort)
		}
		if cohort.Snapshot == nil || cohort.Snapshot.Slug != "2026-04-20-fixture" {
			t.Fatalf("expected cohort detail to carry fixture snapshot, got %+v", cohort.Snapshot)
		}

		domainResp := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		var detail PublicAnalysisDomainDetail
		if err := json.NewDecoder(domainResp.Body).Decode(&detail); err != nil {
			t.Fatalf("decode domain detail: %v", err)
		}
		if len(detail.Nameservers) != 1 || detail.Nameservers[0].Nameserver != "ns-new.example" {
			t.Fatalf("expected only new snapshot's nameserver, got %+v", detail.Nameservers)
		}
		if len(detail.Addresses) != 1 || detail.Addresses[0].Address != "198.51.100.20" {
			t.Fatalf("expected only new snapshot's address, got %+v", detail.Addresses)
		}
		for _, t2 := range detail.Tags {
			if t2.Tag == "OLD_TAG" {
				t.Fatalf("expected domain detail to exclude older-snapshot tags, got %+v", detail.Tags)
			}
		}

		oldNS := getPublic(t, f.srv, f.publicURL("nameservers/ns-old.example"))
		if oldNS.Code != http.StatusNotFound {
			t.Fatalf("expected older nameserver to be hidden under auto-latest, got %d: %s", oldNS.Code, oldNS.Body)
		}

		oldASN := getPublic(t, f.srv, f.publicURL("asns/64500"))
		if oldASN.Code != http.StatusNotFound {
			t.Fatalf("expected older ASN to be hidden under auto-latest, got %d: %s", oldASN.Code, oldASN.Body)
		}

		oldPrefix := getPublic(t, f.srv, f.publicURL("prefix?prefix=192.0.2.0/24"))
		if oldPrefix.Code != http.StatusNotFound {
			t.Fatalf("expected older prefix to be hidden under auto-latest, got %d: %s", oldPrefix.Code, oldPrefix.Body)
		}

		// Pinning the older slug in the path makes that nameserver visible again.
		oldNSExplicit := getPublic(t, f.srv, f.publicURLForSnapshot("2026-04-17-old", "nameservers/ns-old.example"))
		if oldNSExplicit.Code != http.StatusOK {
			t.Fatalf("expected older nameserver visible with explicit slug, got %d: %s", oldNSExplicit.Code, oldNSExplicit.Body)
		}
	})
}

func TestPublicAnalysisEndpointDetailRequiresNameserverWhenAddressIsShared(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

		f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{{Module: "BASIC", Testcase: "basic01", Tag: "A", Level: "NOTICE"}})
		f.seedEndpoint("run-alpha.example-"+ts.Format("20060102150405"),
			"alpha.example", "ns1.shared.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
		f.seedGraduatedRun("beta.example", ts, []engine.LogEntry{{Module: "BASIC", Testcase: "basic01", Tag: "B", Level: "NOTICE"}})
		f.seedEndpoint("run-beta.example-"+ts.Format("20060102150405"),
			"beta.example", "ns2.shared.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")

		ambiguous := getPublic(t, f.srv, f.publicURL("endpoints/192.0.2.10"))
		if ambiguous.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for ambiguous endpoint, got %d: %s", ambiguous.Code, ambiguous.Body)
		}

		selected := getPublic(t, f.srv, f.publicURL("endpoints/192.0.2.10?nameserver=ns2.shared.example"))
		if selected.Code != http.StatusOK {
			t.Fatalf("expected 200 for disambiguated endpoint, got %d: %s", selected.Code, selected.Body)
		}
		var detail PublicAnalysisEndpointDetail
		if err := json.NewDecoder(selected.Body).Decode(&detail); err != nil {
			t.Fatalf("decode endpoint detail: %v", err)
		}
		if detail.Nameserver != "ns2.shared.example" || detail.DomainCount != 1 || len(detail.Domains) != 1 || detail.Domains[0] != "beta.example" {
			t.Fatalf("unexpected endpoint detail: %+v", detail)
		}
	})
}

func TestPublicAnalysisDetailHandlersLoadAllEntriesForRun(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		entries := make([]engine.LogEntry, 0, 10050)
		for range 10049 {
			entries = append(entries, engine.LogEntry{
				Module: "DNSSEC", Testcase: "bulk01", Tag: "BULK_TAG", Level: "NOTICE",
			})
		}
		entries = append(entries, engine.LogEntry{
			Module: "DNSSEC", Testcase: "bulk01", Tag: "LATE_TAG", Level: "ERROR",
		})
		f.seedGraduatedRun("alpha.example", ts, entries)

		domainResp := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		if domainResp.Code != http.StatusOK {
			t.Fatalf("expected 200 domain detail, got %d: %s", domainResp.Code, domainResp.Body)
		}
		var domainDetail PublicAnalysisDomainDetail
		if err := json.NewDecoder(domainResp.Body).Decode(&domainDetail); err != nil {
			t.Fatalf("decode domain detail: %v", err)
		}
		foundLateTag := false
		for _, t2 := range domainDetail.Tags {
			if t2.Tag == "LATE_TAG" {
				foundLateTag = true
				break
			}
		}
		if !foundLateTag {
			t.Fatalf("expected domain detail to include LATE_TAG after 10k entries, got %d tags", len(domainDetail.Tags))
		}

		tagResp := getPublic(t, f.srv, f.publicURL("tags/LATE_TAG"))
		if tagResp.Code != http.StatusOK {
			t.Fatalf("expected 200 tag detail, got %d: %s", tagResp.Code, tagResp.Body)
		}
		var tagDetail PublicAnalysisTagDetail
		if err := json.NewDecoder(tagResp.Body).Decode(&tagDetail); err != nil {
			t.Fatalf("decode tag detail: %v", err)
		}
		if tagDetail.OccurrenceCount != 1 {
			t.Fatalf("expected one LATE_TAG occurrence, got %+v", tagDetail)
		}

		testcaseResp := getPublic(t, f.srv, f.publicURL("testcase?module=DNSSEC&testcase=bulk01"))
		if testcaseResp.Code != http.StatusOK {
			t.Fatalf("expected 200 testcase detail, got %d: %s", testcaseResp.Code, testcaseResp.Body)
		}
		var testcaseDetail PublicAnalysisTestcaseDetail
		if err := json.NewDecoder(testcaseResp.Body).Decode(&testcaseDetail); err != nil {
			t.Fatalf("decode testcase detail: %v", err)
		}
		if testcaseDetail.EntryCount != 10050 {
			t.Fatalf("expected 10050 testcase entries, got %+v", testcaseDetail)
		}
	})
}
