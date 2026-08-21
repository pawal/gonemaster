package tctest

import (
	"slices"
	"strings"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

// ArgShape describes the expected split ns/address args of one entry.
// Empty fields are not asserted.
type ArgShape struct {
	NS      string
	Address string
}

// Servers coerces a servers-shaped arg value to rows, failing on other types.
func Servers(t TB, v any) []map[string]any {
	t.Helper()
	switch items := v.(type) {
	case []map[string]any:
		return items
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			row, ok := item.(map[string]any)
			if !ok {
				t.Fatalf("server entry has unexpected type %T (%#v)", item, item)
			}
			out = append(out, row)
		}
		return out
	default:
		t.Fatalf("'servers' arg has unexpected type %T (%#v)", v, v)
		return nil
	}
}

// ServerNames returns the sorted ns names of args["servers"].
func ServerNames(t TB, args map[string]any) []string {
	t.Helper()
	rows := requireServers(t, args, "servers")
	var names []string
	for _, row := range rows {
		if ns, ok := row["ns"].(string); ok && ns != "" {
			names = append(names, ns)
		}
	}
	slices.Sort(names)
	return names
}

// FirstServerName returns the ns name of the first server row, or "".
func FirstServerName(args map[string]any) string {
	if args == nil {
		return ""
	}
	rows, ok := serverRows(args["servers"])
	if !ok || len(rows) == 0 {
		return ""
	}
	name, _ := rows[0]["ns"].(string)
	return name
}

// ServerEndpoints returns the sorted "ns/address" endpoints of args["servers"].
func ServerEndpoints(t TB, args map[string]any) []string {
	t.Helper()
	return endpoints(requireServers(t, args, "servers"))
}

// EndpointsAt returns the sorted endpoints at key, or nil when key is absent
// or holds another shape.
func EndpointsAt(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	rows, ok := serverRows(args[key])
	if !ok {
		return nil
	}
	return endpoints(rows)
}

// Strings returns the string slice at key.
func Strings(t TB, args map[string]any, key string) []string {
	t.Helper()
	raw, ok := args[key]
	if !ok {
		t.Fatalf("expected %s key in args", key)
	}
	switch items := raw.(type) {
	case []string:
		return append([]string{}, items...)
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			value, ok := item.(string)
			if !ok {
				t.Fatalf("unexpected %s element type: %T", key, item)
			}
			out = append(out, value)
		}
		return out
	default:
		t.Fatalf("unexpected %s type: %T", key, raw)
		return nil
	}
}

// Ints returns the sorted int slice at key, accepting JSON float64 elements.
func Ints(t TB, args map[string]any, key string) []int {
	t.Helper()
	raw, ok := args[key]
	if !ok {
		t.Fatalf("expected %s key in args", key)
	}
	var out []int
	switch items := raw.(type) {
	case []int:
		out = append([]int{}, items...)
	case []any:
		out = make([]int, 0, len(items))
		for _, item := range items {
			switch v := item.(type) {
			case int:
				out = append(out, v)
			case float64:
				out = append(out, int(v))
			default:
				t.Fatalf("unexpected %s element type: %T", key, item)
			}
		}
	default:
		t.Fatalf("unexpected %s type: %T", key, raw)
		return nil
	}
	slices.Sort(out)
	return out
}

// ArgValues returns the string value of key for every entry carrying tag,
// preserving emission order.
func ArgValues(entries []*logger.Entry, tag string, key string) []string {
	var out []string
	for _, entry := range All(entries, tag) {
		if v, ok := entry.Args[key].(string); ok {
			out = append(out, v)
		}
	}
	return out
}

// RequireArgShape asserts the entry uses split ns/address args and no arg_schema.
func RequireArgShape(t TB, entry *logger.Entry, want ArgShape) {
	t.Helper()
	if entry == nil {
		t.Fatalf("expected an entry to check arg shape")
	}
	if _, ok := entry.Args["arg_schema"]; ok {
		t.Fatalf("did not expect arg_schema in args: %#v", entry.Args["arg_schema"])
	}
	ns, _ := entry.Args["ns"].(string)
	if strings.Contains(ns, "/") {
		t.Fatalf("expected nameserver-only ns argument, got %q", ns)
	}
	if want.NS != "" && ns != want.NS {
		t.Fatalf("expected ns=%s, got %#v", want.NS, entry.Args["ns"])
	}
	if want.Address != "" {
		if address, _ := entry.Args["address"].(string); address != want.Address {
			t.Fatalf("expected address=%s, got %#v", want.Address, entry.Args["address"])
		}
	}
}

func requireServers(t TB, args map[string]any, key string) []map[string]any {
	t.Helper()
	raw, ok := args[key]
	if !ok {
		t.Fatalf("expected %s key in args", key)
	}
	rows, ok := serverRows(raw)
	if !ok {
		t.Fatalf("unexpected %s type: %T", key, raw)
	}
	return rows
}

func serverRows(v any) ([]map[string]any, bool) {
	switch items := v.(type) {
	case []map[string]any:
		return items, true
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, row)
		}
		return out, true
	default:
		return nil, false
	}
}

func endpoints(rows []map[string]any) []string {
	var out []string
	for _, row := range rows {
		ns, _ := row["ns"].(string)
		address, _ := row["address"].(string)
		ns = strings.TrimSpace(ns)
		address = strings.TrimSpace(address)
		switch {
		case ns != "" && address != "":
			out = append(out, ns+"/"+address)
		case ns != "":
			out = append(out, ns)
		case address != "":
			out = append(out, address)
		}
	}
	slices.Sort(out)
	return out
}
