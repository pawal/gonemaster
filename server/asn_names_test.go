package server

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"codeberg.org/pawal/gonemaster/engine"
)

func TestParseASLabel(t *testing.T) {
	for _, tc := range []struct {
		label, handle, name, country string
	}{
		{"MIGR-AS - Migrationsverket, SE", "MIGR-AS", "Migrationsverket", "SE"},
		{"CLOUDFLARENET - Cloudflare, Inc., US", "CLOUDFLARENET", "Cloudflare, Inc.", "US"},
		{"TWELVE99 - Arelion Sweden AB, SE", "TWELVE99", "Arelion Sweden AB", "SE"},
		{"NETNOD-IX - Netnod AB, SE", "NETNOD-IX", "Netnod AB", "SE"},
		{"CLOUDFLARENET, US", "", "CLOUDFLARENET", "US"},
		{"Some Org - Unit, SE", "", "Some Org - Unit", "SE"},
		{"EXAMPLE-AS - Example Org", "EXAMPLE-AS", "Example Org", ""},
		{"EXAMPLE-AS - , SE", "EXAMPLE-AS", "EXAMPLE-AS", "SE"},
		{"Example, se", "", "Example, se", ""},
		{"", "", "", ""},
		{"  MIGR-AS - Migrations\x00verket\n, SE\t", "MIGR-AS", "Migrationsverket", "SE"},
	} {
		handle, name, country := parseASLabel(tc.label)
		if handle != tc.handle || name != tc.name || country != tc.country {
			t.Errorf("parseASLabel(%q) = %q, %q, %q; want %q, %q, %q",
				tc.label, handle, name, country, tc.handle, tc.name, tc.country)
		}
	}
}

func TestCleanASLabelCapsLength(t *testing.T) {
	got := cleanASLabel(strings.Repeat("Ä", 300))
	if n := len([]rune(got)); n != asnLabelMaxRunes {
		t.Fatalf("cleaned label has %d runes, want %d", n, asnLabelMaxRunes)
	}
}

func TestCollectASNs(t *testing.T) {
	entries := []JobResultEntry{
		{Args: map[string]any{"asn": 199973}},
		{Args: map[string]any{"asn": float64(13335)}},
		{Args: map[string]any{"asn": json.Number("1299")}},
		{Args: map[string]any{"asns": []any{float64(8674), float64(1299), "64500", float64(1.5)}}},
		{Args: map[string]any{"asns": []int{0, -1, 4294967296, 4294967295}}},
		{Args: map[string]any{"asn": int64(13335), "asns": []int64{199973}}},
		{Args: map[string]any{"asn": json.Number("1e3")}},
		{Args: nil},
	}
	got := collectASNs(entries)
	want := []int64{1299, 8674, 13335, 199973, 4294967295}
	if !slices.Equal(got, want) {
		t.Fatalf("collectASNs = %v, want %v", got, want)
	}
}

func TestCollectASNsCaps(t *testing.T) {
	list := make([]any, 0, 40)
	for i := 40; i >= 1; i-- {
		list = append(list, float64(i))
	}
	got := collectASNs([]JobResultEntry{{Args: map[string]any{"asns": list}}})
	if len(got) != asnNamesMax || got[0] != 1 || got[len(got)-1] != asnNamesMax {
		t.Fatalf("collectASNs = %v, want 1..%d", got, asnNamesMax)
	}
}

// fakeLabeler answers from a fixed map; ASNs in block wait for release.
type fakeLabeler struct {
	labels  map[int64]string
	block   map[int64]bool
	release chan struct{}
	mu      sync.Mutex
	calls   []int64
}

func (f *fakeLabeler) EnrichASNLabel(ctx context.Context, asn int64) (string, bool) {
	f.mu.Lock()
	f.calls = append(f.calls, asn)
	f.mu.Unlock()
	if f.block[asn] {
		<-f.release
	}
	label, ok := f.labels[asn]
	return label, ok
}

