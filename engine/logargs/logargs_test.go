package logargs

import "testing"

type fakeNS struct {
	name string
	addr string
}

func (f fakeNS) NameString() string    { return f.name }
func (f fakeNS) AddressString() string { return f.addr }

func TestNS(t *testing.T) {
	got := NS("NS1.Example.ORG", "192.0.2.10")
	if got["ns"] != "ns1.example.org" {
		t.Fatalf("unexpected ns: %#v", got["ns"])
	}
	if got["address"] != "192.0.2.10" {
		t.Fatalf("unexpected address: %#v", got["address"])
	}
}

func TestServersFromNameservers(t *testing.T) {
	items := []fakeNS{
		{name: "b.example.", addr: "192.0.2.2"},
		{name: "a.example.", addr: "192.0.2.1"},
	}
	got := ServersFromNameservers(items)
	raw, ok := got["servers"].([]map[string]any)
	if !ok {
		t.Fatalf("missing servers list: %#v", got["servers"])
	}
	if len(raw) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(raw))
	}
	if raw[0]["ns"] != "a.example" || raw[1]["ns"] != "b.example" {
		t.Fatalf("servers not sorted/normed: %#v", raw)
	}
}

func TestServersDeterministicOrdering(t *testing.T) {
	got := Servers([]Server{
		{NS: "b.example.", Address: "192.0.2.9"},
		{NS: "a.example.", Address: "192.0.2.2"},
		{NS: "a.example.", Address: "192.0.2.1"},
		{NS: " ", Address: "2001:db8::53"},
		{NS: "", Address: ""},
	})

	raw, ok := got["servers"].([]map[string]any)
	if !ok {
		t.Fatalf("missing servers list: %#v", got["servers"])
	}
	if len(raw) != 4 {
		t.Fatalf("expected 4 emitted server objects, got %d", len(raw))
	}
	if raw[0]["ns"] != "a.example" || raw[0]["address"] != "192.0.2.1" {
		t.Fatalf("unexpected first server: %#v", raw[0])
	}
	if raw[1]["ns"] != "a.example" || raw[1]["address"] != "192.0.2.2" {
		t.Fatalf("unexpected second server: %#v", raw[1])
	}
	if raw[2]["ns"] != "b.example" || raw[2]["address"] != "192.0.2.9" {
		t.Fatalf("unexpected third server: %#v", raw[2])
	}
	if _, hasNS := raw[3]["ns"]; hasNS {
		t.Fatalf("expected address-only server entry, got: %#v", raw[3])
	}
	if raw[3]["address"] != "2001:db8::53" {
		t.Fatalf("unexpected fourth server address: %#v", raw[3])
	}
}

func TestSetQueryIdentity(t *testing.T) {
	args := map[string]any{}
	SetQueryIdentity(args, "WWW.Example.org.", "soa", "in")
	if args["query_name"] != "www.example.org" {
		t.Fatalf("unexpected query_name: %#v", args["query_name"])
	}
	if args["query_type"] != "SOA" {
		t.Fatalf("unexpected query_type: %#v", args["query_type"])
	}
	if args["query_class"] != "IN" {
		t.Fatalf("unexpected query_class: %#v", args["query_class"])
	}
}

func TestSetQueryIdentityOmitsEmptyValues(t *testing.T) {
	args := map[string]any{}
	SetQueryIdentity(args, " ", " ", "")
	if len(args) != 0 {
		t.Fatalf("expected no emitted fields, got %#v", args)
	}
}

func TestEnsureQueryIdentityFromRrtype(t *testing.T) {
	args := map[string]any{"rrtype": "soa"}
	EnsureQueryIdentity(args)

	if args["query_type"] != "SOA" {
		t.Fatalf("unexpected query_type: %#v", args["query_type"])
	}
	if args["query_class"] != "IN" {
		t.Fatalf("unexpected query_class: %#v", args["query_class"])
	}
}

func TestEnsureQueryIdentityFromTypeAndClass(t *testing.T) {
	args := map[string]any{
		"type":  "mx",
		"class": "ch",
	}
	EnsureQueryIdentity(args)

	if args["query_type"] != "MX" {
		t.Fatalf("unexpected query_type: %#v", args["query_type"])
	}
	if args["query_class"] != "CH" {
		t.Fatalf("unexpected query_class: %#v", args["query_class"])
	}
}

func TestEnsureQueryIdentityKeepsCanonicalValues(t *testing.T) {
	args := map[string]any{
		"query_type":  "TXT",
		"query_class": "HS",
		"rrtype":      "A",
		"class":       "IN",
	}
	EnsureQueryIdentity(args)

	if args["query_type"] != "TXT" {
		t.Fatalf("expected canonical query_type to win, got %#v", args["query_type"])
	}
	if args["query_class"] != "HS" {
		t.Fatalf("expected canonical query_class to win, got %#v", args["query_class"])
	}
}

func TestSetNSSetsOnlyProvidedValues(t *testing.T) {
	args := map[string]any{}
	SetNS(args, "", " 192.0.2.44 ")
	if _, ok := args["ns"]; ok {
		t.Fatalf("did not expect ns key in %#v", args)
	}
	if args["address"] != "192.0.2.44" {
		t.Fatalf("unexpected address: %#v", args["address"])
	}
}

func TestEndpointName(t *testing.T) {
	if got := EndpointName("NS1.Example.org./192.0.2.1"); got != "ns1.example.org" {
		t.Fatalf("unexpected endpoint name: %q", got)
	}
	if got := EndpointName("192.0.2.0/24"); got != "192.0.2.0/24" {
		t.Fatalf("prefix should be unchanged: %q", got)
	}
	if got := EndpointName("  - "); got != "-" {
		t.Fatalf("unexpected passthrough value: %q", got)
	}
}

func TestUniqueSortedEndpointNames(t *testing.T) {
	got := UniqueSortedEndpointNames([]string{
		"ns2.example/2001:db8::53",
		"NS1.Example/192.0.2.1",
		"ns1.example/192.0.2.2",
		"192.0.2.0/24",
		"",
	})
	want := []string{"192.0.2.0/24", "ns1.example", "ns2.example"}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected value at %d: got=%v want=%v", i, got, want)
		}
	}
}
