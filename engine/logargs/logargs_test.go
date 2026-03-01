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

func TestEnsureSchema(t *testing.T) {
	args := EnsureSchema(nil)
	if args["arg_schema"] != SchemaID {
		t.Fatalf("missing schema marker: %#v", args["arg_schema"])
	}
}