var holderLabels = map[int64]string{
	199973: "MIGR-AS - Migrationsverket, SE",
	13335:  "CLOUDFLARENET - Cloudflare, Inc., US",
	1299:   "TWELVE99 - Arelion Sweden AB, SE",
	8674:   "NETNOD-IX - Netnod AB, SE",
}

func asnEntries() []engine.LogEntry {
	return []engine.LogEntry{
		{Module: "Connectivity", Testcase: "Connectivity03", Tag: "IPV4_ONE_ASN", Level: "WARNING", Args: map[string]any{"asn": 199973}},
		{Module: "Connectivity", Testcase: "Connectivity03", Tag: "IPV6_DIFFERENT_ASN", Level: "INFO", Args: map[string]any{"asns": []int{13335, 1299, 64500}}},
		{Module: "Connectivity", Testcase: "Connectivity03", Tag: "ASN_INFOS_ANNOUNCE_BY", Level: "INFO", Args: map[string]any{"asns": []any{float64(8674)}}},
	}
}

func seedASNRun(t *testing.T, srv *Server, entries []engine.LogEntry) string {
	t.Helper()
	return seedGraduatedRun(t, srv.store, runSpec{Progress: 100, Origin: JobOriginPublic, Entries: entries}).PublicID
}

func TestPublicASNNamesReturnsSortedNames(t *testing.T) {
	srv := newTestServer(t)
	srv.SetASNLabeler(&fakeLabeler{labels: holderLabels})
	publicID := seedASNRun(t, srv, asnEntries())

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/asn-names", nil)

	got := mustJSON[asnNamesResponse](t, resp, http.StatusOK)
	if cc := resp.Header().Get("Cache-Control"); cc != "public, max-age=86400" {
		t.Errorf("Cache-Control = %q, want public, max-age=86400", cc)
	}
	if !got.Complete {
		t.Error("complete = false, want true")
	}
	want := []asnName{
		{ASN: 1299, Name: "Arelion Sweden AB", Handle: "TWELVE99", Country: "SE", Label: holderLabels[1299]},
		{ASN: 8674, Name: "Netnod AB", Handle: "NETNOD-IX", Country: "SE", Label: holderLabels[8674]},
		{ASN: 13335, Name: "Cloudflare, Inc.", Handle: "CLOUDFLARENET", Country: "US", Label: holderLabels[13335]},
		{ASN: 199973, Name: "Migrationsverket", Handle: "MIGR-AS", Country: "SE", Label: holderLabels[199973]},
	}
	if !slices.Equal(got.ASNs, want) {
		t.Fatalf("asns = %+v\nwant %+v", got.ASNs, want)
	}
}

func TestPublicASNNamesNoASArgs(t *testing.T) {
	srv := newTestServer(t)
	labeler := &fakeLabeler{labels: holderLabels}
	srv.SetASNLabeler(labeler)
	publicID := seedASNRun(t, srv, systemStartEntry())

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/asn-names", nil)

	wantStatus(t, resp, http.StatusOK)
	if body := strings.TrimSpace(resp.Body.String()); body != `{"complete":true,"asns":[]}` {
		t.Fatalf("body = %s", body)
	}
	if len(labeler.calls) != 0 {
		t.Fatalf("labeler called for %v, want no calls", labeler.calls)
	}
}

