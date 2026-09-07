package extdata

import (
	"strings"
	"testing"
	"time"
)

// seedBootstrap loads the fixture bootstrap straight into the cache so URL
// resolution can be tested without any HTTP.
func seedBootstrap(t *testing.T, p *Provider) {
	t.Helper()
	value, version, err := parseRDAPBootstrap(readFixture(t, "rdap-dns.json"))
	if err != nil {
		t.Fatalf("parse bootstrap fixture: %v", err)
	}
	p.store(datasetKey(datasetRDAPBootstrap), kindDataset, Item{
		Value:     value,
		FetchedAt: p.now(),
		SourceURL: p.cfg.Sources.RDAPBootstrap,
		Version:   version,
	}, "", "")
}

func enabledProvider() *Provider {
	cfg := DefaultConfig()
	cfg.Enabled = true
	return New(cfg)
}

func TestRDAPDomainURLTLDUsesIANA(t *testing.T) {
	p := enabledProvider()
	url, state := p.RDAPDomainURL("SE.")
	if state != StateFresh {
		t.Fatalf("state = %q, want fresh: a TLD needs no bootstrap", state)
	}
	if url != "https://rdap.iana.org/domain/se" {
		t.Fatalf("url = %q, want the IANA endpoint", url)
	}
}

func TestRDAPDomainURLFromBootstrap(t *testing.T) {
	p := enabledProvider()
	seedBootstrap(t, p)
	url, state := p.RDAPDomainURL("Example.SE")
	if state != StateFresh {
		t.Fatalf("state = %q, want fresh", state)
	}
	if url != "https://rdap.iis.se/domain/example.se" {
		t.Fatalf("url = %q, want the registry endpoint", url)
	}
}

func TestRDAPDomainURLPendingWithoutBootstrap(t *testing.T) {
	p := enabledProvider()
	url, state := p.RDAPDomainURL("example.se")
	if state != StatePending || url != "" {
		t.Fatalf("state/url = %q/%q, want pending and no URL", state, url)
	}
}

func TestRDAPDomainURLUnavailableForUnservedTLD(t *testing.T) {
	p := enabledProvider()
	seedBootstrap(t, p)
	for _, name := range []string{"example.invalidtld", "example.example", "example.internal"} {
		if url, state := p.RDAPDomainURL(name); state != StateUnavailable || url != "" {
			t.Errorf("%s: state/url = %q/%q, want unavailable and no URL", name, state, url)
		}
	}
}

func TestRDAPDomainURLDisabledProvider(t *testing.T) {
	p := New(DefaultConfig())
	if url, state := p.RDAPDomainURL("se"); state != StateDisabled || url != "" {
		t.Fatalf("state/url = %q/%q, want disabled and no URL", state, url)
	}
}

func TestRDAPDomainURLEscapesName(t *testing.T) {
	p := enabledProvider()
	// A hostile label must not escape the path segment.
	url, _ := p.RDAPDomainURL("we ird/../x")
	if strings.Contains(url, "..") || strings.Contains(url, " ") {
		t.Fatalf("url = %q, want the name percent-encoded into one segment", url)
	}
}

func TestParseRDAPDomainGTLD(t *testing.T) {
	value, err := parseRDAPDomain(readFixture(t, "rdap-domain-gtld.json"), "https://rdap.example.com/domain/example.com")
	if err != nil {
		t.Fatalf("parseRDAPDomain: %v", err)
	}
	got := value.(*RDAPDomainSummary)
	if got.Handle != "2336799_DOMAIN_COM-VRSN" {
		t.Errorf("handle = %q", got.Handle)
	}
	if got.Registrar != "Example Registrar, Inc." {
		t.Errorf("registrar = %q, want the vCard fn", got.Registrar)
	}
	if len(got.Status) != 2 || got.Status[0] != "client delete prohibited" {
		t.Errorf("status = %v", got.Status)
	}
	if got.RegisteredAt == nil || got.RegisteredAt.Year() != 1995 {
		t.Errorf("registered_at = %v, want 1995", got.RegisteredAt)
	}
	if got.ExpiresAt == nil || got.ChangedAt == nil {
		t.Errorf("expires/changed = %v/%v, want both set", got.ExpiresAt, got.ChangedAt)
	}
	// The unparseable "last update of RDAP database" event is skipped, not fatal.
	if got.DelegationSigned == nil || !*got.DelegationSigned {
		t.Errorf("delegation_signed = %v, want true", got.DelegationSigned)
	}
	want := []string{"a.iana-servers.net", "b.iana-servers.net"}
	if len(got.Nameservers) != 2 || got.Nameservers[0] != want[0] || got.Nameservers[1] != want[1] {
		t.Errorf("nameservers = %v, want %v normalized", got.Nameservers, want)
	}
	if got.SourceURL != "https://rdap.example.com/domain/example.com" {
		t.Errorf("source_url = %q", got.SourceURL)
	}
}

