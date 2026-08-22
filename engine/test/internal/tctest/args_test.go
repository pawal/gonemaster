package tctest

import (
	"reflect"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/internal/tbtest"
)

// serverArgs builds a servers arg in the []map[string]any shape the engine emits.
func serverArgs(rows ...map[string]any) map[string]any {
	return map[string]any{"servers": rows}
}

// jsonServerArgs builds a servers arg in the []any shape a JSON round-trip yields.
func jsonServerArgs(rows ...any) map[string]any {
	return map[string]any{"servers": rows}
}

func TestServersAcceptsBothShapes(t *testing.T) {
	rows := Servers(t, []map[string]any{{"ns": "ns1.example"}})
	if len(rows) != 1 || rows[0]["ns"] != "ns1.example" {
		t.Fatalf("expected one ns1.example row, got %#v", rows)
	}

	rows = Servers(t, []any{map[string]any{"ns": "ns2.example"}})
	if len(rows) != 1 || rows[0]["ns"] != "ns2.example" {
		t.Fatalf("expected one ns2.example row, got %#v", rows)
	}
}

func TestServersRejectsOtherTypes(t *testing.T) {
	tbtest.MustFail(t, "unexpected type string", func(tb *tbtest.TB) { Servers(tb, "ns1.example") })
	tbtest.MustFail(t, "server entry has unexpected type", func(tb *tbtest.TB) { Servers(tb, []any{"ns1.example"}) })
}

func TestServerNamesSortsAndSkipsEmpty(t *testing.T) {
	args := serverArgs(
		map[string]any{"ns": "ns2.example"},
		map[string]any{"ns": ""},
		map[string]any{"address": "192.0.2.1"},
		map[string]any{"ns": "ns1.example"},
	)

	if got, want := ServerNames(t, args), []string{"ns1.example", "ns2.example"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestServerNamesFailsOnMissingOrWrongType(t *testing.T) {
	tbtest.MustFail(t, "expected servers key", func(tb *tbtest.TB) { ServerNames(tb, map[string]any{}) })
	tbtest.MustFail(t, "unexpected servers type", func(tb *tbtest.TB) { ServerNames(tb, map[string]any{"servers": 7}) })
}

func TestFirstServerName(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"nil args", nil, ""},
		{"missing key", map[string]any{}, ""},
		{"empty list", serverArgs(), ""},
		{"typed rows", serverArgs(map[string]any{"ns": "ns1.example"}, map[string]any{"ns": "ns2.example"}), "ns1.example"},
		{"json rows", jsonServerArgs(map[string]any{"ns": "ns3.example"}), "ns3.example"},
		{"no ns key", serverArgs(map[string]any{"address": "192.0.2.1"}), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FirstServerName(tc.args); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestServerEndpointsJoinsNameAndAddress(t *testing.T) {
	args := serverArgs(
		map[string]any{"ns": " ns2.example ", "address": " 192.0.2.2 "},
		map[string]any{"ns": "ns1.example"},
		map[string]any{"address": "192.0.2.3"},
		map[string]any{},
	)

	want := []string{"192.0.2.3", "ns1.example", "ns2.example/192.0.2.2"}
	if got := ServerEndpoints(t, args); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	tbtest.MustFail(t, "expected servers key", func(tb *tbtest.TB) { ServerEndpoints(tb, map[string]any{}) })
}

func TestEndpointsAtIsLenient(t *testing.T) {
	args := map[string]any{"parent_servers": []map[string]any{{"ns": "ns1.other", "address": "192.0.2.1"}}}

	if got, want := EndpointsAt(args, "parent_servers"), []string{"ns1.other/192.0.2.1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if got := EndpointsAt(args, "zone_servers"); got != nil {
		t.Fatalf("expected nil for a missing key, got %v", got)
	}
	if got := EndpointsAt(nil, "parent_servers"); got != nil {
		t.Fatalf("expected nil for nil args, got %v", got)
	}
	if got := EndpointsAt(map[string]any{"zone_servers": 7}, "zone_servers"); got != nil {
		t.Fatalf("expected nil for an unexpected type, got %v", got)
	}
}

func TestStrings(t *testing.T) {
	if got, want := Strings(t, map[string]any{"k": []string{"a", "b"}}, "k"), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if got, want := Strings(t, map[string]any{"k": []any{"a", "b"}}, "k"), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	tbtest.MustFail(t, "expected k key in args", func(tb *tbtest.TB) { Strings(tb, map[string]any{}, "k") })
	tbtest.MustFail(t, "unexpected k element type: int", func(tb *tbtest.TB) { Strings(tb, map[string]any{"k": []any{1}}, "k") })
	tbtest.MustFail(t, "unexpected k type: int", func(tb *tbtest.TB) { Strings(tb, map[string]any{"k": 1}, "k") })
}

func TestIntsSortsAndAcceptsFloats(t *testing.T) {
	if got, want := Ints(t, map[string]any{"k": []int{3, 1}}, "k"), []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if got, want := Ints(t, map[string]any{"k": []any{3, float64(1)}}, "k"), []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	tbtest.MustFail(t, "expected k key in args", func(tb *tbtest.TB) { Ints(tb, map[string]any{}, "k") })
	tbtest.MustFail(t, "unexpected k element type: string", func(tb *tbtest.TB) { Ints(tb, map[string]any{"k": []any{"1"}}, "k") })
	tbtest.MustFail(t, "unexpected k type: string", func(tb *tbtest.TB) { Ints(tb, map[string]any{"k": "1"}, "k") })
}

func TestArgValuesKeepsEmissionOrder(t *testing.T) {
	entries := []*logger.Entry{
		entry("MISSING", map[string]any{"ns": "ns2.example"}),
		nil,
		entry("OTHER", map[string]any{"ns": "ns3.example"}),
		entry("MISSING", map[string]any{"ns": "ns1.example"}),
		entry("MISSING", map[string]any{"address": "192.0.2.1"}),
	}

	want := []string{"ns2.example", "ns1.example"}
	if got := ArgValues(entries, "MISSING", "ns"); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if got := ArgValues(entries, "NONE", "ns"); got != nil {
		t.Fatalf("expected nil for a missing tag, got %v", got)
	}
}

func TestRequireArgShape(t *testing.T) {
	split := entry("NO_RESPONSE", map[string]any{"ns": "a.root", "address": "192.0.2.1"})
	RequireArgShape(t, split, ArgShape{})
	RequireArgShape(t, split, ArgShape{NS: "a.root", Address: "192.0.2.1"})

	tbtest.MustFail(t, "expected an entry", func(tb *tbtest.TB) { RequireArgShape(tb, nil, ArgShape{}) })
	tbtest.MustFail(t, "did not expect arg_schema", func(tb *tbtest.TB) {
		RequireArgShape(tb, entry("T", map[string]any{"arg_schema": "ns_ip"}), ArgShape{})
	})
	tbtest.MustFail(t, "nameserver-only ns argument", func(tb *tbtest.TB) {
		RequireArgShape(tb, entry("T", map[string]any{"ns": "a.root/192.0.2.1"}), ArgShape{})
	})
	tbtest.MustFail(t, "expected ns=b.root", func(tb *tbtest.TB) { RequireArgShape(tb, split, ArgShape{NS: "b.root"}) })
	tbtest.MustFail(t, "expected address=192.0.2.2", func(tb *tbtest.TB) { RequireArgShape(tb, split, ArgShape{Address: "192.0.2.2"}) })
}