func TestPublicASNNamesNotFound(t *testing.T) {
	t.Run("flag off", func(t *testing.T) {
		srv := newTestServer(t, withConfig(func(c *Config) { c.ShowASNNamesPublic = false }))
		srv.SetASNLabeler(&fakeLabeler{labels: holderLabels})
		publicID := seedASNRun(t, srv, asnEntries())
		resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/asn-names", nil)
		wantErrorCode(t, resp, http.StatusNotFound, "not_found")
	})
	t.Run("no labeler", func(t *testing.T) {
		srv := newTestServer(t)
		publicID := seedASNRun(t, srv, asnEntries())
		resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/asn-names", nil)
		wantErrorCode(t, resp, http.StatusNotFound, "not_found")
	})
	t.Run("unknown id", func(t *testing.T) {
		srv := newTestServer(t)
		srv.SetASNLabeler(&fakeLabeler{labels: holderLabels})
		resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/nosuchid1/asn-names", nil)
		wantErrorCode(t, resp, http.StatusNotFound, "not_found")
	})
	t.Run("no result", func(t *testing.T) {
		srv := newTestServer(t)
		srv.SetASNLabeler(&fakeLabeler{labels: holderLabels})
		job, err := srv.store.Create(Job{ID: newID("job"), Domain: "example.com", Origin: JobOriginPublic})
		if err != nil {
			t.Fatalf("create job: %v", err)
		}
		resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+job.PublicID+"/asn-names", nil)
		wantErrorCode(t, resp, http.StatusNotFound, "not_found")
	})
}

func TestPublicASNNamesDeadlineAnswersPartial(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := newTestServer(t)
		labeler := &fakeLabeler{labels: holderLabels, block: map[int64]bool{13335: true}, release: make(chan struct{})}
		srv.SetASNLabeler(labeler)
		publicID := seedASNRun(t, srv, asnEntries())

		resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+publicID+"/asn-names", nil)
		close(labeler.release)

		got := mustJSON[asnNamesResponse](t, resp, http.StatusOK)
		if cc := resp.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", cc)
		}
		if got.Complete {
			t.Error("complete = true, want false")
		}
		var asns []int64
		for _, n := range got.ASNs {
			asns = append(asns, n.ASN)
		}
		if want := []int64{1299, 8674, 199973}; !slices.Equal(asns, want) {
			t.Fatalf("asns = %v, want %v", asns, want)
		}
	})
}

func TestPublicInfoASNNamesNeedsLabeler(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flag    bool
		labeler bool
		want    bool
	}{
		{"flag on, no labeler", true, false, false},
		{"flag on, labeler", true, true, true},
		{"flag off, labeler", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t, withConfig(func(c *Config) { c.ShowASNNamesPublic = tc.flag }))
			if tc.labeler {
				srv.SetASNLabeler(&fakeLabeler{})
			}
			resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/info", nil)
			info := mustJSON[publicInfoResponse](t, resp, http.StatusOK)
			if info.ShowASNNamesPublic != tc.want {
				t.Fatalf("show_asn_names_public = %v, want %v", info.ShowASNNamesPublic, tc.want)
			}
		})
	}
}

func TestSettingsASNNamesRoundTrip(t *testing.T) {
	srv := newTestServer(t)

	settings := mustJSON[map[string]settingEntry](t, doJSON(t, srv, http.MethodGet, "/api/v1/settings", nil), http.StatusOK)
	if v, ok := settings["show_asn_names_public"].Value.(bool); !ok || !v {
		t.Fatalf("show_asn_names_public = %#v, want true", settings["show_asn_names_public"].Value)
	}

	wantStatus(t, doJSON(t, srv, http.MethodPut, "/api/v1/settings", `{"show_asn_names_public": false}`), http.StatusOK)

	if v, ok := srv.store.GetSetting("show_asn_names_public"); !ok || v != "false" {
		t.Fatalf("stored show_asn_names_public = %q, %v; want \"false\"", v, ok)
	}
	if srv.cfg.ShowASNNamesPublic {
		t.Fatal("ShowASNNamesPublic = true after PUT false")
	}
	settings = mustJSON[map[string]settingEntry](t, doJSON(t, srv, http.MethodGet, "/api/v1/settings", nil), http.StatusOK)
	if v, ok := settings["show_asn_names_public"].Value.(bool); !ok || v {
		t.Fatalf("show_asn_names_public = %#v, want false", settings["show_asn_names_public"].Value)
	}
}