func TestParseRDAPDomainCCTLD(t *testing.T) {
	value, err := parseRDAPDomain(readFixture(t, "rdap-domain-cctld.json"), "https://rdap.iis.se/domain/example.se")
	if err != nil {
		t.Fatalf("parseRDAPDomain: %v", err)
	}
	got := value.(*RDAPDomainSummary)
	// No fn property: the org array is joined instead.
	if got.Registrar != "Registrar AB, Domains Unit" {
		t.Errorf("registrar = %q, want the joined org value", got.Registrar)
	}
	// A date-only registration event still parses.
	if got.RegisteredAt == nil || got.RegisteredAt.Format("2006-01-02") != "2010-01-02" {
		t.Errorf("registered_at = %v, want 2010-01-02", got.RegisteredAt)
	}
	if got.DelegationSigned == nil || *got.DelegationSigned {
		t.Errorf("delegation_signed = %v, want false", got.DelegationSigned)
	}
	if len(got.Nameservers) != 0 {
		t.Errorf("nameservers = %v, want none", got.Nameservers)
	}
}

func TestParseRDAPDomainTLDUsesRegistrantAsRegistry(t *testing.T) {
	value, err := parseRDAPDomain(readFixture(t, "rdap-domain-tld.json"), "https://rdap.iana.org/domain/se")
	if err != nil {
		t.Fatalf("parseRDAPDomain: %v", err)
	}
	got := value.(*RDAPDomainSummary)
	if got.RegistryOrg != "The Swedish Internet Foundation" {
		t.Errorf("registry_org = %q, want the registrant entity name", got.RegistryOrg)
	}
	if got.Registrar != "" {
		t.Errorf("registrar = %q, want empty: a TLD record has no registrar", got.Registrar)
	}
	if got.DelegationSigned != nil {
		t.Errorf("delegation_signed = %v, want nil when secureDNS is absent", got.DelegationSigned)
	}
}

func TestParseRDAPDomainTolerantOfOddShapes(t *testing.T) {
	body := []byte(`{
	  "ldhName": "odd.example",
	  "status": ["ok"],
	  "events": [{"eventAction":"registration"},{"eventAction":"expiration","eventDate":"yesterday"}],
	  "entities": [{"roles":["registrar"]},{"roles":["registry"],"vcardArray":["vcard"]}],
	  "nameservers": [{"ldhName":""}],
	  "secureDNS": {}
	}`)
	value, err := parseRDAPDomain(body, "https://rdap.example.invalid/domain/odd.example")
	if err != nil {
		t.Fatalf("parseRDAPDomain: %v", err)
	}
	got := value.(*RDAPDomainSummary)
	if got.RegisteredAt != nil || got.ExpiresAt != nil {
		t.Errorf("dates = %v/%v, want both nil", got.RegisteredAt, got.ExpiresAt)
	}
	if got.Registrar != "" || got.RegistryOrg != "" {
		t.Errorf("entities = %q/%q, want empty when the vCards are missing", got.Registrar, got.RegistryOrg)
	}
	if len(got.Nameservers) != 0 {
		t.Errorf("nameservers = %v, want none", got.Nameservers)
	}
}

func TestParseRDAPDomainRejectsGarbage(t *testing.T) {
	if _, err := parseRDAPDomain([]byte("not json"), "https://rdap.example.invalid/"); err == nil {
		t.Fatal("parseRDAPDomain accepted a non-JSON body")
	}
}

func TestCleanText(t *testing.T) {
	if got := cleanText("  hel\x00lo\n "); got != "hello" {
		t.Errorf("cleanText control strip = %q, want hello", got)
	}
	long := strings.Repeat("x", maxTextLength+50)
	if got := cleanText(long); len(got) != maxTextLength {
		t.Errorf("cleanText length = %d, want %d", len(got), maxTextLength)
	}
}

func TestParseRDAPDate(t *testing.T) {
	at, err := parseRDAPDate("2026-09-07T10:00:00+02:00")
	if err != nil {
		t.Fatalf("parseRDAPDate: %v", err)
	}
	if !at.Equal(time.Date(2026, 9, 7, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("parsed = %v, want the UTC instant", at)
	}
	if _, err := parseRDAPDate(""); err == nil {
		t.Error("parseRDAPDate accepted an empty date")
	}
}
