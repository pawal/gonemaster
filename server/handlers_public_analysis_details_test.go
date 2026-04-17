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
func seedDetailFixture(t *testing.T) *analysisAPITestFixture {
	t.Helper()
	f := newAnalysisAPITestFixture(t)
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
	f := seedDetailFixture(t)
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
}

func TestPublicAnalysisCohortDetailNotFound(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/unknown")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.Code, resp.Body)
	}
}

func TestPublicAnalysisDomainDetail(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains/alpha.example")
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
	tags := map[string]PublicAnalysisDomainTag{}
	for _, tag := range got.Tags {
		tags[tag.Tag] = tag
	}
	if _, ok := tags["DS07_NOT_SIGNED"]; !ok {
		t.Fatalf("expected DS07_NOT_SIGNED tag, got %+v", got.Tags)
	}
	if got.Tags[0].Level != "ERROR" {
		t.Fatalf("expected ERROR-level tag first, got %+v", got.Tags)
	}
}

func TestPublicAnalysisDomainDetailNotInCohort(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains/missing.example")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.Code, resp.Body)
	}
}

func TestPublicAnalysisNameserverDetail(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/nameservers/ns1.shared.example")
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
}

func TestPublicAnalysisNameserverDetailNotFound(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/nameservers/missing.example")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestPublicAnalysisEndpointDetail(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/endpoints/192.0.2.10")
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
}

func TestPublicAnalysisASNDetail(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/asns/64500")
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
}

func TestPublicAnalysisASNDetailNotFound(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/asns/99999")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestPublicAnalysisPrefixDetail(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/prefix?prefix=192.0.2.0/24")
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
}

func TestPublicAnalysisPrefixDetailMissingParam(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/prefix")
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestPublicAnalysisTagDetail(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/tags/DS07_NOT_SIGNED")
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
}

func TestPublicAnalysisTagDetailNotFound(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/tags/UNKNOWN_TAG")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestPublicAnalysisTestcaseDetail(t *testing.T) {
	f := seedDetailFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/testcase?module=DNSSEC&testcase=dnssec07")
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
}

func TestPublicAnalysisDetailsRedactInternalIDs(t *testing.T) {
	f := seedDetailFixture(t)
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
}

func TestPublicAnalysisCohortAndDetailsUseLatestRunFacts(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	t1 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	f.seedGraduatedRun("alpha.example", t1, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "OLD_TAG", Level: "ERROR"},
	})
	f.seedEndpoint("run-alpha.example-"+t1.Format("20060102150405"),
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
		t.Fatalf("expected latest-only cohort counts, got %+v", cohort)
	}

	domainResp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains/alpha.example")
	var detail PublicAnalysisDomainDetail
	if err := json.NewDecoder(domainResp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode domain detail: %v", err)
	}
	if len(detail.Nameservers) != 1 || detail.Nameservers[0].Nameserver != "ns-new.example" {
		t.Fatalf("expected only latest nameserver, got %+v", detail.Nameservers)
	}
	if len(detail.Addresses) != 1 || detail.Addresses[0].Address != "198.51.100.20" {
		t.Fatalf("expected only latest address, got %+v", detail.Addresses)
	}
	for _, tag := range detail.Tags {
		if tag.Tag == "OLD_TAG" {
			t.Fatalf("expected domain detail to exclude old run tags, got %+v", detail.Tags)
		}
	}

	oldNS := getPublic(t, f.srv, "/pub/api/v1/analysis/nameservers/ns-old.example")
	if oldNS.Code != http.StatusNotFound {
		t.Fatalf("expected old nameserver to be absent, got %d: %s", oldNS.Code, oldNS.Body)
	}

	oldASN := getPublic(t, f.srv, "/pub/api/v1/analysis/asns/64500")
	if oldASN.Code != http.StatusNotFound {
		t.Fatalf("expected old ASN to be absent, got %d: %s", oldASN.Code, oldASN.Body)
	}

	oldPrefix := getPublic(t, f.srv, "/pub/api/v1/analysis/prefix?prefix=192.0.2.0/24")
	if oldPrefix.Code != http.StatusNotFound {
		t.Fatalf("expected old prefix to be absent, got %d: %s", oldPrefix.Code, oldPrefix.Body)
	}
}

func TestPublicAnalysisEndpointDetailRequiresNameserverWhenAddressIsShared(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{{Module: "BASIC", Testcase: "basic01", Tag: "A", Level: "NOTICE"}})
	f.seedEndpoint("run-alpha.example-"+ts.Format("20060102150405"),
		"alpha.example", "ns1.shared.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")
	f.seedGraduatedRun("beta.example", ts, []engine.LogEntry{{Module: "BASIC", Testcase: "basic01", Tag: "B", Level: "NOTICE"}})
	f.seedEndpoint("run-beta.example-"+ts.Format("20060102150405"),
		"beta.example", "ns2.shared.example", "192.0.2.10", "ipv4", ts, 64500, "192.0.2.0/24")

	ambiguous := getPublic(t, f.srv, "/pub/api/v1/analysis/endpoints/192.0.2.10")
	if ambiguous.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for ambiguous endpoint, got %d: %s", ambiguous.Code, ambiguous.Body)
	}

	selected := getPublic(t, f.srv, "/pub/api/v1/analysis/endpoints/192.0.2.10?nameserver=ns2.shared.example")
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
}

func TestPublicAnalysisDetailHandlersLoadAllEntriesForRun(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	entries := make([]engine.LogEntry, 0, 10050)
	for i := 0; i < 10049; i++ {
		entries = append(entries, engine.LogEntry{
			Module: "DNSSEC", Testcase: "bulk01", Tag: "BULK_TAG", Level: "NOTICE",
		})
	}
	entries = append(entries, engine.LogEntry{
		Module: "DNSSEC", Testcase: "bulk01", Tag: "LATE_TAG", Level: "ERROR",
	})
	f.seedGraduatedRun("alpha.example", ts, entries)

	domainResp := getPublic(t, f.srv, "/pub/api/v1/analysis/domains/alpha.example")
	if domainResp.Code != http.StatusOK {
		t.Fatalf("expected 200 domain detail, got %d: %s", domainResp.Code, domainResp.Body)
	}
	var domainDetail PublicAnalysisDomainDetail
	if err := json.NewDecoder(domainResp.Body).Decode(&domainDetail); err != nil {
		t.Fatalf("decode domain detail: %v", err)
	}
	foundLateTag := false
	for _, tag := range domainDetail.Tags {
		if tag.Tag == "LATE_TAG" {
			foundLateTag = true
			break
		}
	}
	if !foundLateTag {
		t.Fatalf("expected domain detail to include tag after 10k entries, got %+v", domainDetail.Tags)
	}

	tagResp := getPublic(t, f.srv, "/pub/api/v1/analysis/tags/LATE_TAG")
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

	testcaseResp := getPublic(t, f.srv, "/pub/api/v1/analysis/testcase?module=DNSSEC&testcase=bulk01")
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
}
