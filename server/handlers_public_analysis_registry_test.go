package server

import (
	"net/http"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/server/extdata"
)

// fakeRegistry stands in for the external-data provider so handler tests
// never touch the network.
type fakeRegistry struct {
	enabled   bool
	url       string
	urlState  extdata.State
	summary   *extdata.RDAPDomainSummary
	fetchedAt time.Time
	state     extdata.State
	// asked records the domains the handler looked up.
	asked []string
}

func (f *fakeRegistry) Enabled() bool { return f.enabled }

func (f *fakeRegistry) RDAPDomainURL(domain string) (string, extdata.State) {
	f.asked = append(f.asked, domain)
	return f.url, f.urlState
}

func (f *fakeRegistry) RDAPDomain(string) (*extdata.RDAPDomainSummary, time.Time, extdata.State) {
	return f.summary, f.fetchedAt, f.state
}

// seedRegistryFixture seeds one graduated domain and installs reg on the
// fixture's server.
func seedRegistryFixture(t *testing.T, f *analysisFixture, reg registryLookup) {
	t.Helper()
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
	})
	f.srv.registry = reg
}

func TestDomainDetailOmitsRegistryWhenProviderDisabled(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedRegistryFixture(t, f, &fakeRegistry{enabled: false})

		resp := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		raw := mustJSON[map[string]any](t, resp, http.StatusOK)
		if _, ok := raw["registry"]; ok {
			t.Error("registry present with the provider disabled")
		}
		if _, ok := raw["rdap_url"]; ok {
			t.Error("rdap_url present with the provider disabled")
		}
	})
}

func TestDomainDetailRegistryPendingOnCacheMiss(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedRegistryFixture(t, f, &fakeRegistry{
			enabled:  true,
			urlState: extdata.StatePending,
			state:    extdata.StatePending,
		})

		resp := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		got := mustJSON[PublicAnalysisDomainDetail](t, resp, http.StatusOK)
		if got.Registry == nil || got.Registry.State != "pending" {
			t.Fatalf("registry = %+v, want state pending", got.Registry)
		}
		if got.Registry.FetchedAt != nil || got.Registry.Handle != "" {
			t.Errorf("registry = %+v, want no fields on a miss", got.Registry)
		}
		if got.RDAPURL != "" {
			t.Errorf("rdap_url = %q, want empty until the bootstrap resolves", got.RDAPURL)
		}
	})
}

func TestDomainDetailRegistryUnavailable(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		seedRegistryFixture(t, f, &fakeRegistry{
			enabled:  true,
			urlState: extdata.StateUnavailable,
			state:    extdata.StateUnavailable,
		})

		resp := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		got := mustJSON[PublicAnalysisDomainDetail](t, resp, http.StatusOK)
		if got.Registry == nil || got.Registry.State != "unavailable" {
			t.Fatalf("registry = %+v, want state unavailable", got.Registry)
		}
	})
}

func TestDomainDetailRegistryRendersSummary(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		registered := time.Date(2001, 3, 4, 0, 0, 0, 0, time.UTC)
		expires := time.Date(2027, 3, 4, 0, 0, 0, 0, time.UTC)
		fetched := time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)
		signed := true
		reg := &fakeRegistry{
			enabled:  true,
			url:      "https://rdap.example.org/domain/alpha.example",
			urlState: extdata.StateFresh,
			state:    extdata.StateFresh,
			summary: &extdata.RDAPDomainSummary{
				Handle:           "ALPHA-1",
				Status:           []string{"active"},
				Registrar:        "Example Registrar",
				RegistryOrg:      "Example Registry",
				RegisteredAt:     &registered,
				ExpiresAt:        &expires,
				Nameservers:      []string{"ns1.example"},
				DelegationSigned: &signed,
				SourceURL:        "https://rdap.example.org/domain/alpha.example",
			},
			fetchedAt: fetched,
		}
		seedRegistryFixture(t, f, reg)

		resp := getPublic(t, f.srv, f.publicURL("domains/alpha.example"))
		got := mustJSON[PublicAnalysisDomainDetail](t, resp, http.StatusOK)
		if got.RDAPURL != reg.url {
			t.Errorf("rdap_url = %q, want %q", got.RDAPURL, reg.url)
		}
		r := got.Registry
		if r == nil || r.State != "fresh" {
			t.Fatalf("registry = %+v, want state fresh", r)
		}
		if r.Handle != "ALPHA-1" || r.Registrar != "Example Registrar" || r.RegistryOrg != "Example Registry" {
			t.Errorf("registry identity = %+v", r)
		}
		if r.RegisteredAt == nil || !r.RegisteredAt.Equal(registered) {
			t.Errorf("registered_at = %v, want %v", r.RegisteredAt, registered)
		}
		if r.ExpiresAt == nil || !r.ExpiresAt.Equal(expires) {
			t.Errorf("expires_at = %v, want %v", r.ExpiresAt, expires)
		}
		if r.ChangedAt != nil {
			t.Errorf("changed_at = %v, want nil when the record has no such event", r.ChangedAt)
		}
		if r.FetchedAt == nil || !r.FetchedAt.Equal(fetched) {
			t.Errorf("fetched_at = %v, want %v", r.FetchedAt, fetched)
		}
		if r.DelegationSigned == nil || !*r.DelegationSigned {
			t.Errorf("delegation_signed = %v, want true", r.DelegationSigned)
		}
		if len(r.Nameservers) != 1 || r.Nameservers[0] != "ns1.example" {
			t.Errorf("nameservers = %v", r.Nameservers)
		}
		// The captured domain name is what gets looked up, not the URL path.
		if len(reg.asked) != 1 || reg.asked[0] != "alpha.example" {
			t.Errorf("looked up %v, want [alpha.example]", reg.asked)
		}
	})
}

func TestAnalysisStatusReportsExternalData(t *testing.T) {
	srv := newTestServer(t, withConfig(func(c *Config) {
		c.ExternalData.Enabled = true
	}))
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/analysis/status", nil)
	got := mustJSON[AnalysisStatusResponse](t, resp, http.StatusOK)
	if got.ExternalData == nil || !got.ExternalData.Enabled {
		t.Fatalf("external_data = %+v, want an enabled block", got.ExternalData)
	}
	if len(got.ExternalData.Datasets) != 2 {
		t.Fatalf("datasets = %+v, want both reference datasets listed", got.ExternalData.Datasets)
	}
	// Nothing has been fetched in a test server, so both are still pending.
	for _, ds := range got.ExternalData.Datasets {
		if ds.State != string(extdata.StatePending) {
			t.Errorf("dataset %s state = %q, want pending", ds.Name, ds.State)
		}
	}
}

func TestAnalysisStatusOmitsExternalDataWhenDisabled(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/analysis/status", nil)
	raw := mustJSON[map[string]any](t, resp, http.StatusOK)
	if _, ok := raw["external_data"]; ok {
		t.Error("external_data present with the provider disabled")
	}
	if srv.ExternalData() != nil {
		t.Error("a disabled server must not build a provider")
	}
}
