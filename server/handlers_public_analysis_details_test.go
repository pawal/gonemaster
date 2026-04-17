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
