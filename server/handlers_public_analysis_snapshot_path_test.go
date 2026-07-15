package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// TestSnapshotPathOverviewServesContent proves the path-segmented
// overview URL returns the captured payload directly.
func TestSnapshotPathOverviewServesContent(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})
	f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
		"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	url := "/pub/api/v1/analysis/cohorts/tld/snapshots/" + f.snapshot.Slug + "/overview"
	resp := getPublicNoRedirect(t, f.srv, url)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisOverviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Overview == nil {
		t.Fatal("expected Overview payload")
	}
	if got.DatasetTag != "tld" {
		t.Errorf("DatasetTag = %q", got.DatasetTag)
	}
	// Snapshots can be rebuilt under the same slug, so the response must not be
	// immutable; it revalidates via the ETag, which changes on rebuild.
	if cc := resp.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, must not be immutable on path-segmented URL", cc)
	}
	if etag := resp.Header().Get("ETag"); etag == "" {
		t.Error("expected ETag on captured snapshot URL")
	}
}

// TestSnapshotPathDomainDetailServesContent confirms a per-domain read
// works at the path-segmented form.
func TestSnapshotPathDomainDetailServesContent(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
		"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	url := "/pub/api/v1/analysis/cohorts/tld/snapshots/" + f.snapshot.Slug + "/domains/alpha.example"
	resp := getPublicNoRedirect(t, f.srv, url)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisDomainDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Domain != "alpha.example" {
		t.Errorf("Domain = %q", got.Domain)
	}
	// Rebuildable snapshot: must revalidate via ETag, not be immutable.
	if cc := resp.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, must not be immutable", cc)
	}
	if resp.Header().Get("ETag") == "" {
		t.Error("expected ETag on captured snapshot URL")
	}
}

// TestSnapshotPathPrefixDetailPreservesQuery verifies the ?prefix=
// query parameter survives the path-segmented routing.
func TestSnapshotPathPrefixDetailPreservesQuery(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
		"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	url := "/pub/api/v1/analysis/cohorts/tld/snapshots/" + f.snapshot.Slug +
		"/prefix?prefix=192.0.2.0%2F24"
	resp := getPublicNoRedirect(t, f.srv, url)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisPrefixDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Prefix != "192.0.2.0/24" {
		t.Errorf("Prefix = %q", got.Prefix)
	}
}

// TestSnapshotPathUnknownSlug404s checks that hitting an unknown
// snapshot slug returns 404.
func TestSnapshotPathUnknownSlug404s(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)

	url := "/pub/api/v1/analysis/cohorts/tld/snapshots/does-not-exist/overview"
	resp := getPublicNoRedirect(t, f.srv, url)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
}

// TestSnapshotPathUnknownCohort404s checks that hitting an unknown
// cohort returns 404.
func TestSnapshotPathUnknownCohort404s(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)

	url := "/pub/api/v1/analysis/cohorts/nope/snapshots/" + f.snapshot.Slug + "/overview"
	resp := getPublicNoRedirect(t, f.srv, url)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
}

// TestLegacyQueryParamRouteIsGone asserts the old query-param routes
// no longer match - the path-segmented form is the only public-read
// URL shape now.
func TestLegacyQueryParamRouteIsGone(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
		"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	for _, path := range []string{
		"/pub/api/v1/analysis/overview",
		"/pub/api/v1/analysis/nameservers",
		"/pub/api/v1/analysis/nameservers/ns1.example",
		"/pub/api/v1/analysis/domains?dataset_tag=tld&snapshot=" + f.snapshot.Slug,
		"/pub/api/v1/analysis/prefix?prefix=192.0.2.0%2F24",
	} {
		resp := getPublicNoRedirect(t, f.srv, path)
		if resp.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404 (legacy route was retired)", path, resp.Code)
		}
	}
}
