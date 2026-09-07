package extdata

import (
	"os"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func TestParseTLDList(t *testing.T) {
	value, version, err := parseTLDList(readFixture(t, "tlds-alpha-by-domain.txt"))
	if err != nil {
		t.Fatalf("parseTLDList: %v", err)
	}
	list, ok := value.(*TLDList)
	if !ok {
		t.Fatalf("parseTLDList returned %T, want *TLDList", value)
	}
	// The version comes from the leading comment, which is not a name.
	if version != "2026090700" || list.Version != "2026090700" {
		t.Fatalf("version = %q / %q, want 2026090700", version, list.Version)
	}
	if got, want := list.Len(), 8; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
	for _, name := range []string{"se", "SE", "se.", " Se. ", "xn--mgbaam7a8h"} {
		if !list.Has(name) {
			t.Errorf("Has(%q) = false, want true", name)
		}
	}
	if list.Has("example.se") {
		t.Error("Has(example.se) = true, want false: multi-label names are never in the list")
	}
	if list.Has("nosuchtld") {
		t.Error("Has(nosuchtld) = true, want false")
	}
	names := list.Names()
	if len(names) != 8 || names[0] != "aaa" {
		t.Fatalf("Names() = %v, want 8 sorted names starting at aaa", names)
	}
}

func TestParseTLDListRejectsEmpty(t *testing.T) {
	if _, _, err := parseTLDList([]byte("# Version 1\n\n")); err == nil {
		t.Fatal("parseTLDList accepted a list with no names")
	}
}

func TestParseRDAPBootstrap(t *testing.T) {
	value, version, err := parseRDAPBootstrap(readFixture(t, "rdap-dns.json"))
	if err != nil {
		t.Fatalf("parseRDAPBootstrap: %v", err)
	}
	bs, ok := value.(*RDAPBootstrap)
	if !ok {
		t.Fatalf("parseRDAPBootstrap returned %T, want *RDAPBootstrap", value)
	}
	if version != "2026-09-01T00:00:00Z" {
		t.Fatalf("version = %q, want the publication timestamp", version)
	}
	if got := bs.BaseURLs("se"); len(got) != 1 || got[0] != "https://rdap.iis.se/" {
		t.Fatalf("BaseURLs(se) = %v, want [https://rdap.iis.se/]", got)
	}
	// Trailing slash is normalized and the plain-HTTP alternative dropped.
	if got := bs.BaseURLs("COM."); len(got) != 1 || got[0] != "https://rdap.verisign.com/com/v1/" {
		t.Fatalf("BaseURLs(COM.) = %v, want the single https base", got)
	}
	if got := bs.BaseURLs("example"); len(got) != 0 {
		t.Fatalf("BaseURLs(example) = %v, want none: the only base is plain HTTP", got)
	}
	if got := bs.BaseURLs("internal"); len(got) != 0 {
		t.Fatalf("BaseURLs(internal) = %v, want none: the base points at loopback", got)
	}
	if got, want := bs.Len(), 4; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
}

func TestParseRDAPBootstrapRejectsUnusable(t *testing.T) {
	for name, body := range map[string]string{
		"not json":    "{",
		"no services": `{"version":"1.0","services":[]}`,
		"http only":   `{"version":"1.0","services":[[["se"],["http://rdap.example.invalid/"]]]}`,
	} {
		if _, _, err := parseRDAPBootstrap([]byte(body)); err == nil {
			t.Errorf("%s: parseRDAPBootstrap accepted the file", name)
		}
	}
}

func TestDatasetSize(t *testing.T) {
	tlds, _, _ := parseTLDList(readFixture(t, "tlds-alpha-by-domain.txt"))
	bs, _, _ := parseRDAPBootstrap(readFixture(t, "rdap-dns.json"))
	if got := datasetSize(tlds); got != 8 {
		t.Errorf("datasetSize(tlds) = %d, want 8", got)
	}
	if got := datasetSize(bs); got != 4 {
		t.Errorf("datasetSize(bootstrap) = %d, want 4", got)
	}
	if got := datasetSize("something else"); got != 0 {
		t.Errorf("datasetSize(other) = %d, want 0", got)
	}
}

func TestNormalizeDomain(t *testing.T) {
	cases := map[string]string{
		"SE":           "se",
		"se.":          "se",
		" Example.SE ": "example.se",
		"":             "",
	}
	for in, want := range cases {
		if got := normalizeDomain(in); got != want {
			t.Errorf("normalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
}
