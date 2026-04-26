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
	if cc := resp.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, want immutable on path-segmented URL", cc)
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
	if cc := resp.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, want immutable", cc)
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

// TestLegacyAutoLatestRedirectsToSnapshotPath proves the headline
// behavior change: a legacy URL without ?snapshot= 307s to the
// content-addressable path-segmented form.
func TestLegacyAutoLatestRedirectsToSnapshotPath(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
		"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	cases := []struct {
		name     string
		path     string
		wantPath string
	}{
		{"overview", "/pub/api/v1/analysis/overview", "/cohorts/tld/snapshots/" + f.snapshot.Slug + "/overview"},
		{"nameservers list", "/pub/api/v1/analysis/nameservers", "/cohorts/tld/snapshots/" + f.snapshot.Slug + "/nameservers"},
		{"nameserver detail", "/pub/api/v1/analysis/nameservers/ns1.example", "/cohorts/tld/snapshots/" + f.snapshot.Slug + "/nameservers/ns1.example"},
		{"prefix detail", "/pub/api/v1/analysis/prefix?prefix=192.0.2.0%2F24", "/cohorts/tld/snapshots/" + f.snapshot.Slug + "/prefix"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := getPublicNoRedirect(t, f.srv, tc.path)
			if resp.Code != http.StatusTemporaryRedirect {
				t.Fatalf("status = %d, want 307", resp.Code)
			}
			loc := resp.Header().Get("Location")
			if !strings.Contains(loc, tc.wantPath) {
				t.Errorf("Location = %q, want path-segmented %q", loc, tc.wantPath)
			}
			if !strings.HasPrefix(loc, "/pub/api/v1/analysis/cohorts/") {
				t.Errorf("Location = %q, want path-segmented prefix", loc)
			}
		})
	}
}

// TestLegacyExplicitSnapshotServesInline confirms the transition
// behavior: a legacy URL with ?snapshot=<slug> still returns content
// directly (no redirect).
func TestLegacyExplicitSnapshotServesInline(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
		"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	url := "/pub/api/v1/analysis/nameservers?snapshot=" + f.snapshot.Slug
	resp := getPublicNoRedirect(t, f.srv, url)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	if cc := resp.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, want immutable", cc)
	}
}

// TestLegacyAutoLatestNoCaptureFallsThrough verifies that when no
// captured public snapshot exists, the legacy URL is served inline
// (no redirect target to point at) rather than 307ing into a void.
func TestLegacyAutoLatestNoCaptureFallsThrough(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	pending, err := f.store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
		ID:       f.snapshot.ID,
		CohortID: f.cohort.ID,
		BatchID:  f.batchID,
		Slug:     f.snapshot.Slug,
		Status:   AnalysisSnapshotStatusPending,
		IsPublic: false,
	})
	if err != nil {
		t.Fatalf("downgrade snapshot: %v", err)
	}
	f.snapshot = pending

	resp := getPublicNoRedirect(t, f.srv, "/pub/api/v1/analysis/overview")
	if resp.Code == http.StatusTemporaryRedirect {
		t.Fatalf("auto-latest with no captured snapshot must not redirect, got 307")
	}
}

// TestLegacyAutoLatestRedirectPreservesQueryParams verifies that
// non-pin query parameters (search, limit, etc.) survive the redirect.
func TestLegacyAutoLatestRedirectPreservesQueryParams(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, nil)
	f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
		"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

	resp := getPublicNoRedirect(t, f.srv, "/pub/api/v1/analysis/domains?search=alpha&limit=25")
	if resp.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want 307", resp.Code)
	}
	loc := resp.Header().Get("Location")
	if !strings.Contains(loc, "search=alpha") {
		t.Errorf("Location lost ?search=: %q", loc)
	}
	if !strings.Contains(loc, "limit=25") {
		t.Errorf("Location lost ?limit=: %q", loc)
	}
	if strings.Contains(loc, "dataset_tag=") {
		t.Errorf("Location should drop dataset_tag (now in path): %q", loc)
	}
	if strings.Contains(loc, "snapshot=") {
		t.Errorf("Location should drop snapshot= (now in path): %q", loc)
	}
}
